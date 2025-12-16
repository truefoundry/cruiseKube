package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Performance - CPU
	ClusterCPUUtilizationCores = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_cpu_utilization_cores",
		Help: "Total CPU utilization in cores for the cluster (GPU nodes excluded)",
	}, []string{"cluster"})

	ClusterCPURequestCores = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_cpu_request_cores",
		Help: "Total CPU requests in cores for the cluster (GPU nodes excluded)",
	}, []string{"cluster"})

	ClusterCPUAllocatedCores = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_cpu_allocated_cores",
		Help: "Total allocatable CPU cores for the cluster (GPU nodes excluded)",
	}, []string{"cluster"})

	NodeCPUWaitingMax = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_node_cpu_waiting_max",
		Help: "Maximum CPU waiting time across nodes (GPU nodes excluded)",
	}, []string{"cluster"})

	NodeCPUWaitingP50 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_node_cpu_waiting_p50",
		Help: "P50 CPU waiting time across nodes (GPU nodes excluded)",
	}, []string{"cluster"})

	NodeCPULoadMax = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_node_cpu_load_max",
		Help: "Maximum CPU load across nodes (GPU nodes excluded)",
	}, []string{"cluster"})

	NodeCPULoadP50 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_node_cpu_load_p50",
		Help: "P50 CPU load across nodes (GPU nodes excluded)",
	}, []string{"cluster"})

	ContainerCPUWaitingMax = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_container_cpu_waiting_max",
		Help: "Maximum CPU waiting time across all containers",
	}, []string{"cluster"})

	ContainerCPUWaitingP50 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_container_cpu_waiting_p50",
		Help: "P50 CPU waiting time across all containers",
	}, []string{"cluster"})

	ClusterSpikeCPU = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_spike_cpu_cores",
		Help: "Sum of MaxRestCPU across all nodes in the cluster",
	}, []string{"cluster"})

	// Performance - Memory
	ClusterMemoryUtilizationBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_memory_utilization_bytes",
		Help: "Total memory utilization in bytes for the cluster (GPU nodes excluded)",
	}, []string{"cluster"})

	ClusterMemoryRequestBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_memory_request_bytes",
		Help: "Total memory requests in bytes for the cluster (GPU nodes excluded)",
	}, []string{"cluster"})

	ClusterMemoryAllocatedBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_memory_allocated_bytes",
		Help: "Total allocatable memory in bytes for the cluster (GPU nodes excluded)",
	}, []string{"cluster"})

	NodeMemoryWaitingMax = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_node_memory_waiting_max",
		Help: "Maximum memory waiting time across nodes (GPU nodes excluded)",
	}, []string{"cluster"})

	NodeMemoryWaitingP50 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_node_memory_waiting_p50",
		Help: "P50 memory waiting time across nodes (GPU nodes excluded)",
	}, []string{"cluster"})

	ContainerMemoryWaitingMax = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_container_memory_waiting_max",
		Help: "Maximum memory waiting time across all containers",
	}, []string{"cluster"})

	ContainerMemoryWaitingP50 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_container_memory_waiting_p50",
		Help: "P50 memory waiting time across all containers",
	}, []string{"cluster"})

	ClusterOOMEventsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_oom_events_total",
		Help: "Total number of OOM killed containers with recent restarts",
	}, []string{"cluster"})

	ClusterSpikeMemory = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_spike_memory_bytes",
		Help: "Sum of MaxRestMemory across all nodes in the cluster (in bytes)",
	}, []string{"cluster"})

	// Eviction Metrics
	ClusterEvictionCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cruisekube_eviction_count",
		Help: "Total number of evictions in the cluster by cruisekube",
	}, []string{"cluster"})

	ClusterKarpenterConsolidationEvictionCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_karpenter_consolidation_eviction_count",
		Help: "Total number of karpenter consolidation + eviction in the cluster",
	}, []string{"cluster"})

	// Scheduling Metrics
	ClusterUnschedulablePodsCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_cluster_unschedulable_pods_count",
		Help: "Total number of unschedulable pods in the cluster",
	}, []string{"cluster"})

	// Algo stats Metrics
	ClusterNonOptimizablePodsCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_non_optimizable_pods_count",
		Help: "Number of non optimizable pods per node in the cluster",
	}, []string{"cluster", "node"})

	ClusterOptimizablePodsCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_optimizable_pods_count",
		Help: "Number of optimizable pods per node in the cluster",
	}, []string{"cluster", "node"})

	WorkloadStatsAgeMax = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_workload_stats_age_max_minutes",
		Help: "Maximum age of workload stats in minutes",
	}, []string{"cluster"})

	WorkloadStatsAgeP90 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_workload_stats_age_p90_minutes",
		Help: "P90 age of workload stats in minutes",
	}, []string{"cluster"})

	WorkloadStatsAgeP50 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_workload_stats_age_p50_minutes",
		Help: "P50 age of workload stats in minutes",
	}, []string{"cluster"})

	TaskCompletionTime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cruisekube_task_completion_time_seconds",
		Help: "Task completion time in seconds",
	}, []string{"cluster", "task_name"})

	TaskRunCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cruisekube_task_run_count",
		Help: "Total number of task runs",
	}, []string{"cluster", "task_name", "status"})

	// Webhook Metrics
	WebhookControllerAPICallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "cruisekube_webhook_controller_api_calls",
		Help: "Total number of API calls from webhook to controller",
	}, []string{"cluster", "status", "endpoint"})
)
