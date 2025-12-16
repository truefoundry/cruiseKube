package utils

import (
	"github.com/truefoundry/cruisekube/pkg/repository/storage"
	"github.com/truefoundry/cruisekube/pkg/types"
)

type WorkloadConstraints = types.WorkloadConstraints
type WorkloadStat = types.WorkloadStat

type ContainerStats = types.ContainerStats

type CPUStats = types.CPUStats
type MemoryStats = types.MemoryStats
type Memory7DayStats = types.Memory7DayStats
type CPU7DayStats = types.CPU7DayStats
type MLPercentilesCPU = types.MLPercentilesCPU
type MLPercentilesCPUPSIAdjusted = types.MLPercentilesCPUPSIAdjusted
type MLPercentilesMemory = types.MLPercentilesMemory
type PSIAdjustedUsageStats = types.PSIAdjustedUsageStats
type OriginalContainerResources = types.OriginalContainerResources
type StatsResponse = types.StatsResponse

func LoadStatsFromClusterStorage(clusterID string) (*StatsResponse, error) {
	var statsFile StatsResponse
	if err := storage.Stg.ReadClusterStats(clusterID, &statsFile); err != nil {
		return nil, err
	}
	return &statsFile, nil
}
