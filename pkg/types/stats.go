package types

import (
	"fmt"
	"time"
)

type WorkloadConstraints struct {
	Blocking                 bool `json:"blocking"`
	PDB                      bool `json:"pdb"`
	DoNotDisruptAnnotation   bool `json:"do_not_disrupt_annotation"`
	Volume                   bool `json:"volume"`
	Affinity                 bool `json:"affinity"`
	TopologySpreadConstraint bool `json:"topology_spread_constraint"`
	PodAntiAffinity          bool `json:"pod_anti_affinity"`
	ExcludedAnnotation       bool `json:"excluded_annotation"`
}

type EvictionRanking int

const (
	EvictionRankingDisabled EvictionRanking = 1
	EvictionRankingLow      EvictionRanking = 2
	EvictionRankingMedium   EvictionRanking = 3
	EvictionRankingHigh     EvictionRanking = 4
)

type Overrides struct {
	EvictionRanking *EvictionRanking `json:"eviction_ranking"`
	Enabled         *bool            `json:"enabled"`
}

type ContainerType int

const (
	InitContainer ContainerType = iota + 1
	SidecarContainer
	AppContainer
)

type WorkloadStat struct {
	WorkloadIdentifier            string               `json:"workload"`
	Kind                          string               `json:"kind"`
	Namespace                     string               `json:"namespace"`
	Name                          string               `json:"name"`
	CreationTime                  time.Time            `json:"creation_time"`
	UpdatedAt                     time.Time            `json:"updated_at"`
	ContinuousOptimization        bool                 `json:"continuous_optimization"`
	IsHorizontallyAutoscaledOnCPU bool                 `json:"is_horizontally_autoscaled_on_cpu"`
	Constraints                   *WorkloadConstraints `json:"constraints,omitempty"`
	EvictionRanking               EvictionRanking      `json:"eviction_ranking"`
	Replicas                      int32                `json:"replicas"`

	ContainerStats             []ContainerStats             `json:"container_stats"`
	OriginalContainerResources []OriginalContainerResources `json:"original_container_resources"`
}

type ContainerStats struct {
	ContainerName string        `json:"container_name"`
	ContainerType ContainerType `json:"container_type"`

	CPUStats         *CPUStats              `json:"cpu_stats"`
	PSIAdjustedUsage *PSIAdjustedUsageStats `json:"psi_adjusted_usage,omitempty"`

	MemoryStats *MemoryStats     `json:"memory_stats"`
	Memory7Day  *Memory7DayStats `json:"memory_7day"`
	CPU7Day     *CPU7DayStats    `json:"cpu_7day"`

	MLPercentilesCPU            *MLPercentilesCPU            `json:"ml_percentiles_cpu,omitempty"`
	MLPercentilesCPUPSIAdjusted *MLPercentilesCPUPSIAdjusted `json:"ml_percentiles_cpu_psi_adjusted,omitempty"`
	SimplePredictionsCPU        *SimplePrediction            `json:"simple_predictions_cpu,omitempty"`

	MLPercentilesMemory     *MLPercentilesMemory `json:"ml_percentiles_memory,omitempty"`
	SimplePredictionsMemory *SimplePrediction    `json:"simple_predictions_memory,omitempty"`
}

type CPUStats struct {
	Max float64 `json:"max"`
	P50 float64 `json:"p50"`
	P75 float64 `json:"p75"`
}

type MemoryStats struct {
	Max       float64 `json:"max"`
	P75       float64 `json:"p75"`
	OOMMemory float64 `json:"oom_memory,omitempty"`
}

type Memory7DayStats struct {
	Max float64 `json:"max"`
}

type CPU7DayStats struct {
	Max float64 `json:"max"`
	P50 float64 `json:"p50"`
	P75 float64 `json:"p75"`
	P90 float64 `json:"p90"`
	P99 float64 `json:"p99"`
}

type MLPercentilesCPU struct {
	Median float64 `json:"median"`
	P90    float64 `json:"p90"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
}

type MLPercentilesCPUPSIAdjusted struct {
	Median float64 `json:"median"`
	P90    float64 `json:"p90"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
}

type MLPercentilesMemory struct {
	Median float64 `json:"median"`
	P90    float64 `json:"p90"`
	P95    float64 `json:"p95"`
	P99    float64 `json:"p99"`
}

