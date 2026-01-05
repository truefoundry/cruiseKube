## Introduction

Resource optimization in Kubernetes can be broken down into two fundamental problems:

1. **Pod-level resource requests**: Correctly specifying resource requests for each pod to match actual usage patterns
2. **Bin-packing**: Efficiently scheduling multiple pods across nodes to maximize utilization

While node auto-provisioners like Karpenter and Cluster Autoscaler address the second problem, they often produce suboptimal results due to reliability constraints like topology spread constraints, affinity rules, and pod disruption budgets. CruiseKube focuses exclusively on the first problem—optimizing pod-level resource requests—since this is where most resource wastage occurs (industry-wide CPU and memory utilization of provisioned resources is typically very low).

## Problem Context

### Current State Challenges

The current approach to resource management in Kubernetes faces several fundamental limitations:

- **Reliability over cost**: Engineers tend to over-provision resources to avoid reliability issues, prioritizing stability over efficiency
- **Manual, workload-level configuration**: Resource requests must be set manually for each workload, and changes require updating workload definitions (Deployments, StatefulSets, DaemonSets). This is particularly problematic for DaemonSets where pods on different nodes may have vastly different resource needs—a pod on a large node might need 10x more resources than one on a smaller node, yet all pods share the same resource specification
- **Peak provisioning**: CPU usage can spike up to 1000x baseline for workloads with sparse requests or job-like characteristics. Provisioning for these peaks leads to massive cumulative over-provisioning across the cluster

### What Changed

Two Kubernetes features enable a fundamentally different approach:

1. **Kubernetes 1.33 - Pod-level resource updates:**
    - Resources can now be updated at the pod level without recreating pods
    - Each pod can be optimized independently based on its actual usage patterns
    - Shorter prediction horizons become viable since decisions can be updated near real-time as new data arrives
    - Pods can be optimized considering the current node's conditions, allowing resource sharing between pods on the same node without sacrificing reliability

2. **Kubernetes 1.34 - PSI (Pressure Stall Indicator) metrics:**
    - PSI metrics expose CPU and memory pressure at both node and container levels
    - CPU limits can be removed, using PSI metrics directly for feedback on CPU contention
    - PSI-adjusted CPU usage provides accurate measurements of actual CPU demand under contention

### Design Philosophy

CruiseKube treats CPU and memory optimization differently:

- **CPU optimization**: Most over-provisioning comes from CPU over-provisioning. Better CPU resource management leads to overall better resource utilization when combined with node auto-provisioners. CPU usage is extremely spiky, but CPU throttling provides a safety mechanism—workloads slow down rather than fail when limits are hit.

- **Memory optimization**: Memory usage tends to be more consistent over restarts. Once a container uses a certain amount of memory, it typically remains at that level. However, containers hitting memory limits are immediately killed via OOM (Out-of-Memory), making memory under-provisioning far more dangerous than CPU under-provisioning.

## Core Concepts

### PSI (Pressure Stall Indicator)

PSI measures the time a task spends waiting for a resource (CPU, memory, or I/O). For CPU, PSI tracks how long processes wait for CPU time due to contention.

**PSI-Adjusted CPU Usage Formula:**

```
PSI_adjusted_usage = cpu_usage × (1 + psi_waiting_rate)
```

Where:
- `cpu_usage`: Raw CPU usage from `container_cpu_usage_seconds_total`
- `psi_waiting_rate`: Rate of CPU waiting time from `container_pressure_cpu_waiting_seconds_total`

**Example:**
If a container uses 0.5 CPU cores but experiences 20% CPU waiting time due to contention:
```
PSI_adjusted_usage = 0.5 × (1 + 0.2) = 0.6 CPU cores
```

This adjusted value reflects the actual CPU demand when the container is competing for resources, providing a more accurate basis for resource allocation decisions.

### Steady State vs Peak Usage

CruiseKube distinguishes between two usage patterns:

- **Steady State (P75)**: 
    - **CPU**: The 75th percentile of CPU usage over the last 10 minutes. This represents typical, sustained usage and serves as the baseline demand. P75 is chosen because it captures normal operation while filtering out transient spikes.
    - **Memory**: The 75th percentile of memory usage over the last 30 minutes. The longer window accounts for memory's more gradual changes compared to CPU.

