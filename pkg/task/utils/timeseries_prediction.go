package utils

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/truefoundry/cruiseKube/pkg/logging"

	"github.com/prometheus/common/model"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const timeStepSize = 60 * time.Minute

// TimeSeriesRequest represents the request payload for the time series model
type TimeSeriesRequest struct {
	TimeSeriesData  []map[string][]float64 `json:"timeseries_data"`
	ForecastHorizon int                    `json:"forecast_horizon"`
}

func callTimeSeriesModel(ctx context.Context, timeseriesData []map[string][]float64, forecastHorizon int) (PredictionStatsResponse, error) {
	// Create request payload
	logging.Infof(ctx, "Calling time series model with forecast horizon: %d", forecastHorizon)
	requestPayload := TimeSeriesRequest{
		TimeSeriesData:  timeseriesData,
		ForecastHorizon: forecastHorizon,
	}

	payloadBytes, err := json.Marshal(requestPayload)
	if err != nil {
		return PredictionStatsResponse{}, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout:   300 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	// Make HTTP POST request
	url := ""
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return PredictionStatsResponse{}, fmt.Errorf("failed to make HTTP request to time series model: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	logging.Infof(ctx, "Response from time series model for query: %d", resp.StatusCode)

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return PredictionStatsResponse{}, fmt.Errorf("time series model returned status code %d", resp.StatusCode)
	}

	// Parse response
	var response PredictionStatsResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return PredictionStatsResponse{}, fmt.Errorf("failed to parse response from time series model: %w", err)
	}

	return response, nil
}

// callTimeSeriesModelInParallel processes multiple samples in parallel with concurrency limit
func callTimeSeriesModelInParallel(ctx context.Context, matrix model.Matrix, namespace string, maxConcurrency int) map[string]WorkloadPrediction {
	semaphore := make(chan struct{}, maxConcurrency)
	results := make(chan struct {
		key        string
		prediction WorkloadPrediction
		err        error
	}, len(matrix))

	var wg sync.WaitGroup

	// Combine replicasets into deployments
	combinedMatrix := combineReplicasetsToDeployments(matrix)

	// Launch goroutines for each sample
	for _, sample := range combinedMatrix {
		wg.Add(1)
		go func(sample *model.SampleStream) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			var values []float64
			for _, v := range sample.Values {
				values = append(values, float64(v.Value))
			}
			timeSeriesData := append([]map[string][]float64{}, map[string][]float64{"cpu": values})

			workloadContainerKey := GetWorkloadContainerKey(
				string(sample.Metric["created_by_kind"]),
				namespace,
				string(sample.Metric["created_by_name"]),
				string(sample.Metric["container"]))

			predictionStat, err := callTimeSeriesModel(ctx, timeSeriesData, 1)
			if err != nil {
				logging.Errorf(ctx, "Error predicting stats from time series model for namespace %s, key %s: %v", namespace, workloadContainerKey, err)
				results <- struct {
					key        string
					prediction WorkloadPrediction
					err        error
				}{key: workloadContainerKey, err: err}
				return
			}

			workloadMedian := predictionStat.Median[0][0][0]
			workloadP90 := predictionStat.P90[0][0][0]
			workloadP95 := predictionStat.P95[0][0][0]
			workloadP99 := predictionStat.P99[0][0][0]

			results <- struct {
				key        string
				prediction WorkloadPrediction
				err        error
			}{
				key: workloadContainerKey,
				prediction: WorkloadPrediction{
					Median: workloadMedian,
					P90:    workloadP90,
					P95:    workloadP95,
					P99:    workloadP99,
				},
			}
		}(sample)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	workloadContainerKeyVsPrediction := make(map[string]WorkloadPrediction)
	for result := range results {
		if result.err == nil {
			workloadContainerKeyVsPrediction[result.key] = result.prediction
		}
	}

	return workloadContainerKeyVsPrediction
}

