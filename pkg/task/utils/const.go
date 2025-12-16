package utils

import "time"

const (
	ExcludedAnnotation                   = "cruiseKube.truefoundry.com/excluded"
	ContinuousOptimizationRatioThreshold = 3.0
	ContinuousOptimizationDiffThreshold  = 0.001
	BytesToMBDivisor                     = 1000_000
)

const (
	TrueValue = "true"
)

const (
	DeploymentKind  = "Deployment"
	StatefulSetKind = "StatefulSet"
	DaemonSetKind   = "DaemonSet"
	ReplicaSetKind  = "ReplicaSet"
	RolloutKind     = "Rollout"
)

const (
	CPULookbackWindow        = 10 * time.Minute
	CPU7DayLookbackWindow    = 7 * 24 * time.Hour
	ReplicaLookbackWindow    = 7 * 24 * time.Hour
	MemoryLookbackWindow     = 30 * time.Minute
	Memory7DayLookbackWindow = 7 * 24 * time.Hour

	MLLookbackWindow    = 7 * 24 * time.Hour
	RateIntervalMinutes = 1
	ResolutionMinutes   = 1
	CPUDecimalScale     = 1000.0

	BytesPerMB                 = 1_000_000
	MemoryDecimalPlaces        = 1
	RecentStatsLookbackMinutes = 10
)