- **Peak Usage (Max)**: The maximum predicted usage calculated using a simple time-series approach: the maximum value observed in the past 60 minutes at the current hour over the last 1 week. This methodology captures recurring patterns (e.g., daily or weekly cycles) while accounting for recent spikes. The difference between peak and steady state represents the "spike demand" or "headroom" needed to accommodate usage bursts.

> The current simple time-series prediction model will be replaced with a more sophisticated time-series forecasting model in future versions to improve prediction accuracy.

### Resource Calculation Methodology

CruiseKube uses specific time windows and calculation methods for CPU and memory:

CPU Calculation: 

- **Requests:**
    - **Base Demand (P75)**: Calculated over the last 10 minutes (`CPULookbackWindow`). Uses PSI-adjusted usage when available, otherwise raw CPU usage.
    - **Peak Demand (Max)**: Calculated using `SimplePredictions.MaxValue`, which takes the maximum of three predictions:
        - **Hourly Prediction**: Maximum value at the same hour over the last 7 days
        - **Current Prediction**: Maximum value in the last 60 minutes
- **Limits**: Not set. Only CPU requests are configured, relying on PSI metrics for contention feedback.

Memory Calculation: 

- **Requests:**
    - **Base Demand (P75)**: Calculated over the last 30 minutes (`MemoryLookbackWindow`)
    - **Peak Demand (Max)**: Uses the same `SimplePredictions.MaxValue` methodology as CPU.
- **Memory Limits**: Set to `2 × max(Memory7Day.Max, OOMMemory)`, where `Memory7Day.Max` is the maximum memory usage over the last 7 days and `OOMMemory` is the memory usage at the time of OOM kill events (if any). This provides a safety buffer to prevent OOM kills while allowing memory requests to be optimized independently.

### Spike Demand Distribution

Rather than provisioning each pod for its maximum spike, CruiseKube distributes spike headroom across pods on a node. This works because:

1. Not all pods spike simultaneously
2. The maximum spike demand across all pods represents the worst-case scenario
3. This headroom can be shared proportionally, reducing total resource requirements

### Eviction Priority

When a node cannot accommodate all pods, CruiseKube uses a priority-based eviction system:

1. **DaemonSets**: Always protected from eviction (they must run on every node)
2. **Eviction Ranking**: Configurable per-workload priority that determines eviction order:
    - **Low**: Low priority workloads are evicted first. This is the priority for most workloads by default
    - **Medium**: Medium priority workloads are evicted next. Single replica or statefulset workloads are set to this value by default
    - **High**: High priority workloads are evicted as a last resort
    - **No-Eviction**: No-eviction workloads are never evicted at all
3. **Tie-breaker**: Among pods with the same eviction ranking, those with higher spike demand (`MaxRestCPU`/`MaxRestMemory`) are evicted first, as they require more headroom

## Algorithm Overview

CruiseKube uses a two-phase optimization approach:

- **Phase 1 (Admission Webhook)**: Sets initial resource requests at scheduling time to ensure the pod can be scheduled and has sufficient resources for peak usage.

- **Phase 2 (Continuous Optimization)**: Periodically optimizes running pods, redistributing resources based on actual node conditions and sharing spike headroom across pods.

## Phase 1: Admission Webhook Optimization

The admission webhook intercepts pod creation requests and sets resource requests based on historical usage patterns.

### When It Runs

- Triggered during pod creation (before scheduling)
- Mutates the pod specification before it reaches the scheduler

### Algorithm

