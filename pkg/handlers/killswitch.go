package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/truefoundry/cruiseKube/pkg/cluster"
	"github.com/truefoundry/cruiseKube/pkg/logging"
	"github.com/truefoundry/cruiseKube/pkg/task/utils"
	"github.com/truefoundry/cruiseKube/pkg/types"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func KillswitchHandler(c *gin.Context) {
	ctx := c.Request.Context()
	mgr := c.MustGet("clusterManager").(cluster.Manager)
	clusterID := c.Param("clusterID")

	dryRun := c.Query("dry_run") == "true"
	if dryRun {
		logging.Infof(ctx, "[Dry Run] Starting dry run for cluster %s", clusterID)
	} else {
		logging.Infof(ctx, "Starting killswitch operation for cluster %s", clusterID)
	}

	clients, err := mgr.GetClusterClients(clusterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Unable to fetch cluster clients for cluster %s: %v", clusterID, err),
		})
		return
	}

	response := &types.KillswitchResponse{
		KilledPods: make([]string, 0),
		Errors:     make([]string, 0),
	}

	// Step 1: delete MutatingWebhookConfiguration for this cluster
	if err := deleteMutatingWebhookConfiguration(ctx, clients.KubeClient, clusterID, dryRun); err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to delete MutatingWebhookConfiguration: %v", err))
		response.DeletedMutatingWebhook = false
	} else {
		response.DeletedMutatingWebhook = true
	}

	// Step 2: kill pods with adjusted resources
	podsAnalyzed, podsKilled, killedPods, errors := analyzeAndKillPods(ctx, clients.KubeClient, dryRun)
	response.PodsAnalyzed = podsAnalyzed
	response.PodsKilled = podsKilled
	response.KilledPods = append(response.KilledPods, killedPods...)
	response.Errors = append(response.Errors, errors...)

	response.Message = fmt.Sprintf("Killswitch completed. MutatingWebhookConfiguration deleted: %v, Pods analyzed: %d, Pods killed: %d",
		response.DeletedMutatingWebhook, response.PodsAnalyzed, response.PodsKilled)

	logging.Infof(ctx, "%s", response.Message)
	c.JSON(http.StatusOK, response)
}

func deleteMutatingWebhookConfiguration(ctx context.Context, kubeClient *kubernetes.Clientset, clusterID string, dryRun bool) error {
	name := fmt.Sprintf("cruiseKube-resource-adjuster-%s", clusterID)

	if dryRun {
		_, err := kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				logging.Infof(ctx, "[Dry Run] MutatingWebhookConfiguration %s not found; nothing to delete", name)
				return nil
			}
			return fmt.Errorf("[Dry Run] failed to get MutatingWebhookConfiguration %s: %w", name, err)
		}
		logging.Infof(ctx, "[Dry Run] MutatingWebhookConfiguration %s found, skipping delete", name)
		return nil
	}

	err := kubeClient.AdmissionregistrationV1().MutatingWebhookConfigurations().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logging.Warnf(ctx, "MutatingWebhookConfiguration %s not found; nothing to delete", name)
			return nil
		}
		return fmt.Errorf("failed to delete MutatingWebhookConfiguration %s: %w", name, err)
	}

	logging.Infof(ctx, "Successfully deleted MutatingWebhookConfiguration %s", name)
	return nil
}

func analyzeAndKillPods(ctx context.Context, kubeClient *kubernetes.Clientset, dryRun bool) (int, int, []string, []string) {
	var errors []string
	var killedPods []string
	podsAnalyzed := 0
	podsKilled := 0

	podToWorkloadMap, allPods, err := utils.BuildPodToWorkloadMapping(ctx, kubeClient, "")
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to build pod-to-workload mapping: %v", err))
		return 0, 0, killedPods, errors
	}

	logging.Infof(ctx, "Analyzing %d pods for resource differences", len(allPods))

	for _, pod := range allPods {
		podsAnalyzed++

		podKey := utils.PodKey{
			Namespace: pod.Namespace,
			PodName:   pod.Name,
		}

		workloadInfo, exists := podToWorkloadMap[podKey]
		if !exists {
			continue
		}

		workloadObj, err := utils.GetWorkloadObject(ctx, kubeClient, workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to get workload %s/%s/%s: %v",
				workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name, err))
			continue
		}

		hasResourceDiff, reason := comparePodWithWorkload(ctx, kubeClient, pod, workloadObj)
		if hasResourceDiff {
			var success bool
			var evictErr string

			if !dryRun {
				success, evictErr = utils.EvictPod(ctx, kubeClient, pod)
			} else {
				success = true
				evictErr = ""
			}

			if success {
				podsKilled++
				killedPodName := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
				killedPods = append(killedPods, killedPodName)
				logging.Infof(ctx, "Killed pod %s (reason: %s)", killedPodName, reason)
			} else {
				errors = append(errors, fmt.Sprintf("Failed to kill pod %s/%s: %s", pod.Namespace, pod.Name, evictErr))
			}
		}
	}

	return podsAnalyzed, podsKilled, killedPods, errors
}

func comparePodWithWorkload(ctx context.Context, kubeClient *kubernetes.Clientset, pod *corev1.Pod, workload utils.WorkloadObject) (bool, string) {
	workloadContainers := workload.GetContainerSpecs(ctx, kubeClient)
	workloadInitContainers := workload.GetInitContainerSpecs(ctx, kubeClient)

	allContainers := make([]corev1.Container, 0, len(workloadContainers)+len(workloadInitContainers))
	allContainers = append(allContainers, workloadContainers...)
	allContainers = append(allContainers, workloadInitContainers...)

	workloadContainerMap := make(map[string]corev1.Container)
	for _, container := range allContainers {
		workloadContainerMap[container.Name] = container
	}

	for _, podContainer := range pod.Spec.Containers {
		workloadContainer, exists := workloadContainerMap[podContainer.Name]
		if !exists {
			return true, fmt.Sprintf("container %s exists in pod but not in workload template", podContainer.Name)
		}

		if !resourcesEqual(podContainer.Resources.Requests.Cpu(), workloadContainer.Resources.Requests.Cpu()) {
			return true, fmt.Sprintf("container %s has different CPU request", podContainer.Name)
		}
		if !resourcesEqual(podContainer.Resources.Limits.Cpu(), workloadContainer.Resources.Limits.Cpu()) {
			return true, fmt.Sprintf("container %s has different CPU limit", podContainer.Name)
		}

		if !resourcesEqual(podContainer.Resources.Requests.Memory(), workloadContainer.Resources.Requests.Memory()) {
			return true, fmt.Sprintf("container %s has different memory request", podContainer.Name)
		}
		if !resourcesEqual(podContainer.Resources.Limits.Memory(), workloadContainer.Resources.Limits.Memory()) {
			return true, fmt.Sprintf("container %s has different memory limit", podContainer.Name)
		}
	}

	return false, "resources match workload template"
}

func resourcesEqual(podResource *resource.Quantity, workloadResource *resource.Quantity) bool {
	if podResource == nil && workloadResource == nil {
		return true
	}
	if podResource == nil || workloadResource == nil {
		return false
	}

	return podResource.Equal(*workloadResource)
}