type PSIAdjustedUsageStats struct {
	Max float64 `json:"max"`
	P50 float64 `json:"p50"`
	P75 float64 `json:"p75"`
}

type OriginalContainerResources struct {
	Name          string        `json:"name"`
	Type          ContainerType `json:"type"`
	CPURequest    float64       `json:"cpu_request"`
	CPULimit      float64       `json:"cpu_limit"`
	MemoryRequest float64       `json:"memory_request,omitempty"`
	MemoryLimit   float64       `json:"memory_limit,omitempty"`
}

type StatsResponse struct {
	Stats []WorkloadStat `json:"stats"`
}

type SimplePrediction struct {
	WeeklyPrediction  float64 `json:"weekly_prediction"`
	HourlyPrediction  float64 `json:"hourly_prediction"`
	CurrentPrediction float64 `json:"current_prediction"`
	MaxValue          float64 `json:"max_value"`
}

func (w *WorkloadStat) CalculateTotalCPUStats(percentile float64) float64 {
	sumAppSidecar := 0.0
	maxInit := 0.0

	for _, c := range w.ContainerStats {
		v := c.CPUStats.GetPercentile(percentile)
		switch c.ContainerType {
		case AppContainer, SidecarContainer:
			sumAppSidecar += v
		case InitContainer:
			maxInit = max(maxInit, v)
		}
	}

	return max(sumAppSidecar, maxInit)
}

func (w *WorkloadStat) CalculateTotalMemoryStats(percentile float64) float64 {
	sumAppSidecar := 0.0
	maxInit := 0.0

	for _, c := range w.ContainerStats {
		if c.MemoryStats == nil {
			continue
		}

		v := c.MemoryStats.GetPercentile(percentile)
		switch c.ContainerType {
		case AppContainer, SidecarContainer:
			sumAppSidecar += v
		case InitContainer:
			maxInit = max(maxInit, v)
		}
	}

	return max(sumAppSidecar, maxInit)
}

func (w *WorkloadStat) CalculateTotalCPURequest() float64 {
	sumAppSidecar := 0.0
	maxInit := 0.0

	for _, r := range w.OriginalContainerResources {
		switch r.Type {
		case AppContainer, SidecarContainer:
			sumAppSidecar += r.CPURequest
		case InitContainer:
			maxInit = max(maxInit, r.CPURequest)
		}
	}

	return max(sumAppSidecar, maxInit)
}

func (w *WorkloadStat) CalculateTotalMemoryRequest() float64 {
	sumAppSidecar := 0.0
	maxInit := 0.0

	for _, r := range w.OriginalContainerResources {
		switch r.Type {
		case AppContainer, SidecarContainer:
			sumAppSidecar += r.MemoryRequest
		case InitContainer:
			maxInit = max(maxInit, r.MemoryRequest)
		}
	}

	return max(sumAppSidecar, maxInit)
}

func (c *CPUStats) GetPercentile(percentile float64) float64 {
	switch percentile {
	case 50:
		return c.P50
	case 75:
		return c.P75
	case 100:
		return c.Max
	}
	return 0.0
}

func (m *MemoryStats) GetPercentile(percentile float64) float64 {
	switch percentile {
	case 75:
		return m.P75
	case 100:
		return m.Max
	}
	return 0.0
}

func (w *WorkloadStat) GetContainerStats(containerName string) (*ContainerStats, error) {
	for _, containerStat := range w.ContainerStats {
		if containerStat.ContainerName == containerName {
			return &containerStat, nil
		}
	}
	return nil, fmt.Errorf("container %s not found in workload %s", containerName, w.WorkloadIdentifier)
}

func (w *WorkloadStat) GetOriginalContainerResource(containerName string) (*OriginalContainerResources, error) {
	for _, containerResource := range w.OriginalContainerResources {
		if containerResource.Name == containerName {
			return &containerResource, nil
		}
	}
	return nil, fmt.Errorf("container %s not found in workload %s", containerName, w.WorkloadIdentifier)
}

type OOMEvent struct {
	ID                 uint      `json:"id"`
	ClusterID          string    `json:"cluster_id"`
	ContainerID        string    `json:"container_id"`
	Timestamp          time.Time `json:"timestamp"`
	MemoryLimit        int64     `json:"memory_limit"`
	MemoryRequest      int64     `json:"memory_request"`
	LastObservedMemory int64     `json:"last_observed_memory"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