func callMemoryTimeSeriesModelInParallel(ctx context.Context, matrix model.Matrix, namespace string, maxConcurrency int) map[string]WorkloadPrediction {
	semaphore := make(chan struct{}, maxConcurrency)
	results := make(chan struct {
		key        string
		prediction WorkloadPrediction
		err        error
	}, len(matrix))

	var wg sync.WaitGroup

	combinedMatrix := combineReplicasetsToDeployments(matrix)

	for _, sample := range combinedMatrix {
		wg.Add(1)
		go func(sample *model.SampleStream) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			var values []float64
			for _, v := range sample.Values {
				values = append(values, float64(v.Value))
			}
			timeSeriesData := append([]map[string][]float64{}, map[string][]float64{"memory": values})

			workloadContainerKey := GetWorkloadContainerKey(
				string(sample.Metric["created_by_kind"]),
				namespace,
				string(sample.Metric["created_by_name"]),
				string(sample.Metric["container"]))

			predictionStat, err := callTimeSeriesModel(ctx, timeSeriesData, 1)
			if err != nil {
				logging.Errorf(ctx, "Error predicting memory stats from time series model for namespace %s, key %s: %v", namespace, workloadContainerKey, err)
				results <- struct {
					key        string
					prediction WorkloadPrediction
					err        error
				}{key: workloadContainerKey, err: err}
				return
			}

			workloadMedian := predictionStat.Median[0][0][0]
			workloadP90 := predictionStat.P90[0][0][0]
			workloadP95 := predictionStat.P95[0][0][0]
			workloadP99 := predictionStat.P99[0][0][0]

			results <- struct {
				key        string
				prediction WorkloadPrediction
				err        error
			}{
				key: workloadContainerKey,
				prediction: WorkloadPrediction{
					Median: workloadMedian,
					P90:    workloadP90,
					P95:    workloadP95,
					P99:    workloadP99,
				},
			}
		}(sample)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	workloadContainerKeyVsPrediction := make(map[string]WorkloadPrediction)
	for result := range results {
		if result.err == nil {
			workloadContainerKeyVsPrediction[result.key] = result.prediction
		}
	}

	return workloadContainerKeyVsPrediction
}

func combineReplicasetsToDeployments(matrix model.Matrix) model.Matrix {
	deploymentGroups := make(map[string][]*model.SampleStream)
	var combinedMatrix model.Matrix

	for _, sample := range matrix {
		if string(sample.Metric["created_by_kind"]) != ReplicaSetKind {
			combinedMatrix = append(combinedMatrix, sample)
			continue
		}

		replicasetName := string(sample.Metric["created_by_name"])
		lastDash := strings.LastIndex(replicasetName, "-")
		deploymentName := replicasetName[:lastDash]

		groupKey := GetWorkloadContainerKey(
			DeploymentKind,
			string(sample.Metric["namespace"]),
			deploymentName,
			string(sample.Metric["container"]))

		deploymentGroups[groupKey] = append(deploymentGroups[groupKey], sample)
	}

	for _, samples := range deploymentGroups {
		if len(samples) == 0 {
			continue
		}

		templateSample := samples[0]
		replicasetName := string(templateSample.Metric["created_by_name"])
		lastDash := strings.LastIndex(replicasetName, "-")
		deploymentName := replicasetName[:lastDash]

		newMetric := make(model.Metric)
		maps.Copy(newMetric, templateSample.Metric)
		newMetric["created_by_kind"] = DeploymentKind
		newMetric["created_by_name"] = model.LabelValue(deploymentName)

		combinedValues := combineTimeSeriesByMax(samples)

		newSample := &model.SampleStream{
			Metric: newMetric,
			Values: combinedValues,
		}

		combinedMatrix = append(combinedMatrix, newSample)
	}

	return combinedMatrix
}

func combineTimeSeriesByMax(samples []*model.SampleStream) []model.SamplePair {
	if len(samples) == 0 {
		return nil
	}

	maxValuesByTime := make(map[model.Time]model.SampleValue)
	for _, sample := range samples {
		for _, pair := range sample.Values {
			if existing, exists := maxValuesByTime[pair.Timestamp]; !exists || pair.Value > existing {
				maxValuesByTime[pair.Timestamp] = pair.Value
			}
		}
	}

	result := make([]model.SamplePair, 0, len(maxValuesByTime))
	for timestamp, value := range maxValuesByTime {
		result = append(result, model.SamplePair{Timestamp: timestamp, Value: value})
	}

	slices.SortFunc(result, func(a, b model.SamplePair) int {
		return cmp.Compare(a.Timestamp, b.Timestamp)
	})

	return result
}

