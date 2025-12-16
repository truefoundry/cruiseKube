package prometheus

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ctxUtils "github.com/truefoundry/cruiseKube/pkg/contextutils"
	"github.com/truefoundry/cruiseKube/pkg/logging"
	"github.com/truefoundry/cruiseKube/pkg/task/utils"
	"github.com/truefoundry/cruiseKube/pkg/telemetry"

	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	CPULookbackWindow        = 10 * time.Minute
	CPU7DayLookbackWindow    = 7 * 24 * time.Hour
	ReplicaLookbackWindow    = 7 * 24 * time.Hour
	MemoryLookbackWindow     = 30 * time.Minute
	Memory7DayLookbackWindow = 7 * 24 * time.Hour
	CPUDecimalScale          = 1000.0
	BytesPerMB               = 1_000_000
	MemoryDecimalPlaces      = 1
	RateIntervalMinutes      = 1
)

var _ = otel.Tracer("cruiseKube/tasks/promql") // promqlTracer unused but kept for future use

type QueryResult struct {
	Result   model.Value
	Warnings []string
	Error    error
	QueryID  string
}

type ParallelQueryRequest struct {
	QueryID string
	Query   string
}

func (p *PrometheusProvider) getOrCreateQuerySemaphore(clusterId string) chan struct{} {
	if semaphore, ok := p.querySemaphores.Load(clusterId); ok {
		return semaphore.(chan struct{})
	}
	semaphore := make(chan struct{}, p.config.MaxConcurrentQueries)
	p.querySemaphores.Store(clusterId, semaphore)
	return semaphore
}

func (p *PrometheusProvider) acquireQuerySlot(clusterId string) {
	sem := p.getOrCreateQuerySemaphore(clusterId)
	sem <- struct{}{}
}

func (p *PrometheusProvider) releaseQuerySlot(ctx context.Context, clusterId string) {
	semaphore, ok := p.querySemaphores.Load(clusterId)
	if !ok {
		logging.Errorf(ctx, "Query semaphore not found for cluster %s", clusterId)
		return
	}
	<-semaphore.(chan struct{})
}

func (p *PrometheusProvider) createQueryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, p.config.QueryTimeout)
}

func (p *PrometheusProvider) ExecuteQueryWithRetry(ctx context.Context, clusterId, query, queryID string) (model.Value, []string, error) {
	// Put identifiers into context; StartSpan will copy them as attributes
	ctx = ctxUtils.WithQueryID(ctx, queryID)
	ctx = ctxUtils.WithCluster(ctx, clusterId)
	ctx, span := telemetry.StartSpan(ctx, "cruiseKube/tasks/promql", fmt.Sprintf("promql.query.%s", queryID))
	defer span.End()

	var lastErr error
	var result model.Value
	var warnings []string

	for attempt := range p.config.MaxQueryRetries {
		ctx, cancel := p.createQueryContext(ctx)

		p.acquireQuerySlot(clusterId)

		logging.Infof(ctx, "Query for %s: %v", queryID, CompressQueryForLogging(query))
		result, warnings, lastErr = p.client.Query(ctx, query, time.Now())

		p.releaseQuerySlot(ctx, clusterId)
		cancel()

		if lastErr == nil {
			if len(warnings) > 0 {
				span.SetAttributes(attribute.Int("warnings.count", len(warnings)))
			}
			return result, warnings, nil
		}

		if attempt < p.config.MaxQueryRetries-1 {
			backoffDuration := p.config.RetryBackoffBase * time.Duration(1<<attempt)
			logging.Infof(ctx, "Query %s failed (attempt %d/%d): %v. Retrying in %v...",
				queryID, attempt+1, p.config.MaxQueryRetries, lastErr, backoffDuration)
			time.Sleep(backoffDuration)
		}
	}

	if lastErr != nil {
		span.RecordError(lastErr)
		span.SetStatus(codes.Error, lastErr.Error())
	}
	return nil, warnings, fmt.Errorf("query %s failed after %d attempts: %w", queryID, p.config.MaxQueryRetries, lastErr)
}