```c
// Fetch historical statistics for this workload/container
stats = fetchWorkloadStats(pod.workloadIdentifier, container.name)

// Calculate CPU base demand (P75 over last 10 minutes)
// Use PSI-adjusted if available, otherwise raw CPU stats
if stats.PSIAdjustedUsage != nil:
  cpuBaseDemand = stats.PSIAdjustedUsage.P75  // P75 over last 10 minutes
else:
  cpuBaseDemand = stats.CPUStats.P75  // P75 over last 10 minutes

// Calculate CPU spike demand (Max - Base)
// Max = max observed in past 60 minutes at current hour over last 1 week
cpuSpikeDemand = stats.SimplePredictionsCPU.MaxValue - cpuBaseDemand

// Calculate Memory base demand (P75 over last 30 minutes)
memoryBaseDemand = stats.MemoryStats.P75  // P75 over last 30 minutes

// Calculate Memory spike demand (Max - Base)
// Max = max observed in past 60 minutes at current hour over last 1 week
memorySpikeDemand = stats.SimplePredictionsMemory.MaxValue - memoryBaseDemand

// Set pod requests to accommodate peak usage
pod.spec.containers[container].resources.requests.cpu = cpuBaseDemand + cpuSpikeDemand
pod.spec.containers[container].resources.requests.memory = memoryBaseDemand + memorySpikeDemand

// Set memory limit to 2 × max memory over last 7 days (safety buffer)
memoryLimit = 2 × max(stats.Memory7Day.Max, stats.MemoryStats.OOMMemory)
pod.spec.containers[container].resources.limits.memory = memoryLimit

// Note: CPU limits are not set (rely on PSI metrics for feedback)
```

### Purpose

- **Ensures schedulability**: The pod requests sufficient resources to be scheduled on a node
- **Prevents repeated evictions**: By setting requests to peak usage, the pod is guaranteed to fit on the node where it's scheduled even after continuous optimization has run.
- **Provides initial optimization**: Even without continuous optimization, pods start with better resource allocation

## Phase 2: Continuous Optimization

The continuous optimization controller runs periodically, optimizing resources for pods already running on nodes.

### When It Runs

- Periodic reconciliation loop (configurable schedule, typically every 5 minutes)
- Processes one node at a time
- Updates pod resources in-place without pod recreation

### Algorithm Flow

```mermaid
flowchart TD
    A[Start: Process Node] --> B[Identify Optimizable Pods]
    B --> C[Calculate Base + Spike Demand per Pod]
    C --> D{Memory Fits?}
    D -->|No| E[Evict Low Priority Pods<br/>Memory]
    D -->|Yes| F{CPU Fits?}
    E --> F
    F -->|No| G[Evict Low Priority Pods<br/>CPU]
    F -->|Yes| H[Calculate Max Spike Demand]
    G --> H
    H --> I[Distribute Spike Headroom<br/>Proportionally]
    I --> J[Apply Resource Updates]
    J --> K[Next Node]
```

### Detailed Algorithm