// returns namespace -> workload -> prediction
func PredictCPUStatsFromTimeSeriesModel(ctx context.Context, namespaces []string, promClient v1.API, psiAdjusted bool) (map[string]map[string]WorkloadPrediction, error) {
	logging.Infof(ctx, "Predicting stats from time series model for namespaces: %v with psiAdjusted: %v", namespaces, psiAdjusted)
	result := make(map[string]map[string]WorkloadPrediction)
	for _, namespace := range namespaces {
		query := EncloseWithinQuantileOverTime(
			BuildBatchCoreCPUExpression(namespace, psiAdjusted),
			timeStepSize,
			1.0,
		)

		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		end := time.Now()
		start := end.Add(-MLLookbackWindow)
		r := v1.Range{
			Start: start,
			End:   end,
			Step:  timeStepSize,
		}

		logging.Infof(ctx, "Querying prometheus with query: %s", CompressQueryForLogging(query))
		val, _, err := promClient.QueryRange(ctx, query, r)
		if err != nil {
			fmt.Printf("Error querying prometheus: %v\n", err)
			return nil, err
		}

		matrix, ok := val.(model.Matrix)
		if !ok {
			logging.Errorf(ctx, "Unable to convert to matrix")
			continue
		}

		workloadContainerKeyVsPrediction := callTimeSeriesModelInParallel(ctx, matrix, namespace, 20)
		result[namespace] = workloadContainerKeyVsPrediction
	}
	return result, nil
}

func PredictMemoryStatsFromTimeSeriesModel(ctx context.Context, namespaces []string, promClient v1.API) (map[string]map[string]WorkloadPrediction, error) {
	logging.Info(ctx, "Predicting memory stats from time series model for namespaces: ", namespaces)
	result := make(map[string]map[string]WorkloadPrediction)
	for _, namespace := range namespaces {
		query := EncloseWithinMemoryCleanupFunction(EncloseWithinQuantileOverTime(
			BuildBatchMemoryUsageExpression(namespace),
			timeStepSize,
			1.0,
		), MemoryDecimalPlaces)

		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		end := time.Now()
		start := end.Add(-MLLookbackWindow)
		r := v1.Range{
			Start: start,
			End:   end,
			Step:  timeStepSize,
		}

		logging.Infof(ctx, "Querying prometheus for memory with query: %s", CompressQueryForLogging(query))
		val, _, err := promClient.QueryRange(ctx, query, r)
		if err != nil {
			fmt.Printf("Error querying prometheus for memory: %v\n", err)
			return nil, err
		}

		matrix, ok := val.(model.Matrix)
		if !ok {
			logging.Errorf(ctx, "Unable to convert to matrix for memory")
			continue
		}

		workloadContainerKeyVsPrediction := callMemoryTimeSeriesModelInParallel(ctx, matrix, namespace, 20)
		result[namespace] = workloadContainerKeyVsPrediction
	}
	return result, nil
}

func EncloseWithinMemoryCleanupFunction(query string, memoryDecimalPlaces int) string {
	template := `round(
		%s / %.0f,
		%d
	)`
	return fmt.Sprintf(template, query, float64(BytesPerMB), memoryDecimalPlaces)
}

func computeSimpleTimeSeriesPredictions(ctx context.Context, timeseriesData []SimpleTimeSeriesData) []SimplePredictionResponse {
	logging.Infof(ctx, "Computing simple time series predictions for %d entities", len(timeseriesData))

	var results []SimplePredictionResponse
	currentTime := time.Now().UTC()
	currentHour := currentTime.Hour()
	currentWeekday := currentTime.Weekday()

	for idx, entity := range timeseriesData {
		entityName := entity.EntityName
		if entityName == "" {
			entityName = fmt.Sprintf("entity_%d", idx)
		}

		timestamps := entity.Timestamps
		values := entity.Values
		if len(timestamps) != len(values) {
			logging.Warnf(ctx, "Timestamp and values length mismatch for entity %s (timestamps=%d, values=%d)", entityName, len(timestamps), len(values))
			continue
		}

		parsed := make([]time.Time, 0, len(timestamps))
		for _, ts := range timestamps {
			t, err := time.Parse("2006-01-02T15:04:05", ts)
			if err != nil {
				logging.Errorf(ctx, "Could not parse timestamp '%s' for entity %s: %v", ts, entityName, err)
				continue
			}
			parsed = append(parsed, t.UTC())
		}

		if len(parsed) != len(values) {
			logging.Warnf(ctx, "Failed to parse all timestamps for entity %s (parsed=%d, values=%d)", entityName, len(parsed), len(values))
			continue
		}

		weeklyPrediction := 0.0
		for i, t := range parsed {
			if t.Weekday() == currentWeekday && t.Hour() == currentHour {
				weeklyPrediction = max(weeklyPrediction, values[i])
			}
		}

		hourlyPrediction := 0.0
		for i, t := range parsed {
			if t.Hour() == currentHour {
				hourlyPrediction = max(hourlyPrediction, values[i])
			}
		}

		currentPrediction := 0.0
		oneHourAgo := currentTime.Add(-1 * time.Hour)
		for i, t := range parsed {
			if !t.Before(oneHourAgo) {
				currentPrediction = max(currentPrediction, values[i])
			}
		}

		maxValue := max(weeklyPrediction, hourlyPrediction, currentPrediction)

		results = append(results, SimplePredictionResponse{
			EntityName:        entityName,
			WeeklyPrediction:  weeklyPrediction,
			HourlyPrediction:  hourlyPrediction,
			CurrentPrediction: currentPrediction,
			MaxValue:          maxValue,
		})
	}

	logging.Infof(ctx, "Generated simple predictions for %d entities", len(results))
	return results
}

