package utils

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/truefoundry/cruisekube/pkg/logging"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// WorkloadObject represents any Kubernetes workload that can be managed
type WorkloadObject interface {
	GetNamespace() string
	GetName() string
	GetContainerSpecs(ctx context.Context, kubeClient *kubernetes.Clientset) []corev1.Container
	GetInitContainerSpecs(ctx context.Context, kubeClient *kubernetes.Clientset) []corev1.Container
	GetSelector() (labels.Selector, error)
	GetCreationTime() time.Time
}

// DeploymentWrapper wraps appsv1.Deployment to implement WorkloadObject
type DeploymentWrapper struct {
	*appsv1.Deployment
}

func (d DeploymentWrapper) GetContainerSpecs(ctx context.Context, kubeClient *kubernetes.Clientset) []corev1.Container {
	selector, err := d.GetSelector()
	if err != nil {
		logging.Errorf(ctx, "Error getting selector for deployment %s/%s: %v", d.Namespace, d.Name, err)
		return d.Spec.Template.Spec.Containers
	}

	// getting fresh pods as dynamically injected containers are not tracked in workload spec
	pods, err := GetPods(ctx, kubeClient, d.Namespace, selector)
	if err != nil || len(pods.Items) == 0 {
		logging.Errorf(ctx, "Error getting pods for deployment %s/%s: %v", d.Namespace, d.Name, err)
		return d.Spec.Template.Spec.Containers
	}

	return pods.Items[0].Spec.Containers
}

func (d DeploymentWrapper) GetInitContainerSpecs(ctx context.Context, kubeClient *kubernetes.Clientset) []corev1.Container {
	selector, err := d.GetSelector()
	if err != nil {
		logging.Errorf(ctx, "Error getting selector for deployment %s/%s: %v", d.Namespace, d.Name, err)
		return d.Spec.Template.Spec.InitContainers
	}

	// getting fresh pods as dynamically injected containers are not tracked in workload spec
	pods, err := GetPods(ctx, kubeClient, d.Namespace, selector)
	if err != nil || len(pods.Items) == 0 {
		logging.Warnf(ctx, "Could not get pods for deployment %s/%s, falling back to template: %v", d.Namespace, d.Name, err)
		return d.Spec.Template.Spec.InitContainers
	}

	return pods.Items[0].Spec.InitContainers
}

func (d DeploymentWrapper) GetSelector() (labels.Selector, error) {
	selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("LabelSelectorAsSelector failed for selector %v: %w", d.Spec.Selector, err)
	}
	return selector, nil
}

func (d DeploymentWrapper) GetCreationTime() time.Time {
	return d.CreationTimestamp.Time
}

// StatefulSetWrapper wraps appsv1.StatefulSet to implement WorkloadObject
type StatefulSetWrapper struct {
	*appsv1.StatefulSet
}

func (s StatefulSetWrapper) GetContainerSpecs(ctx context.Context, kubeClient *kubernetes.Clientset) []corev1.Container {
	selector, err := s.GetSelector()
	if err != nil {
		logging.Errorf(ctx, "Error getting selector for statefulset %s/%s: %v", s.Namespace, s.Name, err)
		return s.Spec.Template.Spec.Containers
	}

	// getting fresh pods as dynamically injected containers are not tracked in workload spec
	pods, err := GetPods(ctx, kubeClient, s.Namespace, selector)
	if err != nil || len(pods.Items) == 0 {
		logging.Warnf(ctx, "Could not get pods for statefulset %s/%s, falling back to template: %v", s.Namespace, s.Name, err)
		return s.Spec.Template.Spec.Containers
	}

	return pods.Items[0].Spec.Containers
}

