package task

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/common/model"
	"github.com/truefoundry/cruiseKube/pkg/adapters/metricsProvider/prometheus"
	"github.com/truefoundry/cruiseKube/pkg/config"
	"github.com/truefoundry/cruiseKube/pkg/contextutils"
	"github.com/truefoundry/cruiseKube/pkg/logging"
	"github.com/truefoundry/cruiseKube/pkg/repository/storage"
	"github.com/truefoundry/cruiseKube/pkg/task/utils"
	"github.com/truefoundry/cruiseKube/pkg/types"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type CreateStatsMetadata struct {
	SkipMemory bool `yaml:"skipMemory" json:"skipMemory" mapstructure:"skipMemory"`
}

type CreateStatsTaskConfig struct {
	Name                       string
	Enabled                    bool
	Schedule                   string
	ClusterID                  string
	TargetClusterID            string
	TargetNamespace            string
	RecentStatsLookbackMinutes int
	TimeStepSize               time.Duration
	MLLookbackWindow           time.Duration
	Metadata                   CreateStatsMetadata
}

type CreateStatsTask struct {
	kubeClient    *kubernetes.Clientset
	dynamicClient dynamic.Interface
	promClient    *prometheus.PrometheusProvider
	storage       *storage.Storage
	config        *CreateStatsTaskConfig
}

func NewCreateStatsTask(ctx context.Context, kubeClient *kubernetes.Clientset, dynamicClient dynamic.Interface, promClient *prometheus.PrometheusProvider, storage *storage.Storage, config *CreateStatsTaskConfig, taskConfig *config.TaskConfig) *CreateStatsTask {
	var createStatsMetadata CreateStatsMetadata
	if err := taskConfig.ConvertMetadataToStruct(&createStatsMetadata); err != nil {
		logging.Errorf(ctx, "Error converting metadata to struct: %v", err)
		return nil
	}

	config.Metadata = createStatsMetadata
	return &CreateStatsTask{
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
		promClient:    promClient,
		storage:       storage,
		config:        config,
	}
}

func (c *CreateStatsTask) GetCoreTask() any {
	return c
}

func (c *CreateStatsTask) GetName() string {
	return c.config.Name
}

func (c *CreateStatsTask) GetSchedule() string {
	return c.config.Schedule
}

func (c *CreateStatsTask) IsEnabled() bool {
	return c.config.Enabled
}

