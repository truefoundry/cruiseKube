package task

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/truefoundry/autopilot-oss/pkg/adapters/metricsProvider/prometheus"
	"github.com/truefoundry/autopilot-oss/pkg/contextutils"
	"github.com/truefoundry/autopilot-oss/pkg/logging"
	"github.com/truefoundry/autopilot-oss/pkg/task/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type ModifyEqualCPUResourcesTaskConfig struct {
	Name                     string
	Enabled                  bool
	Schedule                 string
	ClusterID                string
	IsClusterWriteAuthorized bool
}

type ModifyEqualCPUResourcesTask struct {
	config        *ModifyEqualCPUResourcesTaskConfig
	kubeClient    *kubernetes.Clientset
	dynamicClient dynamic.Interface
	promClient    *prometheus.PrometheusProvider
}

func NewModifyEqualCPUResourcesTask(ctx context.Context, kubeClient *kubernetes.Clientset, dynamicClient dynamic.Interface, promClient *prometheus.PrometheusProvider, config *ModifyEqualCPUResourcesTaskConfig) *ModifyEqualCPUResourcesTask {
	return &ModifyEqualCPUResourcesTask{
		config:        config,
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
		promClient:    promClient,
	}
}

func (m *ModifyEqualCPUResourcesTask) GetCoreTask() any {
	return m
}

func (m *ModifyEqualCPUResourcesTask) GetName() string {
	return m.config.Name
}

func (m *ModifyEqualCPUResourcesTask) GetSchedule() string {
	return m.config.Schedule
}

func (m *ModifyEqualCPUResourcesTask) IsEnabled() bool {
	return m.config.Enabled
}

func (m *ModifyEqualCPUResourcesTask) Run(ctx context.Context) error {
	ctx = contextutils.WithTask(ctx, m.config.Name)
	ctx = contextutils.WithCluster(ctx, m.config.ClusterID)

	if !m.config.IsClusterWriteAuthorized {
		logging.Infof(ctx, "Cluster %s is not write authorized, skipping ModifyEqualCPUResources task", m.config.ClusterID)
		return nil
	}

	const targetNamespace = ""

	workloadList, err := utils.ListAllWorkloadsWithSelectors(ctx, m.kubeClient, targetNamespace)
	if err != nil {
		logging.Errorf(ctx, "Error getting workload list with selectors: %v", err)
		return err
	}

	allPods, err := m.getAllPodsAcrossNamespaces(ctx, targetNamespace)
	if err != nil {
		logging.Errorf(ctx, "Error getting pods: %v", err)
		return err
	}

	logging.Infof(ctx, "Found %d workloads and %d pods to process", len(workloadList), len(allPods))

	totalModified := 0
	for _, workloadInfo := range workloadList {
		if m.hasAlivePods(workloadInfo, allPods) {
			modified := m.processWorkload(ctx, workloadInfo)
			totalModified += modified
		} else {
			workloadKey := utils.GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)
			logging.Infof(ctx, "Skipping workload %s - no alive pods found", workloadKey)
		}
	}

	return nil
}

func (m *ModifyEqualCPUResourcesTask) hasAlivePods(workloadInfo utils.WorkloadLabelSelectorList, allPods []corev1.Pod) bool {
	for _, pod := range allPods {
		if pod.Namespace == workloadInfo.Namespace &&
			(pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending) {
			if workloadInfo.Selector.Matches(labels.Set(pod.Labels)) {
				return true
			}
		}
	}

	return false
}

type containerModification struct {
	Name             string
	NewCPULimitMilli int64
}

func (m *ModifyEqualCPUResourcesTask) getAllPodsAcrossNamespaces(ctx context.Context, targetNamespace string) ([]corev1.Pod, error) {

	var podList *corev1.PodList
	var err error

	if targetNamespace != "" {
		podList, err = m.kubeClient.CoreV1().Pods(targetNamespace).List(ctx, metav1.ListOptions{})
	} else {
		podList, err = m.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	return podList.Items, nil
}

func (m *ModifyEqualCPUResourcesTask) processWorkload(ctx context.Context, workloadInfo utils.WorkloadLabelSelectorList) int {
	workloadKey := utils.GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)

	workloadObj, err := utils.GetWorkloadObject(ctx, m.kubeClient, workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)
	if err != nil {
		logging.Errorf(ctx, "Error getting workload %s: %v", workloadKey, err)
		return 0
	}

	containerSpecs := append(workloadObj.GetContainerSpecs(ctx, m.kubeClient), workloadObj.GetInitContainerSpecs(ctx, m.kubeClient)...)
	containersToModify := []containerModification{}

	for _, container := range containerSpecs {
		cpuRequest := container.Resources.Requests[corev1.ResourceCPU]
		cpuLimit := container.Resources.Limits[corev1.ResourceCPU]

		if !cpuRequest.IsZero() && !cpuLimit.IsZero() {
			requestMilliCPU := cpuRequest.MilliValue()
			limitMilliCPU := cpuLimit.MilliValue()

			if requestMilliCPU == limitMilliCPU {
				newLimitMilliCPU := limitMilliCPU + 1
				logging.Infof(ctx, "Found equal CPU request/limit in %s container %s: %dm -> %dm/%dm",
					workloadKey, container.Name, requestMilliCPU, requestMilliCPU, newLimitMilliCPU)

				containersToModify = append(containersToModify, containerModification{
					Name:             container.Name,
					NewCPULimitMilli: newLimitMilliCPU,
				})
			}
		}
	}

	if len(containersToModify) == 0 {
		return 0
	}

	if err := m.updateWorkloadCPULimits(ctx, workloadInfo, containersToModify); err != nil {
		logging.Errorf(ctx, "Error updating workload %s: %v", workloadKey, err)
		return 0
	}

	logging.Infof(ctx, "Successfully modified %d containers in workload %s", len(containersToModify), workloadKey)
	return len(containersToModify)
}