func (s StatefulSetWrapper) GetInitContainerSpecs(ctx context.Context, kubeClient *kubernetes.Clientset) []corev1.Container {
	selector, err := s.GetSelector()
	if err != nil {
		logging.Errorf(ctx, "Error getting selector for statefulset %s/%s: %v", s.Namespace, s.Name, err)
		return s.Spec.Template.Spec.InitContainers
	}

	// getting fresh pods as dynamically injected containers are not tracked in workload spec
	pods, err := GetPods(ctx, kubeClient, s.Namespace, selector)
	if err != nil || len(pods.Items) == 0 {
		logging.Warnf(ctx, "Could not get pods for statefulset %s/%s, falling back to template: %v", s.Namespace, s.Name, err)
		return s.Spec.Template.Spec.InitContainers
	}

	return pods.Items[0].Spec.InitContainers
}

func (s StatefulSetWrapper) GetSelector() (labels.Selector, error) {
	selector, err := metav1.LabelSelectorAsSelector(s.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("LabelSelectorAsSelector failed for selector %v: %w", s.Spec.Selector, err)
	}
	return selector, nil
}

func (s StatefulSetWrapper) GetCreationTime() time.Time {
	return s.CreationTimestamp.Time
}

// DaemonSetWrapper wraps appsv1.DaemonSet to implement WorkloadObject
type DaemonSetWrapper struct {
	*appsv1.DaemonSet
}

func (d DaemonSetWrapper) GetContainerSpecs(ctx context.Context, kubeClient *kubernetes.Clientset) []corev1.Container {
	selector, err := d.GetSelector()
	if err != nil {
		logging.Errorf(ctx, "Error getting selector for daemonset %s/%s: %v", d.Namespace, d.Name, err)
		return d.Spec.Template.Spec.Containers
	}

	// getting fresh pods as dynamically injected containers are not tracked in workload spec
	pods, err := GetPods(ctx, kubeClient, d.Namespace, selector)
	if err != nil || len(pods.Items) == 0 {
		logging.Warnf(ctx, "Could not get pods for daemonset %s/%s, falling back to template: %v", d.Namespace, d.Name, err)
		return d.Spec.Template.Spec.Containers
	}

	return pods.Items[0].Spec.Containers
}

func (d DaemonSetWrapper) GetInitContainerSpecs(ctx context.Context, kubeClient *kubernetes.Clientset) []corev1.Container {
	selector, err := d.GetSelector()
	if err != nil {
		logging.Errorf(ctx, "Error getting selector for daemonset %s/%s: %v", d.Namespace, d.Name, err)
		return d.Spec.Template.Spec.InitContainers
	}

	// getting fresh pods as dynamically injected containers are not tracked in workload spec
	pods, err := GetPods(ctx, kubeClient, d.Namespace, selector)
	if err != nil || len(pods.Items) == 0 {
		logging.Warnf(ctx, "Could not get pods for daemonset %s/%s, falling back to template: %v", d.Namespace, d.Name, err)
		return d.Spec.Template.Spec.InitContainers
	}

	return pods.Items[0].Spec.InitContainers
}

func (d DaemonSetWrapper) GetSelector() (labels.Selector, error) {
	selector, err := metav1.LabelSelectorAsSelector(d.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("LabelSelectorAsSelector failed for selector %v: %w", d.Spec.Selector, err)
	}
	return selector, nil
}

func (d DaemonSetWrapper) GetCreationTime() time.Time {
	return d.CreationTimestamp.Time
}

// GetWorkloadObject retrieves a workload object by kind, namespace, and name
func GetWorkloadObject(ctx context.Context, kubeClient *kubernetes.Clientset, kind, namespace, name string) (WorkloadObject, error) {
	switch kind {
	case DeploymentKind:
		deployment, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error getting deployment %s/%s: %w", namespace, name, err)
		}
		return DeploymentWrapper{deployment}, nil

	case StatefulSetKind:
		statefulSet, err := kubeClient.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error getting statefulset %s/%s: %w", namespace, name, err)
		}
		return StatefulSetWrapper{statefulSet}, nil

	case DaemonSetKind:
		daemonSet, err := kubeClient.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error getting daemonset %s/%s: %w", namespace, name, err)
		}
		return DaemonSetWrapper{daemonSet}, nil

	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", kind)
	}
}