func (p *PrometheusProvider) executeQueriesInParallel(ctx context.Context, clusterId string, requests []ParallelQueryRequest) (map[string]QueryResult, error) {
	results := make(map[string]QueryResult)
	resultsChan := make(chan QueryResult, len(requests))
	var wg sync.WaitGroup

	for _, req := range requests {
		wg.Add(1)
		go func(request ParallelQueryRequest) {
			defer wg.Done()

			result, warnings, err := p.ExecuteQueryWithRetry(ctx, clusterId, request.Query, request.QueryID)

			resultsChan <- QueryResult{
				Result:   result,
				Warnings: warnings,
				Error:    err,
				QueryID:  request.QueryID,
			}
		}(req)
	}

	wg.Wait()
	close(resultsChan)

	for result := range resultsChan {
		results[result.QueryID] = result
		if result.Error != nil {
			return nil, fmt.Errorf("error executing query %s: %w", result.QueryID, result.Error)
		}
	}

	return results, nil
}

func CompressQueryForLogging(query string) string {
	compressed := strings.Fields(query)
	return strings.Join(compressed, " ")
}

func (p *PrometheusProvider) FetchStatsForNamespaces(ctx context.Context, clusterId string, namespaces []string, isPSIEnabled bool) (utils.NamespaceVsContainerMetrics, utils.NamespaceVsWorkloadMetrics, error) {
	globalContainerCache := make(utils.NamespaceVsContainerMetrics)
	globalWorkloadCache := make(utils.NamespaceVsWorkloadMetrics)

	type namespaceResult struct {
		namespace                    string
		cache                        utils.WorkloadKeyVsContainerMetrics
		workloadKeyVsWorkloadMetrics utils.WorkloadKeyVsWorkloadMetrics
		error                        error
	}

	resultsChan := make(chan namespaceResult, len(namespaces))
	var wg sync.WaitGroup

	logging.Infof(ctx, "Starting parallel execution for %d namespaces", len(namespaces))

	for _, namespace := range namespaces {
		wg.Add(1)
		go func(ns string) {
			defer wg.Done()

			cache, workloadKeyVsWorkloadMetrics, err := p.fetchStatsForNamespace(ctx, clusterId, ns, isPSIEnabled)
			resultsChan <- namespaceResult{
				namespace:                    ns,
				cache:                        cache,
				workloadKeyVsWorkloadMetrics: workloadKeyVsWorkloadMetrics,
				error:                        err,
			}
		}(namespace)
	}

	wg.Wait()
	close(resultsChan)

	successCount := 0
	for result := range resultsChan {
		if result.error == nil {
			globalContainerCache[result.namespace] = result.cache
			globalWorkloadCache[result.namespace] = result.workloadKeyVsWorkloadMetrics
			successCount++
		} else {
			logging.Infof(ctx, "Failed to execute batch queries for namespace %s: %v", result.namespace, result.error)
			return nil, nil, result.error
		}
	}

	return globalContainerCache, globalWorkloadCache, nil
}

