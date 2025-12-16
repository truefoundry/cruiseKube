package task

import (
	"context"
	"sort"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/prometheus/common/model"
	"github.com/truefoundry/cruisekube/pkg/adapters/metricsProvider/prometheus"
	"github.com/truefoundry/cruisekube/pkg/contextutils"
	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/metrics"
	"github.com/truefoundry/cruisekube/pkg/repository/storage"
)

type FetchMetricsTaskConfig struct {
	Name      string
	Enabled   bool
	Schedule  string
	ClusterID string
}

type FetchMetricsTask struct {
	kubeClient    *kubernetes.Clientset
	dynamicClient dynamic.Interface
	promClient    *prometheus.PrometheusProvider
	storage       *storage.Storage
	config        *FetchMetricsTaskConfig
}

func NewFetchMetricsTask(ctx context.Context, kubeClient *kubernetes.Clientset, dynamicClient dynamic.Interface, promClient *prometheus.PrometheusProvider, storage *storage.Storage, config *FetchMetricsTaskConfig) *FetchMetricsTask {
	return &FetchMetricsTask{
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
		promClient:    promClient,
		storage:       storage,
		config:        config,
	}
}

func (f *FetchMetricsTask) GetCoreTask() any {
	return f
}

func (f *FetchMetricsTask) GetName() string {
	return f.config.Name
}

func (f *FetchMetricsTask) GetSchedule() string {
	return f.config.Schedule
}

func (f *FetchMetricsTask) IsEnabled() bool {
	return f.config.Enabled
}

func (f *FetchMetricsTask) Run(ctx context.Context) error {
	ctx = contextutils.WithTask(ctx, f.config.Name)
	ctx = contextutils.WithCluster(ctx, f.config.ClusterID)
	f.fetchClusterCPUUtilization(ctx)
	f.fetchClusterCPURequest(ctx)
	f.fetchClusterCPUAllocated(ctx)
	f.fetchNodeCPUWaiting(ctx)
	f.fetchNodeCPULoad(ctx)
	f.fetchContainerCPUWaiting(ctx)
	f.fetchClusterMemoryUtilization(ctx)
	f.fetchClusterMemoryRequest(ctx)
	f.fetchClusterMemoryAllocated(ctx)
	f.fetchNodeMemoryWaiting(ctx)
	f.fetchContainerMemoryWaiting(ctx)
	f.fetchClusterOOMEvents(ctx)
	f.fetchKarpenterConsolidationEvictionCount(ctx)
	f.fetchClusterUnschedulablePods(ctx)
	f.fetchWorkloadStatsAge(ctx)
	return nil
}