// ListAllWorkloads lists all workloads of all supported types in a namespace
func ListAllWorkloads(ctx context.Context, kubeClient *kubernetes.Clientset, targetNamespace string) ([]WorkloadInfo, error) {
	var workloads []WorkloadInfo

	// List Deployments
	deployments, err := kubeClient.AppsV1().Deployments(targetNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logging.Infof(ctx, "Could not list deployments: %v", err)
	} else {
		for _, deployment := range deployments.Items {
			if deployment.Spec.Selector != nil {
				workloads = append(workloads, WorkloadInfo{
					Kind:      DeploymentKind,
					Namespace: deployment.Namespace,
					Name:      deployment.Name,
				})
			}
		}
	}

	// List StatefulSets
	statefulSets, err := kubeClient.AppsV1().StatefulSets(targetNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logging.Errorf(ctx, "Could not list statefulsets: %v", err)
	} else {
		for _, statefulSet := range statefulSets.Items {
			if statefulSet.Spec.Selector != nil {
				workloads = append(workloads, WorkloadInfo{
					Kind:      StatefulSetKind,
					Namespace: statefulSet.Namespace,
					Name:      statefulSet.Name,
				})
			}
		}
	}

	// List DaemonSets
	daemonSets, err := kubeClient.AppsV1().DaemonSets(targetNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logging.Errorf(ctx, "Could not list daemonsets: %v", err)
	} else {
		for _, daemonSet := range daemonSets.Items {
			if daemonSet.Spec.Selector != nil {
				workloads = append(workloads, WorkloadInfo{
					Kind:      DaemonSetKind,
					Namespace: daemonSet.Namespace,
					Name:      daemonSet.Name,
				})
			}
		}
	}

	return workloads, nil
}

// ListAllWorkloadsWithSelectors lists all workloads with their label selectors
func ListAllWorkloadsWithSelectors(ctx context.Context, kubeClient *kubernetes.Clientset, targetNamespace string) ([]WorkloadLabelSelectorList, error) {
	var workloads []WorkloadLabelSelectorList

	// List Deployments
	deployments, err := kubeClient.AppsV1().Deployments(targetNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logging.Errorf(ctx, "Could not list deployments: %v", err)
	} else {
		for _, deployment := range deployments.Items {
			if deployment.Spec.Selector != nil {
				selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
				if err != nil {
					logging.Errorf(ctx, "Invalid selector for deployment %s/%s: %v", deployment.Namespace, deployment.Name, err)
					continue
				}
				workloads = append(workloads, WorkloadLabelSelectorList{
					Kind:      DeploymentKind,
					Namespace: deployment.Namespace,
					Name:      deployment.Name,
					Selector:  selector,
				})
			}
		}
	}

	// List StatefulSets
	statefulSets, err := kubeClient.AppsV1().StatefulSets(targetNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logging.Errorf(ctx, "Could not list statefulsets: %v", err)
	} else {
		for _, statefulSet := range statefulSets.Items {
			if statefulSet.Spec.Selector != nil {
				selector, err := metav1.LabelSelectorAsSelector(statefulSet.Spec.Selector)
				if err != nil {
					logging.Errorf(ctx, "Invalid selector for statefulset %s/%s: %v", statefulSet.Namespace, statefulSet.Name, err)
					continue
				}
				workloads = append(workloads, WorkloadLabelSelectorList{
					Kind:      StatefulSetKind,
					Namespace: statefulSet.Namespace,
					Name:      statefulSet.Name,
					Selector:  selector,
				})
			}
		}
	}

	// List DaemonSets
	daemonSets, err := kubeClient.AppsV1().DaemonSets(targetNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logging.Errorf(ctx, "Could not list daemonsets: %v", err)
	} else {
		for _, daemonSet := range daemonSets.Items {
			if daemonSet.Spec.Selector != nil {
				selector, err := metav1.LabelSelectorAsSelector(daemonSet.Spec.Selector)
				if err != nil {
					logging.Errorf(ctx, "Invalid selector for daemonset %s/%s: %v", daemonSet.Namespace, daemonSet.Name, err)
					continue
				}
				workloads = append(workloads, WorkloadLabelSelectorList{
					Kind:      DaemonSetKind,
					Namespace: daemonSet.Namespace,
					Name:      daemonSet.Name,
					Selector:  selector,
				})
			}
		}
	}

	return workloads, nil
}

