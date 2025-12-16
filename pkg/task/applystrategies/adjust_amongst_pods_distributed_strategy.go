package applystrategies

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/task/utils"
	"github.com/truefoundry/cruisekube/pkg/types"

	"k8s.io/client-go/kubernetes"
)

func NewAdjustAmongstPodsDistributedStrategy(ctx context.Context) utils.OptimizationStrategy {
	return &AdjustAmongstPodsDistributedStrategy{}
}

type AdjustAmongstPodsDistributedStrategy struct {
}

func (s *AdjustAmongstPodsDistributedStrategy) GetName() string {
	return "AdjustAmongstPodsDistributed"
}

func (s *AdjustAmongstPodsDistributedStrategy) OptimizeNode(kubeClient *kubernetes.Clientset, overridesMap map[string]*types.WorkloadOverrideInfo, data utils.NodeOptimizationData) (utils.OptimizationResult, error) {
	result := utils.OptimizationResult{
		PodContainerRecommendations: make([]utils.PodContainerRecommendation, 0),
		MaxRestCPU:                  0,
		MaxRestMemory:               0,
	}

	podInfosClone := make([]utils.PodInfo, len(data.PodInfos))
	copy(podInfosClone, data.PodInfos)

	podMetricsCache := make(map[string]utils.PodMetrics)

	// Calculate the recommendation for each pod
	for _, podInfo := range podInfosClone {
		podKey := utils.GetPodKey(podInfo.Namespace, podInfo.Name)
		var totalRecommendedCPU, totalRecommendedMemory, maxRestCPU, maxRestMemory float64
		for _, containerRec := range podInfo.Stats.ContainerStats {
			if containerRec.ContainerType == types.InitContainer {
				continue
			}
			recommendedCPU, cpuRest, err := s.calculateForSpecificPercentile(podInfo, containerRec)
			if err != nil {
				logging.Errorf(context.Background(), "Error calculating for specific percentile for container %s: %v", containerRec.ContainerName, err)
				continue
			}
			recommendedMemory, memoryRest := s.getRecommendedAndRestMemory(containerRec)

			if podInfo.WorkloadKind == utils.DaemonSetKind {
				containerResource, err := podInfo.GetContainerResource(containerRec.ContainerName)
				if err != nil {
					logging.Errorf(context.Background(), "Error getting container resource for container %s: %v", containerRec.ContainerName, err)
					continue
				}
				currentCPU := containerResource.CPURequest
				currentMemory := containerResource.MemoryRequest

				if recommendedCPU > currentCPU {
					// We don't want to increase the CPU request for daemonsets
					// as they might end up in a state that not all daemonsets will fit in a single node
					recommendedCPU = currentCPU
				}
				cpuRest = 0

				if recommendedMemory > currentMemory {
					recommendedMemory = currentMemory
				}
				memoryRest = 0
			}
			totalRecommendedCPU += recommendedCPU
			maxRestCPU = math.Max(maxRestCPU, cpuRest)
			totalRecommendedMemory += recommendedMemory
			maxRestMemory = math.Max(maxRestMemory, memoryRest)
		}

		var maxInitCPU, maxInitMemory float64
		if podInfo.Stats != nil && podInfo.Stats.OriginalContainerResources != nil {
			for _, containerRes := range podInfo.Stats.OriginalContainerResources {
				if containerRes.Type == types.InitContainer {
					maxInitCPU = max(maxInitCPU, containerRes.CPURequest)
					maxInitMemory = max(maxInitMemory, containerRes.MemoryRequest)
				}
			}
		}

		evictionRanking := types.EvictionRankingHigh
		if podInfo.Stats.EvictionRanking != 0 {
			evictionRanking = podInfo.Stats.EvictionRanking
		}
		overrides, ok := overridesMap[podInfo.Stats.WorkloadIdentifier]
		if ok && overrides.EvictionRanking != 0 {
			evictionRanking = overrides.EvictionRanking
		}

		podMetricsCache[podKey] = utils.PodMetrics{
			TotalRecommendedCPU:    max(totalRecommendedCPU, maxInitCPU),
			TotalRecommendedMemory: max(totalRecommendedMemory, maxInitMemory),
			MaxRestCPU:             maxRestCPU,
			MaxRestMemory:          maxRestMemory,
			EvictionRanking:        evictionRanking,
		}
	}

	// Sort and perform eviction based on memory
	sort.Slice(podInfosClone, func(i, j int) bool {
		if podInfosClone[i].WorkloadKind == utils.DaemonSetKind && podInfosClone[j].WorkloadKind != utils.DaemonSetKind {
			return false
		}
		if podInfosClone[i].WorkloadKind != utils.DaemonSetKind && podInfosClone[j].WorkloadKind == utils.DaemonSetKind {
			return true
		}
		podKey_i := utils.GetPodKey(podInfosClone[i].Namespace, podInfosClone[i].Name)
		podKey_j := utils.GetPodKey(podInfosClone[j].Namespace, podInfosClone[j].Name)
		metrics_i := podMetricsCache[podKey_i]
		metrics_j := podMetricsCache[podKey_j]

		if metrics_i.EvictionRanking < metrics_j.EvictionRanking {
			return false
		}
		if metrics_i.EvictionRanking > metrics_j.EvictionRanking {
			return true
		}
		return metrics_i.MaxRestMemory > metrics_j.MaxRestMemory
	})
	podInfosClone = s.performEvictionLoop(podInfosClone, podMetricsCache, data.AllocatableMemory,
		func(metrics utils.PodMetrics) float64 { return metrics.TotalRecommendedMemory },
		func(metrics utils.PodMetrics) float64 { return metrics.MaxRestMemory },
		&result)

	// Sort and perform eviction based on CPU
	sort.Slice(podInfosClone, func(i, j int) bool {
		if podInfosClone[i].WorkloadKind == utils.DaemonSetKind && podInfosClone[j].WorkloadKind != utils.DaemonSetKind {
			return false
		}
		if podInfosClone[i].WorkloadKind != utils.DaemonSetKind && podInfosClone[j].WorkloadKind == utils.DaemonSetKind {
			return true
		}

		podKey_i := utils.GetPodKey(podInfosClone[i].Namespace, podInfosClone[i].Name)
		podKey_j := utils.GetPodKey(podInfosClone[j].Namespace, podInfosClone[j].Name)

		metrics_i := podMetricsCache[podKey_i]
		metrics_j := podMetricsCache[podKey_j]

		if metrics_i.EvictionRanking < metrics_j.EvictionRanking {
			return false
		}
		if metrics_i.EvictionRanking > metrics_j.EvictionRanking {
			return true
		}

		return metrics_i.MaxRestCPU > metrics_j.MaxRestCPU
	})
	podInfosClone = s.performEvictionLoop(podInfosClone, podMetricsCache, data.AllocatableCPU,
		func(metrics utils.PodMetrics) float64 { return metrics.TotalRecommendedCPU },
		func(metrics utils.PodMetrics) float64 { return metrics.MaxRestCPU },
		&result)

	maxRestCPU := 0.0
	maxRestMemory := 0.0
	totalRestCPU := 0.0
	totalRestMemory := 0.0
	totalRecommendedCPU := 0.0
	totalRecommendedMemory := 0.0

	containerMetrics := make([]struct {
		pod               utils.PodInfo
		containerStats    utils.ContainerStats
		currentCPU        float64
		currentMemory     float64
		restCPU           float64
		restMemory        float64
		recommendedCPU    float64
		recommendedMemory float64
	}, 0)

	for _, pod := range podInfosClone {
		if pod.Stats.ContainerStats != nil {
			for _, containerStat := range pod.Stats.ContainerStats {
				if containerStat.ContainerType == types.InitContainer {
					continue
				}
				currentResource, err := pod.GetContainerResource(containerStat.ContainerName)
				if err != nil {
					logging.Errorf(context.Background(), "Error getting container resource for container %s: %v", containerStat.ContainerName, err)
					continue
				}
				currentCPU := currentResource.CPURequest
				currentMemory := currentResource.MemoryRequest

				recommendedCPU, restCPU, err := s.calculateForSpecificPercentile(pod, containerStat)
				if err != nil {
					logging.Errorf(context.Background(), "Error calculating for specific percentile for container %s: %v", containerStat.ContainerName, err)
					continue
				}
				recommendedMemory, restMemory := s.getRecommendedAndRestMemory(containerStat)

				containerMetrics = append(containerMetrics, struct {
					pod               utils.PodInfo
					containerStats    utils.ContainerStats
					currentCPU        float64
					currentMemory     float64
					restCPU           float64
					restMemory        float64
					recommendedCPU    float64
					recommendedMemory float64
				}{
					pod:               pod,
					containerStats:    containerStat,
					currentCPU:        currentCPU,
					currentMemory:     currentMemory,
					restCPU:           restCPU,
					restMemory:        restMemory,
					recommendedCPU:    recommendedCPU,
					recommendedMemory: recommendedMemory,
				})

				maxRestCPU = math.Max(maxRestCPU, restCPU)
				maxRestMemory = math.Max(maxRestMemory, restMemory)
				totalRestCPU += restCPU
				totalRestMemory += restMemory
				totalRecommendedCPU += recommendedCPU
				totalRecommendedMemory += recommendedMemory
			}
		}
	}

	// s.logNodeMaxRestResources(podMetricsCache, data.NodeName)

	for _, metric := range containerMetrics {
		var additionalCPU, additionalMemory float64

		if totalRecommendedCPU > 0 {
			cpuRatio := metric.restCPU / totalRestCPU
			additionalCPU = maxRestCPU * cpuRatio
		}

		if totalRecommendedMemory > 0 {
			memoryRatio := metric.restMemory / totalRestMemory
			additionalMemory = maxRestMemory * memoryRatio
		}

		finalCPU := metric.recommendedCPU + additionalCPU
		finalMemory := metric.recommendedMemory + additionalMemory

		logging.Infof(context.Background(), "Distributed strategy for %s/%s/%s: base_cpu=%.3f, additional_cpu=%.3f, final_cpu=%.3f, base_memory=%.3f, additional_memory=%.3f, final_memory=%.3f",
			metric.pod.Namespace, metric.pod.Name, metric.containerStats.ContainerName,
			metric.recommendedCPU, additionalCPU, finalCPU, metric.recommendedMemory, additionalMemory, finalMemory)

		podContainerRec := utils.PodContainerRecommendation{
			PodInfo:       metric.pod,
			ContainerName: metric.containerStats.ContainerName,
			CPU:           finalCPU,
			Memory:        finalMemory,
			Evict:         false,
		}
		result.PodContainerRecommendations = append(result.PodContainerRecommendations, podContainerRec)
	}

	for _, pod := range podInfosClone {
		if pod.Stats.ContainerStats == nil {
			logging.Errorf(context.Background(), "No container recommendations found for pod %s/%s", pod.Namespace, pod.Name)
		}
	}

	result.MaxRestCPU = maxRestCPU
	result.MaxRestMemory = maxRestMemory

	return result, nil
}

