package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/truefoundry/cruisekube/pkg/contextutils"
	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/metrics"
	"github.com/truefoundry/cruisekube/pkg/types"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

func updatePodResources(
	ctx context.Context,
	kubeClient *kubernetes.Clientset,
	pod *corev1.Pod,
	containerName string,
	resourceType corev1.ResourceName,
	requestValue, limitValue float64,
	requestFormat, limitFormat string,
	resourceTypeName string,
) (bool, string) {
	containerPatch := corev1.Container{
		Name: containerName,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				resourceType: resource.MustParse(fmt.Sprintf(requestFormat, requestValue)),
			},
		},
	}
	if limitValue != 0.0 {
		containerPatch.Resources.Limits = corev1.ResourceList{
			resourceType: resource.MustParse(fmt.Sprintf(limitFormat, limitValue)),
		}
	}
	patch := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{containerPatch},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return false, fmt.Sprintf("failed to marshal %s patch: %v", resourceTypeName, err)
	}

	_, err = kubeClient.CoreV1().Pods(pod.Namespace).Patch(
		ctx,
		pod.Name,
		k8stypes.StrategicMergePatchType,
		patchBytes,
		metav1.PatchOptions{},
		"resize",
	)

	if err != nil {
		logging.Errorf(ctx, "Strategic merge patch failed for pod %s container %s %s update: %v", pod.Name, containerName, resourceTypeName, err)
		return false, fmt.Sprintf("failed to update container %s %s resources: %v", containerName, resourceTypeName, err)
	}
	return true, ""
}

func UpdatePodCPUResources(
	ctx context.Context,
	kubeClient *kubernetes.Clientset,
	pod *corev1.Pod,
	containerName string,
	recommendedCPURequest float64,
	recommendedCPULimit float64,
) (bool, string) {
	return updatePodResources(
		ctx, kubeClient, pod, containerName,
		corev1.ResourceCPU, recommendedCPURequest, recommendedCPULimit,
		"%.3f", "%.3f", "CPU",
	)
}

func UpdatePodMemoryResources(
	ctx context.Context,
	kubeClient *kubernetes.Clientset,
	pod *corev1.Pod,
	containerName string,
	recommendedMemoryRequest float64,
	recommendedMemoryLimit float64,
) (bool, string) {
	return updatePodResources(
		ctx, kubeClient, pod, containerName,
		corev1.ResourceMemory, recommendedMemoryRequest, recommendedMemoryLimit,
		"%.0fM", "%.0fM", "memory",
	)
}

func EvictPod(ctx context.Context, kubeClient *kubernetes.Clientset, pod *corev1.Pod) (bool, string) {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}

	err := kubeClient.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction)
	if err != nil {
		return false, fmt.Sprintf("failed to evict pod: %v", err)
	}

	logging.Infof(ctx, "Successfully evicted pod %s from node %s", pod.Name, pod.Spec.NodeName)
	clusterID, ok := contextutils.GetCluster(ctx)
	if !ok {
		logging.Warnf(ctx, "EvictPod: Failed to get cluster ID from context, pod: %s/%s", pod.Namespace, pod.Name)
	}
	metrics.ClusterEvictionCount.WithLabelValues(clusterID).Inc()
	return true, ""
}

func BuildPodToWorkloadMapping(ctx context.Context, kubeClient *kubernetes.Clientset, targetNamespace string) (map[PodKey]WorkloadInfo, []*corev1.Pod, error) {
	logging.Infof(ctx, "Building pod-to-workload mapping for namespace: %s", getNamespaceLogMessage(targetNamespace))

	allPods, err := getScheduledPodsAcrossNamespaces(ctx, kubeClient, targetNamespace)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load scheduled pods: %w", err)
	}
	logging.Infof(ctx, "Loaded %d scheduled pods (with spec.nodeName set)", len(allPods))

	workloadLabelSelectorList, err := ListAllWorkloadsWithSelectors(ctx, kubeClient, targetNamespace)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build workload cache: %w", err)
	}
	logging.Infof(ctx, "Built label selector list with %d workloads", len(workloadLabelSelectorList))

	podToWorkloadMap := createPodToWorkloadMapping(allPods, workloadLabelSelectorList)
	logging.Infof(ctx, "Created mapping for %d pods to their parent workloads", len(podToWorkloadMap))

	return podToWorkloadMap, allPods, nil
}