func (f *FetchMetricsTask) fetchClusterCPUUtilization(ctx context.Context) {
	query := `sum(
		sum by (node) (
			rate(container_cpu_usage_seconds_total{job="kubelet",container!~"POD|"}[1m])
		)
		unless sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)
	)`

	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, query, "cluster_cpu_utilization")
	if err != nil {
		logging.Errorf(ctx, "fetchClusterCPUUtilization: %v", err)
		return
	}

	if val := f.extractScalarValue(result); val != nil {
		metrics.ClusterCPUUtilizationCores.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchClusterCPURequest(ctx context.Context) {
	query := `(
		sum(
			sum by (node) (
				sum by (namespace, pod, node) (
				  kube_pod_container_resource_requests{container!="",job="kube-state-metrics",resource="cpu"}
				)
			* on (namespace, pod) group_left ()
			  max by (namespace, pod) (kube_pod_status_phase{job="kube-state-metrics",phase="Running"} > 0)
			)
			unless
			  sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)
		)
	)`

	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, query, "cluster_cpu_request")
	if err != nil {
		logging.Errorf(ctx, "fetchClusterCPURequest: %v", err)
		return
	}

	if val := f.extractScalarValue(result); val != nil {
		metrics.ClusterCPURequestCores.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchClusterCPUAllocated(ctx context.Context) {
	query := `sum(
		kube_node_status_allocatable{job="kube-state-metrics",resource="cpu"}
		unless sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)
	)`

	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, query, "cluster_cpu_allocated")
	if err != nil {
		logging.Errorf(ctx, "fetchClusterCPUAllocated: %v", err)
		return
	}

	if val := f.extractScalarValue(result); val != nil {
		metrics.ClusterCPUAllocatedCores.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchNodeCPUWaiting(ctx context.Context) {
	baseQuery := `rate(node_pressure_cpu_waiting_seconds_total{job="node-exporter"}[1m])
	unless on (node) sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)`

	maxQuery := `max(` + baseQuery + `)`
	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, maxQuery, "node_cpu_waiting_max")
	if err != nil {
		logging.Errorf(ctx, "fetchNodeCPUWaiting (max): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.NodeCPUWaitingMax.WithLabelValues(f.config.ClusterID).Set(*val)
	}

	p50Query := `quantile(0.50, ` + baseQuery + `)`
	result, _, err = f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, p50Query, "node_cpu_waiting_p50")
	if err != nil {
		logging.Errorf(ctx, "fetchNodeCPUWaiting (p50): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.NodeCPUWaitingP50.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchNodeCPULoad(ctx context.Context) {
	baseQuery := `
		(
			node_load1{job="node-exporter"}
		  / on (node) group_left ()
			kube_node_status_capacity{job="kube-state-metrics",resource="cpu"}
		)
	  *
		100
	unless on (node)
	  sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)`

	maxQuery := `max(` + baseQuery + `)`
	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, maxQuery, "node_cpu_load_max")
	if err != nil {
		logging.Errorf(ctx, "fetchNodeCPULoad (max): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.NodeCPULoadMax.WithLabelValues(f.config.ClusterID).Set(*val)
	}

	p50Query := `quantile(0.50, ` + baseQuery + `)`
	result, _, err = f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, p50Query, "node_cpu_load_p50")
	if err != nil {
		logging.Errorf(ctx, "fetchNodeCPULoad (p50): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.NodeCPULoadP50.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchContainerCPUWaiting(ctx context.Context) {
	baseQuery := `rate(container_pressure_cpu_waiting_seconds_total{job="kubelet",container!=""}[1m])
	unless on (node) sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)`

	maxQuery := `max(` + baseQuery + `)`
	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, maxQuery, "container_cpu_waiting_max")
	if err != nil {
		logging.Errorf(ctx, "fetchContainerCPUWaiting (max): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.ContainerCPUWaitingMax.WithLabelValues(f.config.ClusterID).Set(*val)
	}

	p50Query := `quantile(0.50, ` + baseQuery + `)`
	result, _, err = f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, p50Query, "container_cpu_waiting_p50")
	if err != nil {
		logging.Errorf(ctx, "fetchContainerCPUWaiting (p50): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.ContainerCPUWaitingP50.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchClusterMemoryUtilization(ctx context.Context) {
	query := `sum(
		sum by (node) (
			container_memory_working_set_bytes{job="kubelet",container!~"POD|"}
		)
		unless sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)
	)`

	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, query, "cluster_memory_utilization")
	if err != nil {
		logging.Errorf(ctx, "fetchClusterMemoryUtilization: %v", err)
		return
	}

	if val := f.extractScalarValue(result); val != nil {
		metrics.ClusterMemoryUtilizationBytes.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchClusterMemoryRequest(ctx context.Context) {
	query :=
		`(
		sum(
		    sum by (node) (
				sum by (namespace, pod, node) (
                    kube_pod_container_resource_requests{container!="",job="kube-state-metrics",resource="memory"}
                )
				* on (namespace, pod) group_left ()
				max by (namespace, pod) (kube_pod_status_phase{job="kube-state-metrics",phase="Running"} > 0)
            )
			unless
			  sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)
		)
	)`

	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, query, "cluster_memory_request")
	if err != nil {
		logging.Errorf(ctx, "fetchClusterMemoryRequest: %v", err)
		return
	}

	if val := f.extractScalarValue(result); val != nil {
		metrics.ClusterMemoryRequestBytes.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchClusterMemoryAllocated(ctx context.Context) {
	query := `sum(
		kube_node_status_allocatable{job="kube-state-metrics",resource="memory"}
		unless sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)
	)`

	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, query, "cluster_memory_allocated")
	if err != nil {
		logging.Errorf(ctx, "fetchClusterMemoryAllocated: %v", err)
		return
	}

	if val := f.extractScalarValue(result); val != nil {
		metrics.ClusterMemoryAllocatedBytes.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchNodeMemoryWaiting(ctx context.Context) {
	baseQuery := `rate(node_pressure_memory_waiting_seconds_total{job="node-exporter"}[1m])
	unless on (node) sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)`

	maxQuery := `max(` + baseQuery + `)`
	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, maxQuery, "node_memory_waiting_max")
	if err != nil {
		logging.Errorf(ctx, "fetchNodeMemoryWaiting (max): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.NodeMemoryWaitingMax.WithLabelValues(f.config.ClusterID).Set(*val)
	}

	p50Query := `quantile(0.50, ` + baseQuery + `)`
	result, _, err = f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, p50Query, "node_memory_waiting_p50")
	if err != nil {
		logging.Errorf(ctx, "fetchNodeMemoryWaiting (p50): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.NodeMemoryWaitingP50.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchContainerMemoryWaiting(ctx context.Context) {
	baseQuery := `rate(container_pressure_memory_waiting_seconds_total{job="kubelet",container!=""}[1m])
	unless on (node) sum by (node) (kube_node_status_allocatable{job="kube-state-metrics",resource="nvidia_com_gpu"} > 0)`

	maxQuery := `max(` + baseQuery + `)`
	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, maxQuery, "container_memory_waiting_max")
	if err != nil {
		logging.Errorf(ctx, "fetchContainerMemoryWaiting (max): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.ContainerMemoryWaitingMax.WithLabelValues(f.config.ClusterID).Set(*val)
	}

	p50Query := `quantile(0.50, ` + baseQuery + `)`
	result, _, err = f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, p50Query, "container_memory_waiting_p50")
	if err != nil {
		logging.Errorf(ctx, "fetchContainerMemoryWaiting (p50): %v", err)
	} else if val := f.extractScalarValue(result); val != nil {
		metrics.ContainerMemoryWaitingP50.WithLabelValues(f.config.ClusterID).Set(*val)
	}
}

func (f *FetchMetricsTask) fetchClusterOOMEvents(ctx context.Context) {
	query := `sum by (namespace, pod, container) (
		(kube_pod_container_status_last_terminated_reason{job="kube-state-metrics",reason="OOMKilled"}) 
		* on (namespace, pod, container) 
		sum by (namespace, pod, container) (increase(kube_pod_container_status_restarts_total{job="kube-state-metrics"}[1m]))
	)`

	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, query, "cluster_oom_events")
	if err != nil {
		logging.Errorf(ctx, "fetchClusterOOMEvents: %v", err)
		return
	}

	if vec, ok := result.(model.Vector); ok {
		total := 0.0
		for _, sample := range vec {
			total += float64(sample.Value)
		}
		metrics.ClusterOOMEventsTotal.WithLabelValues(f.config.ClusterID).Set(total)
	}
}

func (f *FetchMetricsTask) fetchKarpenterConsolidationEvictionCount(ctx context.Context) {
	query := `increase((sum by (reason) (karpenter_nodeclaims_disrupted_total))[1m:])`

	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, query, "cluster_unschedulable_pods")
	if err != nil {
		logging.Errorf(ctx, "fetchKarpenterConsolidationEvictionCount: %v", err)
		return
	}

	if val := f.extractScalarValue(result); val != nil {
		metrics.ClusterKarpenterConsolidationEvictionCount.WithLabelValues(f.config.ClusterID).Set(*val)
	} else {
		metrics.ClusterKarpenterConsolidationEvictionCount.WithLabelValues(f.config.ClusterID).Set(0)
	}
}

func (f *FetchMetricsTask) fetchClusterUnschedulablePods(ctx context.Context) {
	query := `sum(kube_pod_status_phase{job="kube-state-metrics",phase="Pending"})`

	result, _, err := f.promClient.ExecuteQueryWithRetry(ctx, f.config.ClusterID, query, "cluster_unschedulable_pods_count")
	if err != nil {
		logging.Errorf(ctx, "fetchClusterUnschedulablePods: %v", err)
		return
	}

	if val := f.extractScalarValue(result); val != nil {
		metrics.ClusterUnschedulablePodsCount.WithLabelValues(f.config.ClusterID).Set(*val)
	} else {
		metrics.ClusterUnschedulablePodsCount.WithLabelValues(f.config.ClusterID).Set(0)
	}
}

func (f *FetchMetricsTask) extractScalarValue(result model.Value) *float64 {
	if result == nil {
		return nil
	}
	if v, ok := result.(model.Vector); ok {
		if len(v) > 0 {
			val := float64(v[0].Value)
			return &val
		}
	}
	if s, ok := result.(*model.Scalar); ok {
		val := float64(s.Value)
		return &val
	}
	return nil
}

func (f *FetchMetricsTask) fetchWorkloadStatsAge(ctx context.Context) {
	stats, err := f.storage.GetAllStatsForCluster(f.config.ClusterID)
	if err != nil {
		logging.Errorf(ctx, "fetchWorkloadStatsAge: failed to read stats from storage: %v", err)
		return
	}

	if len(stats) == 0 {
		logging.Infof(ctx, "fetchWorkloadStatsAge: no stats found for cluster %s", f.config.ClusterID)
		return
	}

	now := time.Now()
	ages := make([]float64, 0, len(stats))

	for _, stat := range stats {
		age := now.Sub(stat.UpdatedAt).Minutes()
		ages = append(ages, age)
	}

	sort.Float64s(ages)

	maxAge := ages[len(ages)-1]
	p90Index := int(float64(len(ages)) * 0.9)
	p50Index := len(ages) / 2

	metrics.WorkloadStatsAgeMax.WithLabelValues(f.config.ClusterID).Set(maxAge)
	metrics.WorkloadStatsAgeP90.WithLabelValues(f.config.ClusterID).Set(ages[p90Index])
	metrics.WorkloadStatsAgeP50.WithLabelValues(f.config.ClusterID).Set(ages[p50Index])

	logging.Infof(ctx, "fetchWorkloadStatsAge: max=%.1fm, p90=%.1fm, p50=%.1fm", maxAge, ages[p90Index], ages[p50Index])
}
