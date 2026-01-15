package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/truefoundry/cruisekube/pkg/client"
	"github.com/truefoundry/cruisekube/pkg/config"
	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/task/utils"

	"github.com/gin-gonic/gin"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	excludedPodPrefixes = []string{}
)

func MutateHandler(c *gin.Context) {
	ctx := c.Request.Context()
	clusterID := c.Param("clusterID")

	body, err := c.GetRawData()
	if err != nil {
		logging.Errorf(ctx, "Failed to read request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		logging.Errorf(ctx, "Failed to unmarshal admission review: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	req := review.Request
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		logging.Errorf(ctx, "Failed to unmarshal pod object: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	logging.Infof(ctx, "Processing pod: %s/%s", req.Namespace, getPodName(&pod))

	cfg := config.GetConfigFromGinContext(c)
	allowed := true
	var patches []map[string]any

	if shouldProcessPod(ctx, &pod, req.Namespace, cfg.RecommendationSettings.ApplyBlacklistedNamespaces) {
		patches, err = adjustResources(ctx, &pod, clusterID, cfg)
		if err != nil {
			logging.Errorf(ctx, "Failed to adjust resources: %v", err)
		}
	} else {
		logging.Infof(ctx, "Skipping pod %s/%s", req.Namespace, getPodName(&pod))
		return
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		logging.Errorf(ctx, "Failed to marshal patches: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	patchType := admissionv1.PatchTypeJSONPatch
	reviewResponse := &admissionv1.AdmissionResponse{
		UID:       req.UID,
		Allowed:   allowed,
		PatchType: &patchType,
		Patch:     patchBytes,
	}

	review.Response = reviewResponse
	logging.Infof(ctx, "Review response for pod %s/%s: %v", req.Namespace, getPodName(&pod), string(patchBytes))

	c.JSON(http.StatusOK, review)
}

func shouldProcessPod(ctx context.Context, pod *corev1.Pod, namespace string, applyBlacklistedNamespaces []string) bool {
	podName := getPodName(pod)

	if slices.Contains(applyBlacklistedNamespaces, namespace) {
		logging.Infof(ctx, "Skipping pod in blacklisted namespace: %s/%s", namespace, podName)
		return false
	}

	for _, prefix := range excludedPodPrefixes {
		if strings.HasPrefix(podName, prefix) {
			logging.Infof(ctx, "Skipping pod with excluded prefix: %s/%s", namespace, podName)
			return false
		}
	}
	if pod.Annotations[utils.ExcludedAnnotation] == "true" {
		logging.Infof(ctx, "Skipping pod with excluded annotation: %s/%s", namespace, podName)
		return false
	}

	return true
}

func getPodName(pod *corev1.Pod) string {
	if pod.Name != "" {
		return pod.Name
	}
	if pod.GenerateName != "" {
		return pod.GenerateName
	}
	return "unknown"
}

func findWorkloadStat(stats *utils.StatsResponse, workloadInfo *utils.WorkloadInfo) *utils.WorkloadStat {
	identifier := utils.GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)

	for _, stat := range stats.Stats {
		if stat.WorkloadIdentifier == identifier {
			return &stat
		}
	}

	return nil
}

func cpuCoresToMillicores(cpuCores float64) string {
	return fmt.Sprintf("%dm", int64(cpuCores*1000))
}

func memoryBytesToMB(memoryBytes int64) string {
	return fmt.Sprintf("%dM", memoryBytes/(utils.BytesToMBDivisor))
}

//nolint:unparam // error return is part of the interface contract even though currently always nil
func adjustResources(ctx context.Context, pod *corev1.Pod, clusterID string, cfg *config.Config) ([]map[string]any, error) {
	workloadInfo := utils.GetWorkloadInfoFromPod(pod)
	if workloadInfo == nil {
		logging.Warnf(ctx, "Could not determine workload for pod %s/%s, allowing without adjustment", pod.Namespace, getPodName(pod))
		return []map[string]any{}, nil
	}

	logging.Infof(ctx, "Pod %s/%s belongs to workload: %s", pod.Namespace, getPodName(pod), utils.GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name))

	workloadID := strings.ReplaceAll(utils.GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name), "/", ":")

	recommenderClient := client.NewRecommenderServiceClientWithClusterToken(
		cfg.Webhook.StatsURL.Host,
		cfg.Webhook.StatsURL.TfyClusterToken,
	)

	workloadOverrides, err := recommenderClient.WebhookGetWorkloadOverrides(ctx, clusterID, workloadID)
	if err != nil {
		logging.Errorf(ctx, "Could not fetch overrides, allowing pod without adjustment: %v", err)
		return []map[string]any{}, nil
	}

	if workloadOverrides.Enabled != nil && !*workloadOverrides.Enabled {
		logging.Infof(ctx, "Workload %s is disabled via overrides, skipping", workloadID)
		return []map[string]any{}, nil
	}

	statsResponse, err := recommenderClient.WebhookGetClusterStats(ctx, clusterID)
	if err != nil {
		logging.Errorf(ctx, "Could not fetch stats, allowing pod without adjustment: %v", err)
		return []map[string]any{}, nil
	}
	if len(statsResponse.Stats) == 0 {
		logging.Infof(ctx, "No stats found for workload %s, allowing pod without adjustment", workloadID)
		return []map[string]any{}, nil
	}

	workloadStat := findWorkloadStat(statsResponse, workloadInfo)
	if workloadStat == nil {
		logging.Infof(ctx, "No stat found for workload %s, allowing pod without adjustment", workloadID)
		return []map[string]any{}, nil
	}

	if workloadStat.IsHorizontallyAutoscaledOnCPU {
		logging.Infof(ctx, "Workload %s is horizontally autoscaled on CPU, skipping", workloadID)
		return []map[string]any{}, nil
	}

	if workloadStat.CreationTime.After(time.Now().Add(-1 * time.Hour * time.Duration(cfg.RecommendationSettings.NewWorkloadThresholdHours))) {
		logging.Infof(ctx, "Workload %s is from a new workload, skipping", workloadID)
		return []map[string]any{}, nil
	}

	containers := make([]corev1.Container, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers))
	containers = append(containers, pod.Spec.Containers...)
	containers = append(containers, pod.Spec.InitContainers...)
	var patches []map[string]any
	for i, container := range containers {
		containerPath := fmt.Sprintf("/spec/containers/%d", i)
		if i >= len(pod.Spec.Containers) {
			containerPath = fmt.Sprintf("/spec/initContainers/%d", i-len(pod.Spec.Containers))
		}

		if container.Resources.Requests == nil {
			patches = append(patches, map[string]any{
				"op":    "add",
				"path":  containerPath + "/resources/requests",
				"value": map[string]string{},
			})
		}
		if container.Resources.Limits == nil {
			patches = append(patches, map[string]any{
				"op":    "add",
				"path":  containerPath + "/resources/limits",
				"value": map[string]string{},
			})
		}

		if container.Resources.Limits != nil {
			if _, exists := container.Resources.Limits[corev1.ResourceCPU]; exists {
				patches = append(patches, map[string]any{
					"op":   "remove",
					"path": containerPath + "/resources/limits/cpu",
				})
			}
		}

		var containerStat *utils.ContainerStats
		for _, stat := range workloadStat.ContainerStats {
			if stat.ContainerName == container.Name {
				containerStat = &stat
				break
			}
		}

		if containerStat == nil || containerStat.CPUStats == nil || containerStat.MemoryStats == nil || containerStat.SimplePredictionsCPU == nil || containerStat.SimplePredictionsMemory == nil {
			logging.Infof(ctx, "No stat found for container: %s in workload: %s/%s/%s", container.Name, workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)
			continue
		}

		recommendedCPU := containerStat.CPUStats.Max
		if containerStat.SimplePredictionsCPU != nil && containerStat.SimplePredictionsCPU.MaxValue > 0 {
			recommendedCPU = containerStat.SimplePredictionsCPU.MaxValue
		}

		recommendedMemory := containerStat.MemoryStats.Max
		if containerStat.MemoryStats.OOMMemory > 0 && containerStat.MemoryStats.OOMMemory > containerStat.MemoryStats.Max {
			recommendedMemory = containerStat.MemoryStats.OOMMemory
		} else if containerStat.SimplePredictionsMemory != nil && containerStat.SimplePredictionsMemory.MaxValue > 0 {
			recommendedMemory = containerStat.SimplePredictionsMemory.MaxValue
		}

		recommendedMemoryLimit := 2 * recommendedMemory
		if containerStat.Memory7Day != nil && containerStat.Memory7Day.Max > 0 {
			recommendedMemoryLimit = math.Max(2*containerStat.Memory7Day.Max, max(2*containerStat.MemoryStats.OOMMemory, recommendedMemoryLimit))
		}

		recommendedMemoryLimitBytes := int64(math.Max(recommendedMemoryLimit, 512) * utils.BytesToMBDivisor)

		logging.Infof(ctx, "Container %s - Recommended CPU: %s (max: %f)", container.Name, cpuCoresToMillicores(recommendedCPU), containerStat.CPUStats.Max)
		logging.Infof(ctx, "Container %s - Recommended Memory: %s", container.Name, memoryBytesToMB(int64(recommendedMemory*utils.BytesToMBDivisor)))

		var currentCPURequest resource.Quantity
		var currentMemoryRequest resource.Quantity
		var currentMemoryLimit resource.Quantity

		if container.Resources.Requests != nil {
			currentCPURequest = container.Resources.Requests[corev1.ResourceCPU]
			currentMemoryRequest = container.Resources.Requests[corev1.ResourceMemory]
		}

		if container.Resources.Limits != nil {
			currentMemoryLimit = container.Resources.Limits[corev1.ResourceMemory]
		}

		// CPU
		currentCPUMillicores := currentCPURequest.MilliValue()
		recommendedCPUMillicores := math.Max(float64(recommendedCPU*1000), 1)

		if currentCPUMillicores > 0 {
			if workloadInfo.Kind != utils.DaemonSetKind {
				patches = append(patches, map[string]any{
					"op":    "replace",
					"path":  containerPath + "/resources/requests/cpu",
					"value": fmt.Sprintf("%dm", int64(recommendedCPUMillicores)),
				})
				logging.Infof(ctx, "Adjusted CPU for container %s from %dm to %dm", container.Name, currentCPUMillicores, int64(recommendedCPUMillicores))
			}
		}

		// Memory
		currentMemoryBytes := currentMemoryRequest.Value()
		recommendedMemoryBytes := int64(recommendedMemory * utils.BytesToMBDivisor)
		thresholdBytes := float64(16 * utils.BytesToMBDivisor)

		if !cfg.RecommendationSettings.DisableMemoryApplication && currentMemoryBytes > 0 && math.Abs(float64(recommendedMemoryBytes-currentMemoryBytes)) > thresholdBytes {
			if workloadInfo.Kind == utils.DaemonSetKind {
				if currentMemoryLimit.Value() > 0 {
					currentMemoryLimitBytes := currentMemoryLimit.Value()
					finalMemoryLimitBytes := math.Max(float64(currentMemoryLimitBytes), math.Max(float64(recommendedMemoryLimitBytes), 16*utils.BytesToMBDivisor))

					patches = append(patches, map[string]any{
						"op":    "replace",
						"path":  containerPath + "/resources/limits/memory",
						"value": memoryBytesToMB(int64(finalMemoryLimitBytes)),
					})

					logging.Infof(ctx, "Adjusted Memory limit only for DaemonSet container %s to %s (request unchanged: %dMB)", container.Name, memoryBytesToMB(int64(finalMemoryLimitBytes)), currentMemoryRequest.Value()/utils.BytesToMBDivisor)
				}
			} else {
				if currentMemoryRequest.Value() > 0 {
					patches = append(patches, map[string]any{
						"op":    "replace",
						"path":  containerPath + "/resources/requests/memory",
						"value": memoryBytesToMB(recommendedMemoryBytes),
					})
				} else {
					patches = append(patches, map[string]any{
						"op":    "add",
						"path":  containerPath + "/resources/requests/memory",
						"value": memoryBytesToMB(recommendedMemoryBytes),
					})
				}
				if currentMemoryLimit.Value() > 0 {
					patches = append(patches, map[string]any{
						"op":    "replace",
						"path":  containerPath + "/resources/limits/memory",
						"value": memoryBytesToMB(recommendedMemoryLimitBytes),
					})
				} else {
					patches = append(patches, map[string]any{
						"op":    "add",
						"path":  containerPath + "/resources/limits/memory",
						"value": memoryBytesToMB(recommendedMemoryLimitBytes),
					})
				}

				logging.Infof(ctx, "Adjusted Memory for container %s from %dMB to %dMB", container.Name, currentMemoryRequest.Value()/utils.BytesToMBDivisor, recommendedMemoryBytes/utils.BytesToMBDivisor)
			}
		} else if cfg.RecommendationSettings.DisableMemoryApplication {
			logging.Infof(ctx, "Skipping memory recommendation application for container %s since memory recommendationapplication is disabled", container.Name)
		}
	}

	if cfg.Webhook.DryRun {
		logging.Infof(ctx, "Dry run mode enabled, skipping applying patches")
		return []map[string]any{}, nil
	}

	return patches, nil
}