// ExtractUniqueNamespaces extracts all unique namespaces from a workload map
func ExtractUniqueNamespaces(workloads map[string]WorkloadInfo) []string {
	namespaceSet := make(map[string]bool)
	for _, workloadInfo := range workloads {
		if workloadInfo.Namespace != "" {
			namespaceSet[workloadInfo.Namespace] = true
		}
	}

	namespaces := make([]string, 0, len(namespaceSet))
	for namespace := range namespaceSet {
		namespaces = append(namespaces, namespace)
	}

	return namespaces
}

// CheckHPAOnCPU checks if workloads are horizontally autoscaled based on CPU metrics
func CheckHPAOnCPU(ctx context.Context, dynamicClient dynamic.Interface, targetNamespace string, workloadHpaCpuMap map[string]bool) error {
	hpaGVR := schema.GroupVersionResource{
		Group:    "autoscaling",
		Version:  "v2",
		Resource: "horizontalpodautoscalers",
	}

	var hpaList *unstructured.UnstructuredList
	var err error

	if targetNamespace == "" {
		hpaList, err = dynamicClient.Resource(hpaGVR).List(ctx, metav1.ListOptions{})
	} else {
		hpaList, err = dynamicClient.Resource(hpaGVR).Namespace(targetNamespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		logging.Errorf(ctx, "Could not list HPAs: %v", err)
		return fmt.Errorf("failed to list HPAs: %w", err)
	}

	for _, hpaUnstructured := range hpaList.Items {
		var hpa autoscalingv2.HorizontalPodAutoscaler
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(hpaUnstructured.Object, &hpa)
		if err != nil {
			logging.Errorf(ctx, "Could not convert HPA %s/%s to structured object: %v",
				hpaUnstructured.GetNamespace(), hpaUnstructured.GetName(), err)
			continue
		}

		hasCPUMetric := false
		for _, metric := range hpa.Spec.Metrics {
			if metric.Type == autoscalingv2.ResourceMetricSourceType &&
				metric.Resource != nil &&
				metric.Resource.Name == corev1.ResourceCPU {
				hasCPUMetric = true
				break
			}
		}

		if !hasCPUMetric {
			continue
		}

		hpaNamespace := hpa.Namespace
		hpaTargetName := hpa.Spec.ScaleTargetRef.Name
		hpaTargetKind := hpa.Spec.ScaleTargetRef.Kind
		hpaTargetAPIVersion := hpa.Spec.ScaleTargetRef.APIVersion

		// Argo Rollouts are treated as Deployments for workload management purposes since they extend Kubernetes Deployment functionality.
		if hpaTargetKind == RolloutKind && hpaTargetAPIVersion == "argoproj.io/v1alpha1" {
			hpaTargetKind = DeploymentKind
		}

		if hpaTargetName != "" && hpaTargetKind != "" {
			targetKey := GetWorkloadKey(hpaTargetKind, hpaNamespace, hpaTargetName)
			if _, exists := workloadHpaCpuMap[targetKey]; exists {
				workloadHpaCpuMap[targetKey] = true
			} else {
				logging.Errorf(ctx, "HPA target %s not found in workload map", targetKey)
			}
		}
	}

	return nil
}

func DetectWorkloadConstraints(ctx context.Context, kubeClient *kubernetes.Clientset, dynamicClient dynamic.Interface, workloadObj WorkloadObject, pdbCache map[string][]policyv1.PodDisruptionBudget) (*WorkloadConstraints, error) {
	constraints := &WorkloadConstraints{}
	if _, ok := workloadObj.(DaemonSetWrapper); ok {
		constraints.Blocking = false
		return constraints, nil
	}

	selector, err := workloadObj.GetSelector()
	if err != nil {
		return nil, fmt.Errorf("error getting workload selector: %w", err)
	}

	constraints.PDB = checkWorkloadAgainstPDBs(ctx, workloadObj.GetNamespace(), selector, pdbCache)

	podTemplate := getPodTemplateSpec(workloadObj)
	if podTemplate != nil {
		constraints.DoNotDisruptAnnotation = checkDoNotDisruptAnnotation(podTemplate)
		constraints.Volume = checkVolumes(podTemplate)
		constraints.Affinity = checkUncommonAffinity(podTemplate)
		constraints.TopologySpreadConstraint = checkTopologySpreadConstraints(podTemplate)
		constraints.PodAntiAffinity = checkPodAntiAffinity(podTemplate)
		constraints.ExcludedAnnotation = checkExcludedAnnotation(podTemplate)
	}

	constraints.Blocking =
		constraints.PDB ||
			constraints.DoNotDisruptAnnotation ||
			constraints.Volume ||
			constraints.Affinity ||
			// constraints.TopologySpreadConstraint ||
			constraints.PodAntiAffinity ||
			constraints.ExcludedAnnotation

	return constraints, nil
}

func FetchPDBsForNamespaces(ctx context.Context, kubeClient *kubernetes.Clientset, namespaces []string) (map[string][]policyv1.PodDisruptionBudget, error) {
	pdbCache := make(map[string][]policyv1.PodDisruptionBudget)

	for _, namespace := range namespaces {
		pdbList, err := kubeClient.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			logging.Errorf(ctx, "Error listing PodDisruptionBudgets for namespace %s: %v", namespace, err)
			continue
		}
		pdbCache[namespace] = pdbList.Items
	}

	return pdbCache, nil
}