func (p *PrometheusProvider) fetchStatsForNamespace(ctx context.Context, clusterId string, namespace string, isPSIEnabled bool) (utils.WorkloadKeyVsContainerMetrics, utils.WorkloadKeyVsWorkloadMetrics, error) {
	logging.Infof(ctx, "Executing all batch queries for namespace: %s", namespace)

	cache := make(utils.WorkloadKeyVsContainerMetrics)
	workloadKeyVsWorkloadMetrics := make(utils.WorkloadKeyVsWorkloadMetrics)
	var requests []ParallelQueryRequest

	cpuQueries := []struct {
		percentile float64
		key        string
	}{
		{0.50, "cpu_p50"},
		{0.75, "cpu_p75"},
		{1.0, "cpu_max"},
	}

	for _, q := range cpuQueries {
		query := p.buildBatchCPURecommendationQuery(namespace, q.percentile, false)
		requests = append(requests, ParallelQueryRequest{
			QueryID: fmt.Sprintf("%s-%s", namespace, q.key),
			Query:   query,
		})
	}

	if isPSIEnabled {
		for _, q := range cpuQueries {
			query := p.buildBatchCPURecommendationQuery(namespace, q.percentile, true)
			requests = append(requests, ParallelQueryRequest{
				QueryID: fmt.Sprintf("%s-%s-psi", namespace, q.key),
				Query:   query,
			})
		}
	}

	cpu7DayQueries := []struct {
		percentile float64
		key        string
	}{
		{0.50, "cpu_p50"},
		{0.75, "cpu_p75"},
		{0.90, "cpu_p90"},
		{0.99, "cpu_p99"},
		{1.0, "cpu_max"},
	}

	for _, q := range cpu7DayQueries {
		query := p.buildBatch7DayCPURecommendationQuery(namespace, q.percentile, false)
		requests = append(requests, ParallelQueryRequest{
			QueryID: fmt.Sprintf("%s-%s-cpu-7day", namespace, q.key),
			Query:   query,
		})
	}

	memoryQueries := []struct {
		percentile float64
		key        string
	}{
		{0.75, "memory_p75"},
		{1.0, "memory_max"},
	}

	for _, q := range memoryQueries {
		query := p.EncloseWithinMemoryCleanupFunction(p.buildBatchMemoryRecommendationQuery(namespace, q.percentile), MemoryDecimalPlaces)
		requests = append(requests, ParallelQueryRequest{
			QueryID: fmt.Sprintf("%s-%s-memory", namespace, q.key),
			Query:   query,
		})
	}

	query := p.EncloseWithinMemoryCleanupFunction(p.buildBatch7DayMemoryRecommendationQuery(namespace, 1.0), MemoryDecimalPlaces)
	requests = append(requests, ParallelQueryRequest{
		QueryID: fmt.Sprintf("%s-%s-memory-7day", namespace, "memory_max"),
		Query:   query,
	})

	oomQuery := p.EncloseWithinMemoryCleanupFunction(p.buildBatchOOMMemoryQuery(namespace), MemoryDecimalPlaces)
	requests = append(requests, ParallelQueryRequest{
		QueryID: fmt.Sprintf("%s-oom-memory", namespace),
		Query:   oomQuery,
	})

	replicaQueryExpr := p.buildBatchReplicaCountQuery(namespace)
	replicaQuery := fmt.Sprintf("quantile_over_time(0.5, (%s)[%s:1h])", replicaQueryExpr, ReplicaLookbackWindow.String())
	requests = append(requests, ParallelQueryRequest{
		QueryID: fmt.Sprintf("%s-replica", namespace),
		Query:   replicaQuery,
	})

	results, err := p.executeQueriesInParallel(ctx, clusterId, requests)
	if err != nil {
		return nil, nil, err
	}

	for _, q := range cpuQueries {
		queryID := fmt.Sprintf("%s-%s", namespace, q.key)
		if result, exists := results[queryID]; exists {
			if result.Error != nil {
				logging.Infof(ctx, "Error getting %s metrics for namespace %s: %v", q.key, namespace, result.Error)
				continue
			}

			if len(result.Warnings) > 0 {
				logging.Infof(ctx, "Warnings from %s query for namespace %s: %v", q.key, namespace, result.Warnings)
			}

			rawResults, err := p.parsePrometheusVectorResultForContainer(result.Result)
			if err != nil {
				logging.Infof(ctx, "Error parsing %s results for namespace %s: %v", q.key, namespace, err)
				continue
			}

			utils.MergeContainerRawResultsIntoCache(ctx, cache, rawResults, q.key, false)
		}
	}

	for _, q := range cpuQueries {
		queryID := fmt.Sprintf("%s-%s-psi", namespace, q.key)
		if result, exists := results[queryID]; exists {
			if result.Error != nil {
				logging.Infof(ctx, "Error getting %s metrics for namespace %s: %v", q.key, namespace, result.Error)
				continue
			}

			if len(result.Warnings) > 0 {
				logging.Infof(ctx, "Warnings from %s query for namespace %s: %v", q.key, namespace, result.Warnings)
			}

			rawResults, err := p.parsePrometheusVectorResultForContainer(result.Result)
			if err != nil {
				logging.Infof(ctx, "Error parsing %s results for namespace %s: %v", q.key, namespace, err)
				continue
			}

			utils.MergeContainerRawResultsIntoCache(ctx, cache, rawResults, q.key, true)
		}
	}

	if result, exists := results[fmt.Sprintf("%s-memory", namespace)]; exists {
		if result.Error != nil {
			logging.Infof(ctx, "Error getting Memory metrics for namespace %s: %v", namespace, result.Error)
		} else {
			if len(result.Warnings) > 0 {
				logging.Infof(ctx, "Warnings from Memory query for namespace %s: %v", namespace, result.Warnings)
			}

			rawResults, err := p.parsePrometheusVectorResultForContainer(result.Result)
			if err != nil {
				logging.Infof(ctx, "Error parsing Memory results for namespace %s: %v", namespace, err)
			} else {
				utils.MergeContainerRawResultsIntoCache(ctx, cache, rawResults, "memory_max", false)
			}
		}
	}

	for _, q := range memoryQueries {
		queryID := fmt.Sprintf("%s-%s-memory", namespace, q.key)
		if result, exists := results[queryID]; exists {
			if result.Error != nil {
				logging.Infof(ctx, "Error getting %s metrics for namespace %s: %v", q.key, namespace, result.Error)
				continue
			}

			if len(result.Warnings) > 0 {
				logging.Infof(ctx, "Warnings from %s query for namespace %s: %v", q.key, namespace, result.Warnings)
			}

			rawResults, err := p.parsePrometheusVectorResultForContainer(result.Result)
			if err != nil {
				logging.Infof(ctx, "Error parsing %s results for namespace %s: %v", q.key, namespace, err)
				continue
			}

			utils.MergeContainerRawResultsIntoCache(ctx, cache, rawResults, q.key, false)
		}
	}

	for _, q := range memoryQueries {
		queryID := fmt.Sprintf("%s-%s-memory-7day", namespace, q.key)
		if result, exists := results[queryID]; exists {
			if result.Error != nil {
				logging.Infof(ctx, "Error getting 7-day %s metrics for namespace %s: %v", q.key, namespace, result.Error)
				continue
			}

			if len(result.Warnings) > 0 {
				logging.Infof(ctx, "Warnings from 7-day %s query for namespace %s: %v", q.key, namespace, result.Warnings)
			}

			rawResults, err := p.parsePrometheusVectorResultForContainer(result.Result)
			if err != nil {
				logging.Infof(ctx, "Error parsing 7-day %s results for namespace %s: %v", q.key, namespace, err)
				continue
			}

			utils.MergeContainerRawResultsIntoCache(ctx, cache, rawResults, q.key+"_7day", false)
		}
	}

	if result, exists := results[fmt.Sprintf("%s-oom-memory", namespace)]; exists {
		if result.Error != nil {
			logging.Infof(ctx, "Error getting OOM memory metrics for namespace %s: %v", namespace, result.Error)
		} else {
			if len(result.Warnings) > 0 {
				logging.Infof(ctx, "Warnings from OOM memory query for namespace %s: %v", namespace, result.Warnings)
			}

			rawResults, err := p.parsePrometheusVectorResultForContainer(result.Result)
			if err != nil {
				logging.Infof(ctx, "Error parsing OOM memory results for namespace %s: %v", namespace, err)
			} else {
				utils.MergeContainerRawResultsIntoCache(ctx, cache, rawResults, "oom_memory", false)
			}
		}
	}

	for _, q := range cpu7DayQueries {
		queryID := fmt.Sprintf("%s-%s-cpu-7day", namespace, q.key)
		if result, exists := results[queryID]; exists {
			if result.Error != nil {
				logging.Infof(ctx, "Error getting 7-day %s CPU metrics for namespace %s: %v", q.key, namespace, result.Error)
				continue
			}

			if len(result.Warnings) > 0 {
				logging.Infof(ctx, "Warnings from 7-day %s CPU query for namespace %s: %v", q.key, namespace, result.Warnings)
			}

			rawResults, err := p.parsePrometheusVectorResultForContainer(result.Result)
			if err != nil {
				logging.Infof(ctx, "Error parsing 7-day %s CPU results for namespace %s: %v", q.key, namespace, err)
				continue
			}

			utils.MergeContainerRawResultsIntoCache(ctx, cache, rawResults, q.key+"_cpu_7day", false)
		}
	}

	if result, exists := results[fmt.Sprintf("%s-replica", namespace)]; exists {
		if result.Error != nil {
			logging.Infof(ctx, "Error getting Replica metrics for namespace %s: %v", namespace, result.Error)
		} else {
			if len(result.Warnings) > 0 {
				logging.Infof(ctx, "Warnings from Replica query for namespace %s: %v", namespace, result.Warnings)
			}

			replicaResults, err := p.parsePrometheusVectorResultForWorkload(ctx, result.Result)
			if err != nil {
				logging.Infof(ctx, "Error parsing Replica results for namespace %s: %v", namespace, err)
			} else {
				for workloadKey, replicaValue := range replicaResults {
					kind, namespace, name, ok := utils.ParseWorkloadKey(workloadKey)
					if !ok {
						logging.Infof(ctx, "Error parsing workload key: %s", workloadKey)
						continue
					}
					if kind == utils.ReplicaSetKind {
						deploymentName, isDeployment := utils.ExtractWorkloadFromReplicaSet(name)
						if isDeployment {
							workloadKey = utils.GetWorkloadKey(utils.DeploymentKind, namespace, deploymentName)
							workloadKeyVsWorkloadMetrics[workloadKey] = utils.WorkloadMetrics{MedianReplicas: replicaValue}
						}
					} else {
						workloadKey = utils.GetWorkloadKey(kind, namespace, name)
						workloadKeyVsWorkloadMetrics[workloadKey] = utils.WorkloadMetrics{MedianReplicas: replicaValue}
					}
				}
			}
		}
	}

	return cache, workloadKeyVsWorkloadMetrics, nil
}