func convertMatrixToSimpleTimeSeriesData(matrix model.Matrix) []SimpleTimeSeriesData {
	var result []SimpleTimeSeriesData
	combinedMatrix := combineReplicasetsToDeployments(matrix)

	for _, sample := range combinedMatrix {
		workloadContainerKey := GetWorkloadContainerKey(
			string(sample.Metric["created_by_kind"]),
			string(sample.Metric["namespace"]),
			string(sample.Metric["created_by_name"]),
			string(sample.Metric["container"]))

		var timestamps []string
		var values []float64
		for _, v := range sample.Values {
			timestamps = append(timestamps, v.Timestamp.Time().UTC().Format("2006-01-02T15:04:05"))
			values = append(values, float64(v.Value))
		}

		if len(values) > 0 {
			result = append(result, SimpleTimeSeriesData{
				EntityName: workloadContainerKey,
				Timestamps: timestamps,
				Values:     values,
			})
		}
	}
	return result
}

func PredictSimpleStatsFromTimeSeriesModel(ctx context.Context, namespaces []string, promClient v1.API, resourceType string, isPSIEnabled bool) (map[string]map[string]SimplePrediction, error) {
	logging.Infof(ctx, "Predicting simple %s stats from time series model for namespaces: %v", resourceType, namespaces)
	result := make(map[string]map[string]SimplePrediction)

	for _, namespace := range namespaces {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		end := time.Now()
		start := end.Add(-MLLookbackWindow)
		r := v1.Range{
			Start: start,
			End:   end,
			Step:  timeStepSize,
		}

		var matrix model.Matrix
		var err error

		if resourceType == "memory" {
			matrix, err = fetchMemoryWithOOM(ctx, promClient, namespace, r)
			if err != nil {
				logging.Errorf(ctx, "Error fetching memory with OOM for namespace %s: %v", namespace, err)
				return nil, err
			}
		} else {
			var query string
			switch resourceType {
			case "cpu":
				query = EncloseWithinQuantileOverTime(
					BuildBatchCoreCPUExpression(namespace, isPSIEnabled),
					timeStepSize,
					1.0,
				)
			default:
				logging.Errorf(ctx, "Invalid resource type: %s", resourceType)
				continue
			}

			logging.Infof(ctx, "Querying prometheus for simple %s predictions with query: %s", resourceType, CompressQueryForLogging(query))
			val, _, err := promClient.QueryRange(ctx, query, r)
			if err != nil {
				logging.Errorf(ctx, "Error querying prometheus for simple %s predictions: %v", resourceType, err)
				return nil, err
			}

			var ok bool
			matrix, ok = val.(model.Matrix)
			if !ok {
				logging.Errorf(ctx, "Unable to convert to matrix for simple %s predictions", resourceType)
				return nil, fmt.Errorf("unable to convert to matrix for simple %s predictions", resourceType)
			}
		}

		timeseriesData := convertMatrixToSimpleTimeSeriesData(matrix)
		if len(timeseriesData) == 0 {
			logging.Infof(ctx, "No timeseries data for namespace %s", namespace)
			continue
		}

		predictions := computeSimpleTimeSeriesPredictions(ctx, timeseriesData)

		namespaceResult := make(map[string]SimplePrediction)
		for _, pred := range predictions {
			namespaceResult[pred.EntityName] = SimplePrediction{
				WeeklyPrediction:  pred.WeeklyPrediction,
				HourlyPrediction:  pred.HourlyPrediction,
				CurrentPrediction: pred.CurrentPrediction,
				MaxValue:          pred.MaxValue,
			}
		}
		result[namespace] = namespaceResult
	}

	return result, nil
}