func (s *AdjustAmongstPodsDistributedStrategy) performEvictionLoop(
	podInfosClone []utils.PodInfo,
	podMetricsCache map[string]utils.PodMetrics,
	allocatableResource float64,
	getTotalRecommended func(utils.PodMetrics) float64,
	getMaxRest func(utils.PodMetrics) float64,
	result *utils.OptimizationResult,
) []utils.PodInfo {
	i := 0
	for i < len(podInfosClone) {
		idealConsumption := 0.0
		idealRequiredRest := 0.0
		for _, p := range podInfosClone {
			podKey := utils.GetPodKey(p.Namespace, p.Name)
			metrics := podMetricsCache[podKey]
			idealConsumption += getTotalRecommended(metrics)
			idealRequiredRest = math.Max(getMaxRest(metrics), idealRequiredRest)
		}

		spare := allocatableResource - idealConsumption
		if idealConsumption <= allocatableResource && idealRequiredRest <= spare {
			break
		}

		podInfo := podInfosClone[i]
		evictionRanking := podMetricsCache[utils.GetPodKey(podInfo.Namespace, podInfo.Name)].EvictionRanking

		if isEvictionExcludedPod(&podInfo, evictionRanking) {
			i++
			continue
		}

		for _, containerStat := range podInfo.Stats.ContainerStats {
			if containerStat.ContainerType == types.InitContainer {
				continue
			}
			result.PodContainerRecommendations = append(result.PodContainerRecommendations, utils.PodContainerRecommendation{
				PodInfo:       podInfo,
				ContainerName: containerStat.ContainerName,
				CPU:           containerStat.SimplePredictionsCPU.MaxValue,
				Memory:        containerStat.SimplePredictionsMemory.MaxValue,
				Evict:         true,
			})
		}

		podInfosClone = append(podInfosClone[:i], podInfosClone[i+1:]...)
	}
	return podInfosClone
}