```c
FOR EACH node IN cluster:

  // Step 1: Identify scope of optimization
  optimizablePods = selectPodsEligibleForOptimization(node.pods)
  nonOptimizablePods = node.pods - optimizablePods
  
  availableCPU = node.allocatableCPU - sum(nonOptimizablePods.cpuRequests)
  availableMemory = node.allocatableMemory - sum(nonOptimizablePods.memoryRequests)

  // Step 2: Compute baseline demand and spike demand per pod
  FOR EACH pod IN optimizablePods:
    FOR EACH container IN pod.containers:
      // CPU base demand: P75 over last 10 minutes
      // Use PSI-adjusted if available, otherwise raw CPU stats
      if container.stats.PSIAdjustedUsage != nil:
        container.baseDemand = container.stats.PSIAdjustedUsage.P75  // P75 over last 10 min
      else:
        container.baseDemand = container.stats.CPUStats.P75  // P75 over last 10 min
      
      // CPU spike demand: Max (past 60 min at current hour over last 1 week) - Base
      container.spikeDemand = container.stats.SimplePredictionsCPU.MaxValue - container.baseDemand
      
      // Memory base demand: P75 over last 30 minutes
      container.memoryBase = container.stats.MemoryStats.P75  // P75 over last 30 min
      
      // Memory spike demand: Max (past 60 min at current hour over last 1 week) - Base
      container.memorySpike = container.stats.SimplePredictionsMemory.MaxValue - container.memoryBase
      
    pod.evictionRanking = determineEvictionRanking(pod)
    pod.totalRecommendedCPU = sum(container.baseDemand) + max(container.spikeDemand)
    pod.totalRecommendedMemory = sum(container.memoryBase) + max(container.memorySpike)
    pod.maxRestCPU = max(container.spikeDemand)
    pod.maxRestMemory = max(container.memorySpike)

  // Step 3: Evict pods if memory doesn't fit
  sort pods by: DaemonSet protection > evictionRanking > maxRestMemory (descending)
  WHILE sum(pod.totalRecommendedMemory) > availableMemory OR 
        max(pod.maxRestMemory) > (availableMemory - sum(pod.totalRecommendedMemory)):
    podToEvict = pods[0]  // Lowest priority
    markForEviction(podToEvict)
    remove podToEvict from pods

  // Step 4: Evict pods if CPU doesn't fit
  sort pods by: DaemonSet protection > evictionRanking > maxRestCPU (descending)
  WHILE sum(pod.totalRecommendedCPU) > availableCPU OR 
        max(pod.maxRestCPU) > (availableCPU - sum(pod.totalRecommendedCPU)):
    podToEvict = pods[0]  // Lowest priority
    markForEviction(podToEvict)
    remove podToEvict from pods

  // Step 5: Distribute spike headroom across surviving pods
  maxSpikeCPU = max(pod.maxRestCPU for pod IN pods)
  maxSpikeMemory = max(pod.maxRestMemory for pod IN pods)
  totalSpikeCPU = sum(container.spikeDemand for all containers in pods)
  totalSpikeMemory = sum(container.memorySpike for all containers in pods)

  FOR EACH pod IN pods:
    FOR EACH container IN pod.containers:
      // Proportional distribution based on each container's spike demand
      if totalSpikeCPU > 0:
        cpuRatio = container.spikeDemand / totalSpikeCPU
        additionalCPU = maxSpikeCPU × cpuRatio
      else:
        additionalCPU = 0
      
      if totalSpikeMemory > 0:
        memoryRatio = container.memorySpike / totalSpikeMemory
        additionalMemory = maxSpikeMemory × memoryRatio
      else:
        additionalMemory = 0
      
      container.finalCPU = container.baseDemand + additionalCPU
      container.finalMemory = container.memoryBase + additionalMemory

  // Step 6: Apply recommendations at runtime
  FOR EACH pod IN pods:
    FOR EACH container IN pod.containers:
      // Apply CPU request (no CPU limit set)
      applyCPURequest(pod, container, container.finalCPU)
      
      // Apply memory request and limit
      // Limit = 2 × max(memory over last 7 days, OOM memory)
      memoryLimit = 2 × max(container.stats.Memory7Day.Max, container.stats.MemoryStats.OOMMemory)
      applyMemoryRequestAndLimit(pod, container, container.finalMemory, memoryLimit)

END
```

### Key Differences from Admission Webhook

- **Resource sharing**: Spike headroom is distributed proportionally rather than allocated fully to each pod
- **Node-aware**: Considers actual node capacity and current pod placement
- **Eviction support**: Can evict pods to make room for better optimization
- **Incremental updates**: Resources are updated in-place without pod recreation

## Key Mechanisms

### PSI Adjustment Calculation

PSI adjustment accounts for CPU contention by measuring waiting time:

```promql
cpu_usage = rate(container_cpu_usage_seconds_total{...}[5m])

psi_waiting_rate = rate(container_pressure_cpu_waiting_seconds_total{...}[5m])

psi_adjusted_cpu = cpu_usage × (1 + psi_waiting_rate)
```

When PSI metrics are available, CruiseKube uses `PSIAdjustedUsage.P75` (calculated over the last 10 minutes) instead of raw `CPUStats.P75` for base demand calculations. This ensures resource requests account for contention, preventing under-provisioning during high-load periods.

### Spike Demand Distribution Formula

The spike distribution algorithm ensures that the maximum spike demand across all pods is accommodated, while distributing this headroom proportionally:

```
For each container:
  additionalCPU = maxSpikeCPU × (container.spikeDemand / totalSpikeDemand)
  finalCPU = baseDemand + additionalCPU
```

**Example:**

- Node has 3 pods with spike demands: Pod A = 2.0, Pod B = 1.0, Pod C = 1.0
- Max spike demand = 2.0
- Total spike demand = 4.0
- Pod A gets: 2.0 × (2.0/4.0) = 1.0 additional CPU
- Pod B gets: 2.0 × (1.0/4.0) = 0.5 additional CPU
- Pod C gets: 2.0 × (1.0/4.0) = 0.5 additional CPU