func getScheduledPodsAcrossNamespaces(ctx context.Context, kubeClient *kubernetes.Clientset, targetNamespace string) ([]*corev1.Pod, error) {
	var podList *corev1.PodList
	var err error

	if targetNamespace != "" {
		podList, err = kubeClient.CoreV1().Pods(targetNamespace).List(ctx, metav1.ListOptions{})
	} else {
		podList, err = kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var scheduledPods []*corev1.Pod
	for _, pod := range podList.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodRunning {
			scheduledPods = append(scheduledPods, &pod)
		}
	}

	return scheduledPods, nil
}

func createPodToWorkloadMapping(pods []*corev1.Pod, workloadCache []WorkloadLabelSelectorList) map[PodKey]WorkloadInfo {
	podToWorkloadMap := make(map[PodKey]WorkloadInfo)

	for _, pod := range pods {
		podLabels := labels.Set(pod.Labels)
		podKey := PodKey{
			Namespace: pod.Namespace,
			PodName:   pod.Name,
		}

		for _, workload := range workloadCache {
			if workload.Namespace == pod.Namespace && workload.Selector.Matches(podLabels) {
				podToWorkloadMap[podKey] = WorkloadInfo{
					Kind:      workload.Kind,
					Namespace: workload.Namespace,
					Name:      workload.Name,
				}
				break
			}
		}
	}

	return podToWorkloadMap
}

func getNamespaceLogMessage(namespace string) string {
	if namespace == "" {
		return "all namespaces"
	}
	return namespace
}

func MergeContainerRawResultsIntoCache(ctx context.Context, cache WorkloadKeyVsContainerMetrics, rawResults RawBatchResult, metricType string, psiAdjusted bool) {
	for rawKey, value := range rawResults {
		kind, namespace, workloadName, containerName, ok := ParseWorkloadContainerKey(rawKey)
		if !ok {
			logging.Errorf(ctx, "[CreateStats] Skipping malformed container raw key: %s", rawKey)
			continue
		}

		if kind == ReplicaSetKind {
			if deploymentName, isDeployment := ExtractWorkloadFromReplicaSet(workloadName); isDeployment {
				kind = DeploymentKind
				workloadName = deploymentName
			}
		}

		workloadKey := GetWorkloadKey(kind, namespace, workloadName)

		workloadContainers, exists := cache[workloadKey]
		if !exists {
			workloadContainers = make(ContainerNameVsContainerMetrics)
			cache[workloadKey] = workloadContainers
		}

		containerMetrics, exists := workloadContainers[containerName]
		if !exists {
			containerMetrics = &ContainerMetrics{}
			workloadContainers[containerName] = containerMetrics
		}

		updateContainerMetrics(containerMetrics, value, metricType, psiAdjusted)
	}

	logging.Infof(ctx, "[CreateStats] Container cache now contains %d workloads after merging %s", len(cache), metricType)
}

func updateContainerMetrics(metrics *ContainerMetrics, value float64, metricType string, psiAdjusted bool) {
	if psiAdjusted {
		updatePSIAdjustedMetrics(metrics, value, metricType)
	} else {
		updateStandardMetrics(metrics, value, metricType)
	}
}

func updateStandardMetrics(metrics *ContainerMetrics, value float64, metricType string) {
	switch {
	case updateCPUMetrics(metrics, value, metricType):
		metrics.HasCPUData = true
	case updateMemoryMetrics(metrics, value, metricType):
		metrics.HasMemoryData = true
	case updateCPU7DayMetrics(metrics, value, metricType):
		metrics.HasCPUData = true
	case updateMemory7DayMetrics(metrics, value, metricType):
		metrics.HasMemoryData = true
	case metricType == "median_replicas":
		metrics.MedianReplicas = value
	}
}

func updateCPUMetrics(metrics *ContainerMetrics, value float64, metricType string) bool {
	switch metricType {
	case "cpu_p50":
		updateMaxValue(&metrics.CPUP50, value)
	case "cpu_p75":
		updateMaxValue(&metrics.CPUP75, value)
	case "cpu_p90":
		updateMaxValue(&metrics.CPUP90, value)
	case "cpu_p95":
		updateMaxValue(&metrics.CPUP95, value)
	case "cpu_p99":
		updateMaxValue(&metrics.CPUP99, value)
	case "cpu_p999":
		updateMaxValue(&metrics.CPUP999, value)
	case "cpu_max":
		updateMaxValue(&metrics.CPUMax, value)
	case "startup_cpu_max":
		updateMaxValue(&metrics.StartupCPUMax, value)
	case "non_startup_cpu_max":
		updateMaxValue(&metrics.NonStartupCPUMax, value)
	default:
		return false
	}
	return true
}