func (c *CreateStatsTask) Run(ctx context.Context) error {
	ctx = contextutils.WithTask(ctx, c.config.Name)
	ctx = contextutils.WithCluster(ctx, c.config.ClusterID)

	targetNamespace := c.config.TargetNamespace

	startTime := time.Now().UTC()
	logging.Infof(ctx, "Running task: CreateStats")

	workloadList, err := utils.ListAllWorkloads(ctx, c.kubeClient, targetNamespace)
	if err != nil {
		logging.Errorf(ctx, "Error getting workload list: %v", err)
		return err
	}

	isPSIEnabled := c.isPSIEnabled(ctx)
	logging.Infof(ctx, "PSI is enabled: %v", isPSIEnabled)

	uniqueWorkloads := make(map[string]utils.WorkloadInfo)
	filteredCount := 0
	for _, workloadInfo := range workloadList {
		workloadKey := utils.GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)

		hasRecent, err := c.storage.HasRecentStats(c.config.ClusterID, workloadKey, utils.RecentStatsLookbackMinutes)
		if err != nil {
			logging.Errorf(ctx, "Error checking recent stats for workload %s: %v", workloadKey, err)
			continue
		}

		if hasRecent {
			filteredCount++
			continue
		}

		uniqueWorkloads[workloadKey] = workloadInfo
	}

	logging.Infof(ctx, "Filtered out %d workloads with recent stats (within 30 minutes)", filteredCount)
	namespaces := utils.ExtractUniqueNamespaces(uniqueWorkloads)
	logging.Infof(ctx, "Found %d unique namespaces to process: %v", len(namespaces), namespaces)

	namespaceQueryResults, namespaceVsWorkloadMetrics, err := c.promClient.FetchStatsForNamespaces(ctx, c.config.ClusterID, namespaces, isPSIEnabled)
	if err != nil {
		logging.Errorf(ctx, "Error executing batch queries: %v", err)
		return err
	}

	// namespaceVsWorkloadPredictions, err := PredictCPUStatsFromTimeSeriesModel(namespaces, promClient, false)
	// if err != nil {
	// 	log.Printf("Error predicting stats from time series model: %v", err)
	// }
	// namespaceVsWorkloadPredictionsPSIAdjusted, err := PredictCPUStatsFromTimeSeriesModel(namespaces, promClient, true)
	// if err != nil {
	// 	log.Printf("Error predicting stats from time series model: %v", err)
	// }
	// namespaceVsWorkloadMemoryPredictions, err := PredictMemoryStatsFromTimeSeriesModel(namespaces, promClient)
	// if err != nil {
	// 	log.Printf("Error predicting memory stats from time series model: %v", err)
	// }

	namespaceVsSimpleCPUPredictions, err := utils.PredictSimpleStatsFromTimeSeriesModel(ctx, namespaces, c.promClient.GetClient(), "cpu", isPSIEnabled)
	if err != nil {
		logging.Errorf(ctx, "Error predicting simple CPU stats from time series model: %v", err)
		return err
	}
	var namespaceVsSimpleMemoryPredictions map[string]map[string]utils.SimplePrediction
	if !c.config.Metadata.SkipMemory {
		namespaceVsSimpleMemoryPredictions, err = utils.PredictSimpleStatsFromTimeSeriesModel(ctx, namespaces, c.promClient.GetClient(), "memory", isPSIEnabled)
		if err != nil {
			logging.Errorf(ctx, "Error predicting simple memory stats from time series model: %v", err)
			return err
		}
	}

	workloadHpaCpuMap := make(map[string]bool)
	for key := range uniqueWorkloads {
		workloadHpaCpuMap[key] = false
	}

	if c.dynamicClient != nil {
		err = utils.CheckHPAOnCPU(ctx, c.dynamicClient, targetNamespace, workloadHpaCpuMap)
		if err != nil {
			logging.Errorf(ctx, "Error checking HPA status: %v", err)
			return err
		}
	}

	pdbCache, err := utils.FetchPDBsForNamespaces(ctx, c.kubeClient, namespaces)
	if err != nil {
		logging.Errorf(ctx, "Error fetching PDBs: %v", err)
		return err
	}

	// namespaceVsWorkloadPredictions := map[string]map[string]contextutils.WorkloadPrediction{}

	var newStats []*utils.WorkloadStat
	for _, workloadInfo := range uniqueWorkloads {
		if stat := c.prepareStatsFromMetrics(
			ctx,
			c.kubeClient,
			workloadInfo,
			namespaceQueryResults,
			namespaceVsWorkloadMetrics,
			// namespaceVsWorkloadPredictions[workloadInfo.Namespace],
			// namespaceVsWorkloadPredictionsPSIAdjusted[workloadInfo.Namespace],
			// namespaceVsWorkloadMemoryPredictions[workloadInfo.Namespace],
			map[string]utils.WorkloadPrediction{},
			map[string]utils.WorkloadPrediction{},
			map[string]utils.WorkloadPrediction{},
			namespaceVsSimpleCPUPredictions[workloadInfo.Namespace],
			namespaceVsSimpleMemoryPredictions[workloadInfo.Namespace],
			workloadHpaCpuMap,
			c.dynamicClient,
			pdbCache,
		); stat != nil {
			newStats = append(newStats, stat)
		}
	}

	if len(newStats) > 0 {
		if err := c.writeStatsToFile(ctx, c.config.ClusterID, newStats, startTime); err != nil {
			logging.Errorf(ctx, "Error writing stats to file: %v", err)
			return err
		}
	}

	logging.Infof(ctx, "Task completed in %v", time.Since(startTime))
	return nil
}

func (c *CreateStatsTask) isPSIEnabled(ctx context.Context) bool {
	query := "max(max_over_time(container_pressure_cpu_waiting_seconds_total[1m]))"

	result, _, err := c.promClient.ExecuteQueryWithRetry(ctx, c.config.ClusterID, query, "PSI_CHECK")
	if err != nil {
		logging.Errorf(ctx, "[isPSIEnabled] Error executing PSI query: %v", err)
		return false
	}

	vector := result.(model.Vector)
	return len(vector) != 0
}

func (c *CreateStatsTask) writeStatsToFile(ctx context.Context, clusterID string, newStats []*utils.WorkloadStat, generatedAt time.Time) error {
	return c.writeStatsToFileForCluster(ctx, clusterID, newStats, generatedAt)
}