func (p *PrometheusProvider) buildBatchPodInfoExpression(namespace string) string {
	template := `max by (namespace, pod, container, node, created_by_kind, created_by_name) (
		kube_pod_info{
			job="kube-state-metrics",
			namespace="%s"
		}
	)`
	return fmt.Sprintf(template, namespace)
}

func (p *PrometheusProvider) buildBatchCPUUsageExpression(namespace string, psiAdjusted bool) string {
	psiAdjustedQuery := ""
	if psiAdjusted {
		psiAdjustedExpression := `
		* on (namespace, pod, container, node) (1 + max by (namespace, pod, container, node) (
			rate(container_pressure_cpu_waiting_seconds_total{container!~"",job="kubelet",namespace="%s"}[%dm])
		))
		`
		psiAdjustedQuery = fmt.Sprintf(psiAdjustedExpression, namespace, RateIntervalMinutes)
	}
	template := `max by (namespace, pod, container, node) (
		rate(container_cpu_usage_seconds_total{container!~"",job="kubelet",namespace="%s"}[%dm])
	)
	%s
	`
	return fmt.Sprintf(template, namespace, RateIntervalMinutes, psiAdjustedQuery)
}

func (p *PrometheusProvider) buildBatchThrottlingAwareCPUExpression(namespace string, psiAdjusted bool) string {
	// throttlingRatio := buildBatchThrottlingRatioExpression(namespace)
	cpuUsage := p.buildBatchCPUUsageExpression(namespace, psiAdjusted)
	podInfo := p.buildBatchPodInfoExpression(namespace)
	// throttledStartupFilter := fmt.Sprintf(`and on (namespace, pod, container) ((time() - kube_pod_container_state_started{job="kube-state-metrics", namespace="%s", container!~""}) >= %d)`, namespace, CPUThrottledLookbackWindow * 60)
	// throttledStartupFilter := ""
	template := `(
		(
			max by (created_by_kind, created_by_name, namespace, container, node) (
				(%s)
				* on (namespace, pod, node) group_left(created_by_kind, created_by_name)
				(%s)
			) or vector(0)
		)
	)`

	return fmt.Sprintf(template,
		cpuUsage, podInfo,
	)
}