// func (s *AdjustAmongstPodsDistributedStrategy) logNodeMaxRestResources(podMetricsCache map[string]utils.PodMetrics, nodeName string) {
// 	maxRestCPU := 0.0
// 	maxRestMemory := 0.0
// 	maxRestCPUPod := ""
// 	maxRestMemoryPod := ""

// 	for podKey, metrics := range podMetricsCache {
// 		if metrics.MaxRestCPU > maxRestCPU {
// 			maxRestCPU = metrics.MaxRestCPU
// 			maxRestCPUPod = podKey
// 		}
// 		if metrics.MaxRestMemory > maxRestMemory {
// 			maxRestMemory = metrics.MaxRestMemory
// 			maxRestMemoryPod = podKey
// 		}
// 	}

// 	s.logMaxRestCPU(nodeName, maxRestCPU, maxRestCPUPod)
// 	s.logMaxRestMemory(nodeName, maxRestMemory, maxRestMemoryPod)
// }

// func (s *AdjustAmongstPodsDistributedStrategy) logMaxRestCPU(nodeName string, maxRestCPU float64, maxRestCPUPod string) {
// 	file, err := os.OpenFile("node_max_rest_cpu.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
// 	if err != nil {
// 		logging.Errorf(context.Background(), "Error opening CPU CSV file: %v", err)
// 		return
// 	}
// 	defer func() { _ = file.Close() }()