func (c *CreateStatsTask) writeStatsToFileForCluster(ctx context.Context, clusterID string, newStats []*utils.WorkloadStat, generatedAt time.Time) error {
	var allStats = utils.StatsResponse{}
	for _, stat := range newStats {
		allStats.Stats = append(allStats.Stats, *stat)
	}

	if err := c.storage.WriteClusterStats(clusterID, allStats, generatedAt); err != nil {
		return fmt.Errorf("error writing stats for cluster %s: %w", clusterID, err)
	}

	logging.Infof(ctx, "Successfully wrote %d stats for cluster %s", len(allStats.Stats), clusterID)
	return nil
}

func (c *CreateStatsTask) prepareStatsFromMetrics(
	ctx context.Context,
	kubeClient *kubernetes.Clientset,
	workloadInfo utils.WorkloadInfo,
	nsVsContainerMetrics utils.NamespaceVsContainerMetrics,
	nsVsWorkloadMetrics utils.NamespaceVsWorkloadMetrics,
	containerKeyVsCPUPrediction map[string]utils.WorkloadPrediction,
	containerKeyVsCPUPredictionPSIAdjusted map[string]utils.WorkloadPrediction,
	containerKeyVsMemoryPrediction map[string]utils.WorkloadPrediction,
	workloadContainerKeyVsSimpleCPUPrediction map[string]utils.SimplePrediction,
	workloadContainerKeyVsSimpleMemoryPrediction map[string]utils.SimplePrediction,
	workloadHPAMap map[string]bool,
	dynamicClient dynamic.Interface,
	pdbCache map[string][]policyv1.PodDisruptionBudget,
) *utils.WorkloadStat {
	workloadKeyVsContainerMetrics, exists := nsVsContainerMetrics[workloadInfo.Namespace]
	if !exists {
		logging.Errorf(ctx, "No batch cache found for namespace %s", workloadInfo.Namespace)
		return nil
	}
	workloadKey := utils.GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)
	_, exists = workloadKeyVsContainerMetrics[workloadKey]
	if !exists {
		return nil
	}

	workloadObj, err := utils.GetWorkloadObject(ctx, kubeClient, workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)
	if err != nil {
		logging.Errorf(ctx, "Error getting workload %s: %v", workloadKey, err)
		return nil
	}

	containerSpecs := workloadObj.GetContainerSpecs(ctx, kubeClient)
	initContainerSpecs := workloadObj.GetInitContainerSpecs(ctx, kubeClient)

	containerResources := c.getAllContainerResourcesFromContainerSpecs(ctx, containerSpecs, initContainerSpecs)

	workloadStat := utils.BuildContainerStatFromCache(ctx, workloadInfo, workloadKeyVsContainerMetrics, containerResources)
	if workloadStat == nil {
		logging.Errorf(ctx, "Could not build container stat for %s", workloadKey)
		return nil
	}

	workloadStat.CreationTime = workloadObj.GetCreationTime()
	workloadStat.UpdatedAt = time.Now()

	// Check if this workload is horizontally autoscaled on CPU
	if workloadHPAMap != nil {
		if isAutoscaled, exists := workloadHPAMap[workloadKey]; exists && isAutoscaled {
			workloadStat.IsHorizontallyAutoscaledOnCPU = true
		}
	}

	// Detect workload constraints
	if dynamicClient != nil {
		constraints, err := utils.DetectWorkloadConstraints(ctx, kubeClient, dynamicClient, workloadObj, pdbCache)
		if err != nil {
			logging.Errorf(ctx, "Error detecting constraints for workload %s: %v", workloadKey, err)
		} else {
			workloadStat.Constraints = constraints
		}
	}

	for i := range workloadStat.ContainerStats {
		containerStat := &workloadStat.ContainerStats[i]

		var mlPredictedCPU *utils.MLPercentilesCPU
		var mlPredictedMemory *utils.MLPercentilesMemory

		workloadContainerKey := utils.GetWorkloadContainerKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name, containerStat.ContainerName)

		containerCPUPrediction, exists := containerKeyVsCPUPrediction[workloadContainerKey]
		if !exists {
			// logging.Infof(ctx, "Error: No ML CPU prediction found for %s", workloadContainerKey)
		} else {
			mlPredictedCPU = &utils.MLPercentilesCPU{
				Median: containerCPUPrediction.Median,
				P90:    containerCPUPrediction.P90,
				P95:    containerCPUPrediction.P95,
				P99:    containerCPUPrediction.P99,
			}
			containerStat.MLPercentilesCPU = mlPredictedCPU
		}
		containerPredictionPSIAdjusted, psiAdjustedExists := containerKeyVsCPUPredictionPSIAdjusted[workloadContainerKey]
		if !psiAdjustedExists {
			// logging.Infof(ctx, "Error: No ML CPU prediction (PSI adjusted) found for %s", workloadContainerKey)
		} else {
			mlPercentilesCPUPSIAdjusted := &utils.MLPercentilesCPUPSIAdjusted{
				Median: containerPredictionPSIAdjusted.Median,
				P90:    containerPredictionPSIAdjusted.P90,
				P95:    containerPredictionPSIAdjusted.P95,
				P99:    containerPredictionPSIAdjusted.P99,
			}
			containerStat.MLPercentilesCPUPSIAdjusted = mlPercentilesCPUPSIAdjusted
		}

		containerMemoryPrediction, memoryExists := containerKeyVsMemoryPrediction[workloadContainerKey]
		if !memoryExists {
			// logging.Infof(ctx, "Error: No ML memory prediction found for %s", workloadContainerKey)
		} else {
			mlPredictedMemory = &utils.MLPercentilesMemory{
				Median: containerMemoryPrediction.Median,
				P90:    containerMemoryPrediction.P90,
				P95:    containerMemoryPrediction.P95,
				P99:    containerMemoryPrediction.P99,
			}
		}
		containerStat.MLPercentilesMemory = mlPredictedMemory

		if simpleCPUPrediction, exists := workloadContainerKeyVsSimpleCPUPrediction[workloadContainerKey]; exists {
			containerStat.SimplePredictionsCPU = &simpleCPUPrediction
		}

		if simpleMemoryPrediction, exists := workloadContainerKeyVsSimpleMemoryPrediction[workloadContainerKey]; exists {
			containerStat.SimplePredictionsMemory = &simpleMemoryPrediction
		}
	}

	workloadMetrics, exists := nsVsWorkloadMetrics[workloadInfo.Namespace][workloadKey]
	if !exists {
		logging.Errorf(ctx, "No workload metrics found for %s", workloadKey)
	} else {
		workloadStat.Replicas = int32(workloadMetrics.MedianReplicas)
	}

	if workloadInfo.Kind == "StatefulSet" || workloadStat.Replicas == 1 {
		workloadStat.EvictionRanking = types.EvictionRankingMedium
	} else {
		workloadStat.EvictionRanking = types.EvictionRankingHigh
	}

	logging.Infof(ctx, "Successfully created container-level stat for %s with %d containers", workloadKey, len(workloadStat.ContainerStats))

	return workloadStat
}