func updateMemoryMetrics(metrics *ContainerMetrics, value float64, metricType string) bool {
	switch metricType {
	case "memory_p50":
		updateMaxValue(&metrics.MemoryP50, value)
	case "memory_p75":
		updateMaxValue(&metrics.MemoryP75, value)
	case "memory_p90":
		updateMaxValue(&metrics.MemoryP90, value)
	case "memory_p95":
		updateMaxValue(&metrics.MemoryP95, value)
	case "memory_p99":
		updateMaxValue(&metrics.MemoryP99, value)
	case "memory_p999":
		updateMaxValue(&metrics.MemoryP999, value)
	case "memory_max":
		updateMaxValue(&metrics.MemoryMax, value)
	case "oom_memory":
		updateMaxValue(&metrics.OOMMemory, value)
	default:
		return false
	}
	return true
}

func updateCPU7DayMetrics(metrics *ContainerMetrics, value float64, metricType string) bool {
	switch metricType {
	case "cpu_p50_cpu_7day":
		updateMaxValue(&metrics.CPU7Day.P50, value)
	case "cpu_p75_cpu_7day":
		updateMaxValue(&metrics.CPU7Day.P75, value)
	case "cpu_p90_cpu_7day":
		updateMaxValue(&metrics.CPU7Day.P90, value)
	case "cpu_p99_cpu_7day":
		updateMaxValue(&metrics.CPU7Day.P99, value)
	case "cpu_max_cpu_7day":
		updateMaxValue(&metrics.CPU7Day.Max, value)
	default:
		return false
	}
	return true
}

func updateMemory7DayMetrics(metrics *ContainerMetrics, value float64, metricType string) bool {
	switch metricType {
	case "memory_max_7day":
		updateMaxValue(&metrics.Memory7Day.Max, value)
	default:
		return false
	}
	return true
}

func updatePSIAdjustedMetrics(metrics *ContainerMetrics, value float64, metricType string) {
	if metrics.PSIAdjustedUsage == nil {
		metrics.PSIAdjustedUsage = &PSIAdjustedUsage{}
	}

	switch metricType {
	case "cpu_p50":
		updateMaxValue(&metrics.PSIAdjustedUsage.CPUP50, value)
	case "cpu_p75":
		updateMaxValue(&metrics.PSIAdjustedUsage.CPUP75, value)
	case "cpu_p90":
		updateMaxValue(&metrics.PSIAdjustedUsage.CPUP90, value)
	case "cpu_p95":
		updateMaxValue(&metrics.PSIAdjustedUsage.CPUP95, value)
	case "cpu_p99":
		updateMaxValue(&metrics.PSIAdjustedUsage.CPUP99, value)
	case "cpu_p999":
		updateMaxValue(&metrics.PSIAdjustedUsage.CPUP999, value)
	case "cpu_max":
		updateMaxValue(&metrics.PSIAdjustedUsage.CPUMax, value)
	}
}

func updateMaxValue(current *float64, newValue float64) {
	if newValue > *current {
		*current = newValue
	}
}