func (p *PrometheusProvider) BuildBatchCoreCPUExpression(namespace string, psiAdjusted bool) string {
	throttlingAwareCPU := p.buildBatchThrottlingAwareCPUExpression(namespace, psiAdjusted)

	template := `(max by (created_by_kind, created_by_name, namespace, container) (%s) or vector(0))`

	return fmt.Sprintf(template, throttlingAwareCPU)
}

func (p *PrometheusProvider) EncloseWithinQuantileOverTime(query string, quantileLookbackWindow time.Duration, percentile float64) string {
	template := `quantile_over_time(%.2f, (%s)[%ds:1m])`
	return fmt.Sprintf(template, percentile, query, int(quantileLookbackWindow.Seconds()))
}

func (p *PrometheusProvider) buildBatchCPURecommendationQuery(namespace string, percentile float64, psiAdjusted bool) string {
	coreCPU := p.BuildBatchCoreCPUExpression(namespace, psiAdjusted)

	template := `ceil(
      quantile_over_time(
        %.2f,
        (%s)
        [%s:1m]
      )
      * %.6f
    ) / %.6f`

	return fmt.Sprintf(template,
		percentile,
		coreCPU,
		CPULookbackWindow.String(),
		CPUDecimalScale,
		CPUDecimalScale,
	)
}