func checkWorkloadAgainstPDBs(ctx context.Context, namespace string, workloadSelector labels.Selector, pdbCache map[string][]policyv1.PodDisruptionBudget) bool {
	pdbs, exists := pdbCache[namespace]
	if !exists {
		return false
	}

	for _, pdb := range pdbs {
		if pdb.Spec.Selector == nil {
			continue
		}

		pdbSelector, err := metav1.LabelSelectorAsSelector(pdb.Spec.Selector)
		if err != nil {
			logging.Errorf(ctx, "Error parsing PDB selector for %s: %v", pdb.Name, err)
			continue
		}

		if selectorsMatch(workloadSelector, pdbSelector) {
			return true
		}
	}

	return false
}

func selectorsMatch(workloadSelector, pdbSelector labels.Selector) bool {
	workloadRequirements, workloadSelectable := workloadSelector.Requirements()
	pdbRequirements, pdbSelectable := pdbSelector.Requirements()

	if !workloadSelectable || !pdbSelectable {
		return false
	}

	pdbRequirementsMap := make(map[string]string)
	for _, req := range pdbRequirements {
		if req.Operator() == selection.Equals || req.Operator() == selection.In {
			if len(req.Values().List()) > 0 {
				pdbRequirementsMap[req.Key()] = req.Values().List()[0]
			}
		}
	}

	for _, req := range workloadRequirements {
		if req.Operator() == selection.Equals || req.Operator() == selection.In {
			if len(req.Values().List()) > 0 {
				workloadValue := req.Values().List()[0]
				if pdbValue, exists := pdbRequirementsMap[req.Key()]; exists {
					if workloadValue == pdbValue {
						return true
					}
				}
			}
		}
	}

	return false
}

func getPodTemplateSpec(workloadObj WorkloadObject) *corev1.PodTemplateSpec {
	switch w := workloadObj.(type) {
	case DeploymentWrapper:
		return &w.Spec.Template
	case StatefulSetWrapper:
		return &w.Spec.Template
	case DaemonSetWrapper:
		return &w.Spec.Template
	default:
		return nil
	}
}

func checkExcludedAnnotation(podTemplate *corev1.PodTemplateSpec) bool {
	if podTemplate.Annotations == nil {
		return false
	}
	return podTemplate.Annotations[ExcludedAnnotation] == TrueValue
}