This ensures that if Pod A spikes, it has sufficient resources (base + 1.0), while other pods share the remaining headroom.

### Eviction Strategy

Eviction follows a strict priority order:

1. **DaemonSets**: Never evicted (must run on every node)
2. **Eviction Ranking**: 
    - Lower ranking numbers = higher priority (No-Eviction=1, Low=2, Medium=3, High=4)
    - Default: Medium priority for single-replica workloads and StatefulSets, Low priority for all other workloads
3. **Tie-breaker**: Among pods with the same ranking, evict those with higher `maxRestCPU`/`maxRestMemory` first

The eviction loop checks both:

- Total recommended resources fit within node capacity
- Maximum spike demand can be accommodated in remaining capacity

### Memory Limit Calculation

Memory limits are set independently from memory requests to provide a safety buffer:

- **Memory Limit Formula**: `2 × max(Memory7Day.Max, OOMMemory)`
  - `Memory7Day.Max`: Maximum memory usage observed over the last 7 days
  - `OOMMemory`: Memory usage at the time of OOM kill events (if any)
- **Purpose**: Prevents OOM kills by providing headroom above the optimized request value
- **Rationale**: Memory requests can be optimized aggressively (down to P75 + distributed spike), while limits ensure containers have sufficient headroom to handle unexpected memory growth

**Note**: CPU limits are not set. CruiseKube relies on CPU requests and PSI metrics for resource management, eliminating unnecessary CPU throttling while maintaining reliability.

### Node Load Monitoring

To mitigate scenarios where multiple pods spike simultaneously, CruiseKube monitors node load:

- **Load Calculation**: `node_load1 / node_cpu_capacity`
- **Threshold**: When load exceeds 100% (1.0), the node is considered overloaded
- **Action**: Node is cordoned (tainted with `NoSchedule`) to prevent new pod scheduling
- **Recovery**: Taint is removed when load drops below threshold

During overload:
- PSI metrics increase, causing future resource requests to be higher
- No new pods are scheduled on the overloaded node
- Existing pods continue running with their current resource allocations

## Safety & Reliability Guarantees

### Preventing Repeated Evictions

The admission webhook ensures pods are scheduled with requests equal to their peak usage. This guarantees:
- The node has sufficient capacity at scheduling time
- Subsequent continuous optimization runs only evict pods whose spikes cannot be accommodated
- Once evicted, a pod will be rescheduled with peak requests, preventing immediate re-eviction

### High-Priority Workload Protection

- Single-replica workloads and StatefulSets default to medium eviction ranking
- Most workloads default to low eviction ranking
- Users can configure eviction ranking per workload via overrides
- DaemonSets are never evicted
- Workloads with no-eviction ranking are never evicted

### Contention Mitigation

- **Node cordoning**: Overloaded nodes are isolated to prevent further scheduling
- **PSI feedback**: High PSI values increase resource requests for future optimizations
- **Proportional distribution**: Spike headroom is shared, reducing the likelihood of simultaneous spikes exhausting resources

### Why CPU Limits Aren't Needed

CruiseKube does not set CPU limits, only CPU requests. This differs from memory, where both requests and limits are configured.

**Traditional CPU limits serve two purposes:**

1. **Resource isolation**: Prevent one pod from consuming all CPU
2. **Throttling feedback**: Indicate when a pod needs more CPU

**With PSI metrics:**

- **PSI provides throttling feedback**: High PSI values indicate CPU contention without needing limits
- **Requests provide isolation**: CPU requests ensure fair scheduling and resource allocation
- **Better observability**: PSI metrics provide more granular insight into contention than throttling metrics

**Contrast with Memory:**

Unlike CPU, memory limits are set to `2 × max(Memory7Day.Max, OOMMemory)`. This is necessary because:

- Memory OOM kills are immediate and catastrophic (unlike CPU throttling)
- Memory limits provide a safety buffer above optimized requests
- Memory requests can be optimized aggressively while limits ensure headroom

By removing CPU limits and using PSI-adjusted usage for requests, CruiseKube:

- Eliminates unnecessary CPU throttling
- Provides more accurate CPU resource allocation
- Maintains reliability through PSI-based feedback
- Allows CPU to burst when available, improving overall utilization