func (p *PrometheusProvider) parsePrometheusVectorResultForContainer(result model.Value) (utils.RawBatchResult, error) {
	rawResults := make(utils.RawBatchResult)

	if result.Type() != model.ValVector {
		return nil, fmt.Errorf("expected vector result, got %s", result.Type())
	}
	vector := result.(model.Vector)
	for _, sample := range vector {
		createdByKind := string(sample.Metric["created_by_kind"])
		createdByName := string(sample.Metric["created_by_name"])
		namespace := string(sample.Metric["namespace"])
		containerName := string(sample.Metric["container"])

		if createdByKind == "" || createdByName == "" || namespace == "" || containerName == "" {
			continue
		}

		rawKey := utils.GetWorkloadContainerKey(createdByKind, namespace, createdByName, containerName)
		value := float64(sample.Value)

		rawResults[rawKey] = value
	}

	return rawResults, nil
}

func (p *PrometheusProvider) parsePrometheusVectorResultForWorkload(_ context.Context, result model.Value) (utils.RawBatchResult, error) {
	rawResults := make(utils.RawBatchResult)

	if result.Type() != model.ValVector {
		return nil, fmt.Errorf("expected vector result, got %s", result.Type())
	}
	vector := result.(model.Vector)
	for _, sample := range vector {
		createdByKind := string(sample.Metric["created_by_kind"])
		createdByName := string(sample.Metric["created_by_name"])
		namespace := string(sample.Metric["namespace"])

		if createdByKind == "" || createdByName == "" || namespace == "" {
			continue
		}

		rawKey := utils.GetWorkloadKey(createdByKind, namespace, createdByName)
		value := float64(sample.Value)

		rawResults[rawKey] = value
	}

	return rawResults, nil
}

func (p *PrometheusProvider) BuildBatchMemoryUsageExpression(namespace string) string {
	podInfo := p.buildBatchPodInfoExpression(namespace)

	template := `max by (created_by_kind, created_by_name, namespace, container) (
      container_memory_working_set_bytes{
        job="kubelet",
        namespace="%s",
        container!~""
      }
      * on (namespace, pod, node) group_left(created_by_kind, created_by_name)
      (%s)
    )`

	return fmt.Sprintf(template, namespace, podInfo)
}

func (p *PrometheusProvider) EncloseWithinMemoryCleanupFunction(query string, memoryDecimalPlaces int) string {
	template := `round(
		%s / %.0f,
		%d
	)`
	return fmt.Sprintf(template, query, float64(BytesPerMB), memoryDecimalPlaces)
}

