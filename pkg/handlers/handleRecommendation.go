package handlers

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/truefoundry/cruisekube/pkg/cluster"
	"github.com/truefoundry/cruisekube/pkg/config"
	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/task"
	"github.com/truefoundry/cruisekube/pkg/task/applystrategies"
	"github.com/truefoundry/cruisekube/pkg/task/utils"
	"github.com/truefoundry/cruisekube/pkg/types"
)

func RecommendationAnalysisHandlerForCluster(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	clusterID := c.Param("clusterID")
	mgr := c.MustGet("clusterManager").(cluster.Manager)
	response, err := generateRecommendationAnalysisForCluster(c.Request.Context(), clusterID, mgr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to generate recommendation analysis for cluster %s: %v", clusterID, err),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

func generateRecommendationAnalysisForCluster(ctx context.Context, clusterID string, clusterMgr cluster.Manager) (*types.RecommendationAnalysisResponse, error) {
	clusterTask, err := clusterMgr.GetTask(clusterID + "_" + config.ApplyRecommendationKey)
	if err != nil {
		return nil, fmt.Errorf("error getting recommendation task: %w", err)
	}

	recomTask := clusterTask.GetCoreTask().(*task.ApplyRecommendationTask)
	nodeRecommendationMap, err := recomTask.GenerateNodeStatsForCluster(ctx)
	if err != nil {
		return nil, fmt.Errorf("error generating node recommendations: %w", err)
	}

	recommendationResults, err := recomTask.ApplyRecommendationsWithStrategy(ctx, nodeRecommendationMap, nil, applystrategies.NewAdjustAmongstPodsDistributedStrategy(ctx), false, true)
	if err != nil {
		return nil, fmt.Errorf("error applying recommendations: %w", err)
	}

	var analysis []types.RecommendationAnalysisItem
	var totalCurrentRequests, totalDifferences, totalCurrentMemoryRequests, totalMemoryDifferences float64

	for _, result := range recommendationResults {
		for _, rec := range result.PodContainerRecommendations {
			if rec.PodInfo.Stats != nil {
				containerResource, err := rec.PodInfo.GetContainerResource(rec.ContainerName)
				if err != nil {
					logging.Errorf(ctx, "error getting container resource for pod %s/%s: %v", rec.PodInfo.Namespace, rec.PodInfo.Name, err)
					continue
				}
				currentRequestedCPU := containerResource.CPURequest
				currentRequestedMemory := containerResource.MemoryRequest
				analysisItem := analyzeWorkloadStats(rec.PodInfo.Stats, rec.PodInfo.Name, rec.ContainerName, result.NodeName, currentRequestedCPU, rec.CPU, currentRequestedMemory, rec.Memory)
				analysis = append(analysis, analysisItem)
				totalCurrentRequests += currentRequestedCPU
				totalDifferences += analysisItem.CPUDifference
				totalCurrentMemoryRequests += currentRequestedMemory
				totalMemoryDifferences += analysisItem.MemoryDifference
			}
		}

		for _, nonOptPod := range result.NonOptimizablePods {
			if nonOptPod.PodInfo.Stats != nil {
				for _, containerResource := range nonOptPod.PodInfo.ContainerResources {
					analysisItem := analyzeWorkloadStats(nonOptPod.PodInfo.Stats, nonOptPod.PodName, containerResource.Name, result.NodeName, containerResource.CPURequest, containerResource.CPURequest, containerResource.MemoryRequest, containerResource.MemoryRequest)
					analysis = append(analysis, analysisItem)
					totalCurrentRequests += containerResource.CPURequest
					totalDifferences += analysisItem.CPUDifference
					totalCurrentMemoryRequests += containerResource.MemoryRequest
					totalMemoryDifferences += analysisItem.MemoryDifference
				}
			}
		}
	}

	summary := types.RecommendationSummary{
		TotalCurrentCPURequests:    totalCurrentRequests,
		TotalCPUDifferences:        totalDifferences,
		TotalCurrentMemoryRequests: totalCurrentMemoryRequests,
		TotalMemoryDifferences:     totalMemoryDifferences,
	}

	return &types.RecommendationAnalysisResponse{
		Analysis: analysis,
		Summary:  summary,
	}, nil
}

func analyzeWorkloadStats(stat *utils.WorkloadStat, podName, containerName, nodeName string, currentRequestedCPU, recommendedCPU, currentRequestedMemory, recommendedMemory float64) types.RecommendationAnalysisItem {
	blockingKarpenter := "No"
	if stat.Constraints != nil && stat.Constraints.Blocking {
		blockingKarpenter = "Yes"
	}

	autoscalingOnCPU := "No"
	if stat.IsHorizontallyAutoscaledOnCPU {
		autoscalingOnCPU = "Yes"
	}

	cpuUsage7Days := "N/A"
	containerStat, err := stat.GetContainerStats(containerName)
	if err == nil && containerStat.CPU7Day != nil {
		cpuUsage7Days = fmt.Sprintf("%.2f / %.2f / %.2f / %.2f / %.2f",
			containerStat.CPU7Day.Max, containerStat.CPU7Day.P99,
			containerStat.CPU7Day.P90, containerStat.CPU7Day.P75, containerStat.CPU7Day.P50)
	}

	spikeRange := 0.0
	if containerStat != nil && containerStat.SimplePredictionsCPU != nil && containerStat.CPUStats != nil {
		spikeRange = containerStat.SimplePredictionsCPU.MaxValue - containerStat.CPUStats.P50
	}

	requestGap := 0.0
	if containerStat != nil && containerStat.CPUStats != nil {
		requestGap = currentRequestedCPU - containerStat.CPUStats.P50
	}

	cpuDifference := math.Round((currentRequestedCPU-recommendedCPU)*1000) / 1000
	memoryDifference := math.Round(currentRequestedMemory - recommendedMemory)

	return types.RecommendationAnalysisItem{
		WorkloadType:           stat.Kind,
		WorkloadNamespace:      stat.Namespace,
		WorkloadName:           stat.Name,
		ContainerName:          containerName,
		PodName:                podName,
		CPUUsage7Days:          cpuUsage7Days,
		SpikeRange:             spikeRange,
		RequestGap:             requestGap,
		AutoscalingOnCPU:       autoscalingOnCPU,
		BlockingKarpenter:      blockingKarpenter,
		NodeName:               nodeName,
		CurrentRequestedCPU:    math.Round(currentRequestedCPU*1000) / 1000,
		RecommendedCPU:         math.Round(recommendedCPU*1000) / 1000,
		CPUDifference:          cpuDifference,
		CurrentRequestedMemory: math.Round(currentRequestedMemory),
		RecommendedMemory:      math.Round(recommendedMemory),
		MemoryDifference:       math.Round(memoryDifference),
	}
}
