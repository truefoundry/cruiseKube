package utils

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/truefoundry/cruisekube/pkg/logging"

	"github.com/prometheus/common/model"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

const timeStepSize = 60 * time.Minute

// TimeSeriesRequest represents the request payload for the time series model
type TimeSeriesRequest struct {
	TimeSeriesData  []map[string][]float64 `json:"timeseries_data"`
	ForecastHorizon int                    `json:"forecast_horizon"`
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
			matrix, err = fetchMemoryMatrix(ctx, promClient, namespace, r)
			if err != nil {
				logging.Errorf(ctx, "Error fetching memory matrix for namespace %s: %v", namespace, err)
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
				return nil, fmt.Errorf("failed to query prometheus for simple %s predictions: %w", resourceType, err)
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

func fetchMemoryMatrix(ctx context.Context, promClient v1.API, namespace string, r v1.Range) (model.Matrix, error) {
	memoryQuery := EncloseWithinMemoryCleanupFunction(EncloseWithinQuantileOverTime(
		BuildBatchMemoryUsageExpression(namespace),
		timeStepSize,
		1.0,
	), MemoryDecimalPlaces)

	logging.Infof(ctx, "Querying prometheus for memory predictions with query: %s", CompressQueryForLogging(memoryQuery))
	memoryVal, _, err := promClient.QueryRange(ctx, memoryQuery, r)
	if err != nil {
		return nil, fmt.Errorf("error querying prometheus for memory predictions: %w", err)
	}

	memoryMatrix, ok := memoryVal.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("unable to convert memory result to matrix")
	}

	return memoryMatrix, nil
}