// 	fileInfo, err := file.Stat()
// 	if err != nil {
// 		logging.Errorf(context.Background(), "Error getting CPU file info: %v", err)
// 		return
// 	}

// 	if fileInfo.Size() == 0 {
// 		header := "timestamp,node_name,max_rest_cpu,max_rest_cpu_pod\n"
// 		if _, err := file.WriteString(header); err != nil {
// 			logging.Errorf(context.Background(), "Error writing CPU CSV header: %v", err)
// 			return
// 		}
// 	}

// 	csvLine := fmt.Sprintf("%s,%s,%.3f,%s\n",
// 		time.Now().Format("2006-01-02 15:04:05"), nodeName, maxRestCPU, maxRestCPUPod)

// 	if _, err := file.WriteString(csvLine); err != nil {
// 		logging.Errorf(context.Background(), "Error writing to CPU CSV file: %v", err)
// 	}
// }

// func (s *AdjustAmongstPodsDistributedStrategy) logMaxRestMemory(nodeName string, maxRestMemory float64, maxRestMemoryPod string) {
// 	file, err := os.OpenFile("node_max_rest_memory.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
// 	if err != nil {
// 		logging.Errorf(context.Background(), "Error opening Memory CSV file: %v", err)
// 		return
// 	}
// 	defer func() { _ = file.Close() }()

// 	fileInfo, err := file.Stat()
// 	if err != nil {
// 		logging.Errorf(context.Background(), "Error getting Memory file info: %v", err)
// 		return
// 	}

// 	if fileInfo.Size() == 0 {
// 		header := "timestamp,node_name,max_rest_memory,max_rest_memory_pod\n"
// 		if _, err := file.WriteString(header); err != nil {
// 			logging.Errorf(context.Background(), "Error writing Memory CSV header: %v", err)
// 			return
// 		}
// 	}

// 	csvLine := fmt.Sprintf("%s,%s,%.3f,%s\n",
// 		time.Now().Format("2006-01-02 15:04:05"), nodeName, maxRestMemory, maxRestMemoryPod)

// 	if _, err := file.WriteString(csvLine); err != nil {
// 		logging.Errorf(context.Background(), "Error writing to Memory CSV file: %v", err)
// 	}
// }

func (s *AdjustAmongstPodsDistributedStrategy) getPmax(containerRec utils.ContainerStats) (float64, error) {
	if containerRec.SimplePredictionsCPU != nil {
		return containerRec.SimplePredictionsCPU.MaxValue, nil
	} else {
		return 0.0, fmt.Errorf("no simple predictions found for container %s", containerRec.ContainerName)
	}
}

func (s *AdjustAmongstPodsDistributedStrategy) getRecommendedAndRestMemory(containerRec utils.ContainerStats) (float64, float64) {
	if containerRec.MemoryStats.OOMMemory > 0 && containerRec.MemoryStats.OOMMemory > containerRec.MemoryStats.P75 {
		logging.Infof(context.Background(), "Using OOM memory for container %s: %v", containerRec.ContainerName, containerRec.MemoryStats.OOMMemory)
		// We are not underestimating the memory requests here, we are just using the OOM memory as the total recommended memory with no extra headspace
		return containerRec.MemoryStats.OOMMemory, 0.0
	}
	if containerRec.SimplePredictionsMemory == nil {
		logging.Errorf(context.Background(), "Error: No simple predictions found for container %s", containerRec.ContainerName)
		return containerRec.MemoryStats.P75, containerRec.MemoryStats.Max - containerRec.MemoryStats.P75
	} else {
		return containerRec.MemoryStats.P75, containerRec.SimplePredictionsMemory.MaxValue - containerRec.MemoryStats.P75
	}
}

func (s *AdjustAmongstPodsDistributedStrategy) calculateForSpecificPercentile(pod utils.PodInfo, containerRec utils.ContainerStats) (float64, float64, error) {
	x := 75.0
	px := containerRec.CPUStats.P75
	if containerRec.PSIAdjustedUsage != nil {
		px = containerRec.PSIAdjustedUsage.P75
	}
	pmax, err := s.getPmax(containerRec)
	if err != nil {
		return 0.0, 0.0, fmt.Errorf("error getting pmax for container %s: %w", containerRec.ContainerName, err)
	}

	request := px

	rest := (pmax - px)

	logging.Infof(context.Background(), "Variable diff calculation for %s/%s/%s: x=%.1f, px=%.3f, pmax=%.3f, request=%.3f, rest=%.3f",
		pod.Namespace, pod.Name, containerRec.ContainerName,
		x, px, pmax, request, rest)

	return request, rest, nil
}