func BuildContainerStatFromCache(ctx context.Context, workloadInfo WorkloadInfo, workloadKeyVsContainerMetrics WorkloadKeyVsContainerMetrics, containerResources []OriginalContainerResources) *WorkloadStat {
	workloadKey := GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name)
	containerMetrics, exists := workloadKeyVsContainerMetrics[workloadKey]
	if !exists {
		logging.Errorf(ctx, "[CreateStats] No container metrics found for workload %s", GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name))
		return nil
	}

	containerStats := []ContainerStats{}
	medianReplicas := 0.0

	for _, containerRes := range containerResources {
		// These are short-duration containers and we will not have enough data to make an optimisation related decision.
		// Can be reviewed later
		if containerRes.Type == types.InitContainer {
			continue
		}

		containerName := containerRes.Name
		metrics, exists := containerMetrics[containerName]
		if !exists {
			logging.Warnf(ctx, "[CreateStats] No container metrics found for workload %s, container %s", GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name), containerName)
			continue
		}

		medianReplicas = metrics.MedianReplicas
		if !metrics.HasCPUData || !metrics.HasMemoryData {
			logging.Errorf(ctx, "[CreateStats] Error: Incomplete metrics for container %s in workload %s (CPU: %v, Memory: %v)",
				containerName, GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name),
				metrics.HasCPUData, metrics.HasMemoryData)
			continue
		}

		cpuStats := &CPUStats{
			Max: metrics.CPUMax,
			P50: metrics.CPUP50,
			P75: metrics.CPUP75,
		}

		memoryStats := &MemoryStats{
			Max:       metrics.MemoryMax,
			P75:       metrics.MemoryP75,
			OOMMemory: metrics.OOMMemory,
		}

		memory7Day := &Memory7DayStats{
			Max: metrics.Memory7Day.Max,
		}

		cpu7Day := &CPU7DayStats{
			Max: metrics.CPU7Day.Max,
			P50: metrics.CPU7Day.P50,
			P75: metrics.CPU7Day.P75,
			P90: metrics.CPU7Day.P90,
			P99: metrics.CPU7Day.P99,
		}

		var psiAdjustedUsageStats *PSIAdjustedUsageStats
		if metrics.PSIAdjustedUsage != nil {
			psiAdjustedUsageStats = &PSIAdjustedUsageStats{
				Max: metrics.PSIAdjustedUsage.CPUMax,
				P75: metrics.PSIAdjustedUsage.CPUP75,
				P50: metrics.PSIAdjustedUsage.CPUP50,
			}
		}

		containerStats = append(containerStats, ContainerStats{
			ContainerName:    containerName,
			ContainerType:    containerRes.Type,
			CPUStats:         cpuStats,
			MemoryStats:      memoryStats,
			Memory7Day:       memory7Day,
			CPU7Day:          cpu7Day,
			PSIAdjustedUsage: psiAdjustedUsageStats,
		})
	}

	if len(containerStats) == 0 {
		logging.Errorf(ctx, "[CreateStats] No valid container recommendations for workload %s", GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name))
		return nil
	}

	workloadStat := &WorkloadStat{
		WorkloadIdentifier:         workloadKey,
		Kind:                       workloadInfo.Kind,
		Namespace:                  workloadInfo.Namespace,
		Name:                       workloadInfo.Name,
		ContinuousOptimization:     true,
		Replicas:                   int32(medianReplicas),
		ContainerStats:             containerStats,
		OriginalContainerResources: containerResources,
	}

	totalMaxCPU := workloadStat.CalculateTotalCPUStats(100)
	totalP50CPU := workloadStat.CalculateTotalCPUStats(50)
	if totalMaxCPU/totalP50CPU < ContinuousOptimizationRatioThreshold || totalMaxCPU-totalP50CPU < ContinuousOptimizationDiffThreshold {
		workloadStat.ContinuousOptimization = false
	}

	return workloadStat
}

func ParseThrottlingResults(ctx context.Context, throttlingResults RawBatchResult) []ThrottledWorkload {
	var throttledWorkloads []ThrottledWorkload

	for rawKey, throttlingRatio := range throttlingResults {
		kind, namespace, workloadName, containerName, ok := ParseWorkloadContainerKey(rawKey)
		if !ok {
			logging.Errorf(ctx, "[CreateStats] Failed to parse throttling result key: %s", rawKey)
			continue
		}

		if kind == ReplicaSetKind {
			if deploymentName, isDeployment := ExtractWorkloadFromReplicaSet(workloadName); isDeployment {
				kind = DeploymentKind
				workloadName = deploymentName
			}
		}

		workloadInfo := WorkloadInfo{
			Kind:      kind,
			Namespace: namespace,
			Name:      workloadName,
		}

		throttledWorkload := ThrottledWorkload{
			WorkloadInfo:    workloadInfo,
			ThrottlingRatio: throttlingRatio,
			ContainerName:   containerName,
		}

		throttledWorkloads = append(throttledWorkloads, throttledWorkload)
	}

	logging.Infof(ctx, "[CreateStats] Detected %d throttled workload containers", len(throttledWorkloads))
	return throttledWorkloads
}

