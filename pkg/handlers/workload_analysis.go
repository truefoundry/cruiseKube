package handlers

import (
	"fmt"
	"net/http"

	"github.com/truefoundry/cruiseKube/pkg/task/utils"
	"github.com/truefoundry/cruiseKube/pkg/types"

	"github.com/gin-gonic/gin"
)

const (
	YesValue = "Yes"
	NoValue  = "No"
)

func WorkloadAnalysisHandlerForCluster(c *gin.Context) {
	// ctx := c.Request.Context()
	clusterID := c.Param("clusterID")

	analysis, err := generateWorkloadAnalysisForCluster(clusterID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to generate workload analysis for cluster %s: %v", clusterID, err),
		})
		return
	}
	c.JSON(http.StatusOK, analysis)
}

func analyzeWorkload(stat utils.WorkloadStat) []types.WorkloadAnalysisItem {
	var analysis []types.WorkloadAnalysisItem

	// Determine if workload is blocking Karpenter
	blockingKarpenter := NoValue
	if stat.Constraints != nil && stat.Constraints.Blocking {
		blockingKarpenter = YesValue
	}

	// Determine autoscaling status
	autoscalingOnCPU := NoValue
	if stat.IsHorizontallyAutoscaledOnCPU {
		autoscalingOnCPU = YesValue
	}

	for _, container := range stat.ContainerStats {
		workloadName := stat.Name
		workloadType := stat.Kind
		workloadNamespace := stat.Namespace
		containerName := container.ContainerName
		containerType := container.ContainerType
		cpuUsage7Days := "N/A"
		if container.CPU7Day != nil {
			cpuUsage7Days = fmt.Sprintf("%.2f / %.2f / %.2f / %.2f / %.2f", container.CPU7Day.Max, container.CPU7Day.P99, container.CPU7Day.P90, container.CPU7Day.P75, container.CPU7Day.P50)
		}
		spikeRange := calculateSpikeRange(container.SimplePredictionsCPU.MaxValue, container.CPUStats.P50)
		requestGap := 0.0
		for _, originalContainerResource := range stat.OriginalContainerResources {
			if originalContainerResource.Name == containerName {
				requestGap = calculateRequestGap(originalContainerResource.CPURequest, container.CPUStats.P50)
				break
			}
		}
		analysis = append(analysis, types.WorkloadAnalysisItem{
			WorkloadName:      workloadName,
			WorkloadType:      workloadType,
			WorkloadNamespace: workloadNamespace,
			ContainerName:     containerName,
			ContainerType:     containerType,
			CPUUsage7Days:     cpuUsage7Days,
			SpikeRange:        spikeRange,
			RequestGap:        requestGap,
			AutoscalingOnCPU:  autoscalingOnCPU,
			BlockingKarpenter: blockingKarpenter,
		})
	}
	return analysis
}

func calculateSpikeRange(pMax, p50 float64) float64 {
	return (pMax - p50)
}

func calculateRequestGap(requestedCPU, p50 float64) float64 {
	return (requestedCPU - p50)
}

func generateWorkloadAnalysisForCluster(clusterID string) ([]types.WorkloadAnalysisItem, error) {
	statsFile, err := utils.LoadStatsFromClusterStorage(clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to load stats from storage for cluster %s: %w", clusterID, err)
	}

	var analysis []types.WorkloadAnalysisItem
	for _, stat := range statsFile.Stats {
		analysis = append(analysis, analyzeWorkload(stat)...)
	}

	return analysis, nil
}