func (c *CreateStatsTask) getAllContainerResourcesFromContainerSpecs(_ context.Context, containerSpecs []corev1.Container, initContainerSpecs []corev1.Container) []utils.OriginalContainerResources {
	containerResources := make([]utils.OriginalContainerResources, 0, len(containerSpecs)+len(initContainerSpecs))

	for _, container := range containerSpecs {
		containerRes := utils.OriginalContainerResources{
			Name: container.Name,
			Type: types.AppContainer,
		}

		c.setResourceRequestAndLimit(&container, &containerRes)
		containerResources = append(containerResources, containerRes)
	}

	for _, container := range initContainerSpecs {
		if utils.IsSidecarContainer(container) {
			containerRes := utils.OriginalContainerResources{
				Name: container.Name,
				Type: types.SidecarContainer,
			}
			c.setResourceRequestAndLimit(&container, &containerRes)
			containerResources = append(containerResources, containerRes)
		} else {
			containerRes := utils.OriginalContainerResources{
				Name: container.Name,
				Type: types.InitContainer,
			}
			c.setResourceRequestAndLimit(&container, &containerRes)
			containerResources = append(containerResources, containerRes)
		}
	}

	return containerResources
}

func (c *CreateStatsTask) setResourceRequestAndLimit(container *corev1.Container, containerRes *utils.OriginalContainerResources) {
	if cpuRequest := container.Resources.Requests[corev1.ResourceCPU]; !cpuRequest.IsZero() {
		containerRes.CPURequest = float64(cpuRequest.MilliValue()) / 1000.0
	}
	if cpuLimit := container.Resources.Limits[corev1.ResourceCPU]; !cpuLimit.IsZero() {
		containerRes.CPULimit = float64(cpuLimit.MilliValue()) / 1000.0
	}

	if memRequest := container.Resources.Requests[corev1.ResourceMemory]; !memRequest.IsZero() {
		containerRes.MemoryRequest = float64(memRequest.Value()) / utils.BytesPerMB
	}
	if memLimit := container.Resources.Limits[corev1.ResourceMemory]; !memLimit.IsZero() {
		containerRes.MemoryLimit = float64(memLimit.Value()) / utils.BytesPerMB
	}
}