func (p *PrometheusProvider) buildBatchMemoryRecommendationQuery(namespace string, percentile float64) string {
	memoryUsage := p.BuildBatchMemoryUsageExpression(namespace)

	template := `
      quantile_over_time(
        %.2f,
        (%s)
        [%s:1m]
      )`

	return fmt.Sprintf(template,
		percentile,
		memoryUsage,
		MemoryLookbackWindow.String(),
	)
}

func (p *PrometheusProvider) buildBatch7DayMemoryRecommendationQuery(namespace string, percentile float64) string {
	memoryUsage := p.BuildBatchMemoryUsageExpression(namespace)

	template := `
      quantile_over_time(
        %.2f,
        (%s)
        [%s:1m]
      )`

	return fmt.Sprintf(template,
		percentile,
		memoryUsage,
		Memory7DayLookbackWindow.String(),
	)
}

func (p *PrometheusProvider) buildBatch7DayCPURecommendationQuery(namespace string, percentile float64, psiAdjusted bool) string {
	coreCPU := p.BuildBatchCoreCPUExpression(namespace, psiAdjusted)

	template := `ceil(
      quantile_over_time(
        %.2f,
        (%s)
        [%s:1m]
      )
      * %.6f
    ) / %.6f`

	return fmt.Sprintf(template,
		percentile,
		coreCPU,
		CPU7DayLookbackWindow.String(),
		CPUDecimalScale,
		CPUDecimalScale,
	)
}

func (p *PrometheusProvider) buildBatchReplicaCountQuery(namespace string) string {
	podInfo := p.buildBatchPodInfoExpression(namespace)

	template := `count by (created_by_kind, created_by_name, namespace) (%s)`

	return fmt.Sprintf(template, podInfo)
}

func (p *PrometheusProvider) buildBatchOOMMemoryQuery(namespace string) string {
	oomExpression := p.buildBatchOOMMemoryExpression(namespace)

	template := `max_over_time(
		(%s)
		[%s:1m]
	)`

	return fmt.Sprintf(template, oomExpression, MemoryLookbackWindow.String())
}

func (p *PrometheusProvider) buildBatchOOMMemoryExpression(namespace string) string {
	podInfo := p.buildBatchPodInfoExpression(namespace)

	template := `max by (created_by_kind, created_by_name, namespace, container) (
		(
			sum by (namespace, pod, container, node) (
				kube_pod_container_resource_limits{job="kube-state-metrics",namespace="%s"}
			)
			* on (namespace, pod, container) group_right(node)
			(
				sum by (namespace, pod, container) (
					kube_pod_container_status_last_terminated_reason{job="kube-state-metrics",namespace="%s",reason="OOMKilled"}
					* on (namespace, pod, container)
					sum by (namespace, pod, container) (
						increase(
							kube_pod_container_status_restarts_total{job="kube-state-metrics",namespace="%s"}[1m]
						)
					)
				)
				>
				bool 0
			)
		)
		* on (namespace, pod, node) group_left(created_by_kind, created_by_name)
		(%s)
	)`

	return fmt.Sprintf(template, namespace, namespace, namespace, podInfo)
}

func BuildBatchMemoryUsageExpression(namespace string) string {
	podInfo := buildBatchPodInfoExpression(namespace)

	template := `max by (created_by_kind, created_by_name, namespace, container) (
      container_memory_working_set_bytes{
        job="kubelet",
        namespace="%s",
        container!~""
      }
      * on (namespace, pod, node) group_left(created_by_kind, created_by_name)
      (%s)
    )`

	return fmt.Sprintf(template, namespace, podInfo)
}

func buildBatchPodInfoExpression(namespace string) string {
	template := `max by (namespace, pod, container, node, created_by_kind, created_by_name) (
		kube_pod_info{
			job="kube-state-metrics",
			namespace="%s"
		}
	)`
	return fmt.Sprintf(template, namespace)
}