func fetchMemoryWithOOM(ctx context.Context, promClient v1.API, namespace string, r v1.Range) (model.Matrix, error) {
	memoryQuery := EncloseWithinMemoryCleanupFunction(EncloseWithinQuantileOverTime(
		BuildBatchMemoryUsageExpression(namespace),
		timeStepSize,
		1.0,
	), MemoryDecimalPlaces)

	oomQuery := EncloseWithinMemoryCleanupFunction(EncloseWithinQuantileOverTime(
		BuildBatchOOMMemoryExpression(namespace),
		timeStepSize,
		1.0,
	), MemoryDecimalPlaces)

	logging.Infof(ctx, "Querying prometheus for memory predictions with query: %s", CompressQueryForLogging(memoryQuery))
	memoryVal, _, err := promClient.QueryRange(ctx, memoryQuery, r)
	if err != nil {
		return nil, fmt.Errorf("error querying prometheus for memory predictions: %w", err)
	}

	logging.Infof(ctx, "Querying prometheus for OOM memory predictions with query: %s", CompressQueryForLogging(oomQuery))
	oomVal, _, err := promClient.QueryRange(ctx, oomQuery, r)
	if err != nil {
		return nil, fmt.Errorf("error querying prometheus for OOM memory predictions: %w, query: %s", err, oomQuery)
	}

	memoryMatrix, ok := memoryVal.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("unable to convert memory result to matrix")
	}

	oomMatrix, ok := oomVal.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("unable to convert OOM memory result to matrix")
	}

	mergedMatrix := mergeMemoryAndOOMTimeSeries(memoryMatrix, oomMatrix)
	return mergedMatrix, nil
}

func mergeMemoryAndOOMTimeSeries(memoryMatrix, oomMatrix model.Matrix) model.Matrix {
	oomMap := make(map[string]*model.SampleStream)
	for _, sample := range oomMatrix {
		key := getSampleKey(sample)
		oomMap[key] = sample
	}

	var mergedMatrix model.Matrix
	for _, memorySample := range memoryMatrix {
		key := getSampleKey(memorySample)
		oomSample, exists := oomMap[key]

		if !exists {
			mergedMatrix = append(mergedMatrix, memorySample)
			continue
		}

		mergedSample := mergeSampleStreams(memorySample, oomSample)
		mergedMatrix = append(mergedMatrix, mergedSample)
	}

	for _, oomSample := range oomMatrix {
		key := getSampleKey(oomSample)
		found := false
		for _, memorySample := range memoryMatrix {
			if getSampleKey(memorySample) == key {
				found = true
				break
			}
		}
		if !found {
			mergedMatrix = append(mergedMatrix, oomSample)
		}
	}

	return mergedMatrix
}

func getSampleKey(sample *model.SampleStream) string {
	kind := string(sample.Metric["created_by_kind"])
	namespace := string(sample.Metric["namespace"])
	name := string(sample.Metric["created_by_name"])
	container := string(sample.Metric["container"])
	return fmt.Sprintf("%s:%s:%s:%s", kind, namespace, name, container)
}

func mergeSampleStreams(memorySample, oomSample *model.SampleStream) *model.SampleStream {
	mergedValues := make([]model.SamplePair, 0)
	memoryMap := make(map[model.Time]model.SampleValue)
	oomMap := make(map[model.Time]model.SampleValue)

	for _, pair := range memorySample.Values {
		memoryMap[pair.Timestamp] = pair.Value
	}

	for _, pair := range oomSample.Values {
		oomMap[pair.Timestamp] = pair.Value
	}

	allTimestamps := make(map[model.Time]bool)
	for ts := range memoryMap {
		allTimestamps[ts] = true
	}
	for ts := range oomMap {
		allTimestamps[ts] = true
	}

	for ts := range allTimestamps {
		memoryVal, memoryExists := memoryMap[ts]
		oomVal, oomExists := oomMap[ts]

		var finalValue model.SampleValue
		switch {
		case memoryExists && oomExists:
			finalValue = model.SampleValue(max(float64(memoryVal), float64(oomVal)))
		case memoryExists:
			finalValue = memoryVal
		default:
			finalValue = oomVal
		}

		mergedValues = append(mergedValues, model.SamplePair{
			Timestamp: ts,
			Value:     finalValue,
		})
	}

	slices.SortFunc(mergedValues, func(a, b model.SamplePair) int {
		return cmp.Compare(a.Timestamp, b.Timestamp)
	})

	return &model.SampleStream{
		Metric: memorySample.Metric,
		Values: mergedValues,
	}
}