func GetUniqueThrottledWorkloads(ctx context.Context, throttledWorkloads []ThrottledWorkload) []WorkloadInfo {
	uniqueWorkloads := make(map[string]WorkloadInfo)

	for _, throttled := range throttledWorkloads {
		workloadKey := GetWorkloadKey(throttled.WorkloadInfo.Kind, throttled.WorkloadInfo.Namespace, throttled.WorkloadInfo.Name)
		uniqueWorkloads[workloadKey] = throttled.WorkloadInfo
	}

	var result []WorkloadInfo
	for _, workloadInfo := range uniqueWorkloads {
		result = append(result, workloadInfo)
	}

	logging.Infof(ctx, "Found %d unique throttled workloads", len(result))
	return result
}

func GetWorkloadKey(kind, namespace, name string) string {
	return fmt.Sprintf("%s:%s:%s", kind, namespace, name)
}

// ParseWorkloadKey parses workload key in format kind:namespace:name
func ParseWorkloadKey(rawKey string) (string, string, string, bool) {
	parts := strings.Split(rawKey, ":")
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func GetWorkloadContainerKey(kind, namespace, name, containerName string) string {
	return fmt.Sprintf("%s:%s:%s:%s", kind, namespace, name, containerName)
}

// ParseWorkloadContainerKey parses workload container key in format kind:namespace:workloadName:containerName
func ParseWorkloadContainerKey(rawKey string) (string, string, string, string, bool) {
	parts := strings.Split(rawKey, ":")
	if len(parts) != 4 {
		return "", "", "", "", false
	}
	return parts[0], parts[1], parts[2], parts[3], true
}

func ExtractWorkloadFromReplicaSet(replicaSetName string) (string, bool) {
	re := regexp.MustCompile(`^(.+)-[a-f0-9]{6,12}$`)
	if matches := re.FindStringSubmatch(replicaSetName); len(matches) == 2 {
		return matches[1], true
	}
	return replicaSetName, false
}

func GetPodKey(namespace, name string) string {
	return fmt.Sprintf("%s:%s", namespace, name)
}

func CheckIfClusterAbove133(ctx context.Context, kubeClient *kubernetes.Clientset) bool {
	version, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		logging.Errorf(ctx, "[ApplyRecommendation] Error getting cluster version: %v", err)
		return false
	}

	gitVersion := strings.TrimPrefix(version.GitVersion, "v")

	versionParts := strings.Split(gitVersion, ".")
	if len(versionParts) < 2 {
		logging.Errorf(ctx, "[ApplyRecommendation] Invalid version format: %s", version.GitVersion)
		return false
	}

	major, err := strconv.Atoi(versionParts[0])
	if err != nil {
		logging.Errorf(ctx, "[ApplyRecommendation] Error parsing major version: %v", err)
		return false
	}

	minor, err := strconv.Atoi(versionParts[1])
	if err != nil {
		logging.Errorf(ctx, "[ApplyRecommendation] Error parsing minor version: %v", err)
		return false
	}

	if major > 1 || (major == 1 && minor >= 33) {
		logging.Infof(ctx, "[ApplyRecommendation] Cluster version %s is above 1.33", version.GitVersion)
		return true
	}

	logging.Infof(ctx, "[ApplyRecommendation] Cluster version %s is not above 1.33", version.GitVersion)
	return false
}

func GetWorkloadInfoFromPod(pod *corev1.Pod) *WorkloadInfo {
	if len(pod.OwnerReferences) == 0 {
		return nil
	}

	var workloadRef *metav1.OwnerReference
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == StatefulSetKind || ownerRef.Kind == DaemonSetKind {
			workloadRef = &ownerRef
			break
		}
		if ownerRef.Kind == ReplicaSetKind {
			re := regexp.MustCompile(`-[a-z0-9]+$`)
			deploymentName := re.ReplaceAllString(ownerRef.Name, "")
			workloadRef = &metav1.OwnerReference{
				Kind: DeploymentKind,
				Name: deploymentName,
			}
		}
	}

	if workloadRef == nil {
		return nil
	}

	return &WorkloadInfo{
		Kind:      workloadRef.Kind,
		Namespace: pod.Namespace,
		Name:      workloadRef.Name,
	}
}

func PtrTo[T any](v T) *T { return &v }

// IsSidecarContainer checks if an InitContainer is a sidecar container
func IsSidecarContainer(initContainer corev1.Container) bool {
	return initContainer.RestartPolicy != nil && *initContainer.RestartPolicy == corev1.ContainerRestartPolicyAlways
}

func GetPods(ctx context.Context, kubeClient *kubernetes.Clientset, namespace string, selector labels.Selector) (*corev1.PodList, error) {
	pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return pods, nil
}