func checkDoNotDisruptAnnotation(podTemplate *corev1.PodTemplateSpec) bool {
	if podTemplate.Annotations == nil {
		return false
	}

	// Check cluster-autoscaler.kubernetes.io/safe-to-evict=false (prevents eviction)
	if value, exists := podTemplate.Annotations["cluster-autoscaler.kubernetes.io/safe-to-evict"]; exists {
		if strings.ToLower(value) == "false" {
			return true
		}
	}

	// Check cruisekube.truefoundry.com/do-not-disrupt=true (prevents disruption)
	if value, exists := podTemplate.Annotations["cruisekube.truefoundry.com/do-not-disrupt"]; exists {
		if strings.ToLower(value) == TrueValue {
			return true
		}
	}

	// Check karpenter.sh/do-not-evict=true (prevents eviction)
	if value, exists := podTemplate.Annotations["karpenter.sh/do-not-evict"]; exists {
		if strings.ToLower(value) == TrueValue {
			return true
		}
	}

	// Check karpenter.sh/do-not-disrupt=true (prevents disruption)
	if value, exists := podTemplate.Annotations["karpenter.sh/do-not-disrupt"]; exists {
		if strings.ToLower(value) == TrueValue {
			return true
		}
	}

	return false
}

func checkVolumes(podTemplate *corev1.PodTemplateSpec) bool {
	if len(podTemplate.Spec.Volumes) == 0 {
		return false
	}

	for _, volume := range podTemplate.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil ||
			volume.HostPath != nil ||
			volume.NFS != nil ||
			volume.Glusterfs != nil ||
			volume.RBD != nil ||
			volume.CephFS != nil ||
			volume.Cinder != nil ||
			volume.FC != nil ||
			volume.FlexVolume != nil ||
			volume.Flocker != nil ||
			volume.AWSElasticBlockStore != nil ||
			volume.GCEPersistentDisk != nil ||
			volume.AzureDisk != nil ||
			volume.AzureFile != nil ||
			volume.VsphereVolume != nil ||
			volume.Quobyte != nil ||
			volume.ISCSI != nil ||
			volume.PhotonPersistentDisk != nil ||
			volume.PortworxVolume != nil ||
			volume.ScaleIO != nil ||
			volume.StorageOS != nil ||
			volume.CSI != nil {
			return true
		}
	}

	return false
}

func checkUncommonAffinity(podTemplate *corev1.PodTemplateSpec) bool {
	if podTemplate.Spec.Affinity == nil {
		return false
	}

	affinity := podTemplate.Spec.Affinity

	if affinity.NodeAffinity != nil {
		if affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
			for _, term := range affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
				for _, expr := range term.MatchExpressions {
					if !isCommonNodeAffinityKey(expr.Key) {
						return true
					}
				}
			}
		}
	}

	return false
}

func isCommonNodeAffinityKey(key string) bool {
	commonKeys := []string{
		"kubernetes.io/arch",
		"kubernetes.io/os",
		"topology.kubernetes.io/region",
		"karpenter.sh/nodepool",
		"class.truefoundry.com/component",
	}

	return slices.Contains(commonKeys, key)
}

func checkTopologySpreadConstraints(podTemplate *corev1.PodTemplateSpec) bool {
	if len(podTemplate.Spec.TopologySpreadConstraints) == 0 {
		return false
	}

	for _, constraint := range podTemplate.Spec.TopologySpreadConstraints {
		if constraint.TopologyKey != "topology.kubernetes.io/zone" {
			return true
		}
	}

	return false
}

func checkPodAntiAffinity(podTemplate *corev1.PodTemplateSpec) bool {
	if podTemplate.Spec.Affinity == nil || podTemplate.Spec.Affinity.PodAntiAffinity == nil {
		return false
	}

	antiAffinity := podTemplate.Spec.Affinity.PodAntiAffinity

	return len(antiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 ||
		len(antiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0
}