func (m *ModifyEqualCPUResourcesTask) updateWorkloadCPULimits(ctx context.Context, workloadInfo utils.WorkloadLabelSelectorList, modifications []containerModification) error {

	switch workloadInfo.Kind {
	case utils.DeploymentKind:
		return m.updateDeploymentCPULimits(ctx, workloadInfo, modifications)
	case utils.StatefulSetKind:
		return m.updateStatefulSetCPULimits(ctx, workloadInfo, modifications)
	case utils.DaemonSetKind:
		return m.updateDaemonSetCPULimits(ctx, workloadInfo, modifications)
	default:
		return fmt.Errorf("unsupported workload kind: %s", workloadInfo.Kind)
	}
}

func (m *ModifyEqualCPUResourcesTask) updateDeploymentCPULimits(ctx context.Context, workloadInfo utils.WorkloadLabelSelectorList, modifications []containerModification) error {
	deployment, err := m.kubeClient.AppsV1().Deployments(workloadInfo.Namespace).Get(ctx, workloadInfo.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting deployment: %w", err)
	}

	var patches []map[string]interface{}
	for _, mod := range modifications {
		for i, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name == mod.Name {
				patches = append(patches, map[string]interface{}{
					"op":    "replace",
					"path":  fmt.Sprintf("/spec/template/spec/containers/%d/resources/limits/cpu", i),
					"value": fmt.Sprintf("%dm", mod.NewCPULimitMilli),
				})
				break
			}
		}
	}

	if len(patches) == 0 {
		return nil
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON patch: %w", err)
	}

	_, err = m.kubeClient.AppsV1().Deployments(workloadInfo.Namespace).Patch(
		ctx,
		workloadInfo.Name,
		types.JSONPatchType,
		patchBytes,
		metav1.PatchOptions{},
	)

	return err
}

func (m *ModifyEqualCPUResourcesTask) updateStatefulSetCPULimits(ctx context.Context, workloadInfo utils.WorkloadLabelSelectorList, modifications []containerModification) error {
	statefulSet, err := m.kubeClient.AppsV1().StatefulSets(workloadInfo.Namespace).Get(ctx, workloadInfo.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting statefulset: %w", err)
	}

	var patches []map[string]interface{}
	for _, mod := range modifications {
		for i, container := range statefulSet.Spec.Template.Spec.Containers {
			if container.Name == mod.Name {
				patches = append(patches, map[string]interface{}{
					"op":    "replace",
					"path":  fmt.Sprintf("/spec/template/spec/containers/%d/resources/limits/cpu", i),
					"value": fmt.Sprintf("%dm", mod.NewCPULimitMilli),
				})
				break
			}
		}
	}

	if len(patches) == 0 {
		return nil
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON patch: %w", err)
	}

	_, err = m.kubeClient.AppsV1().StatefulSets(workloadInfo.Namespace).Patch(
		ctx,
		workloadInfo.Name,
		types.JSONPatchType,
		patchBytes,
		metav1.PatchOptions{},
	)

	return err
}

func (m *ModifyEqualCPUResourcesTask) updateDaemonSetCPULimits(ctx context.Context, workloadInfo utils.WorkloadLabelSelectorList, modifications []containerModification) error {
	daemonSet, err := m.kubeClient.AppsV1().DaemonSets(workloadInfo.Namespace).Get(ctx, workloadInfo.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting daemonset: %w", err)
	}

	var patches []map[string]interface{}
	for _, mod := range modifications {
		for i, container := range daemonSet.Spec.Template.Spec.Containers {
			if container.Name == mod.Name {
				patches = append(patches, map[string]interface{}{
					"op":    "replace",
					"path":  fmt.Sprintf("/spec/template/spec/containers/%d/resources/limits/cpu", i),
					"value": fmt.Sprintf("%dm", mod.NewCPULimitMilli),
				})
				break
			}
		}
	}

	if len(patches) == 0 {
		return nil
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON patch: %w", err)
	}

	_, err = m.kubeClient.AppsV1().DaemonSets(workloadInfo.Namespace).Patch(
		ctx,
		workloadInfo.Name,
		types.JSONPatchType,
		patchBytes,
		metav1.PatchOptions{},
	)

	return err
}
