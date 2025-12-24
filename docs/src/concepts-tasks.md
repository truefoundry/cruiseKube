# Concepts

## Tasks

CruiseKube's control loop is implemented as a set of **scheduled tasks**. Each task performs a well-scoped responsibility and communicates through shared storage, metrics, and Kubernetes APIs. Together, they form a convergent optimization system rather than a reactive autoscaler.

## 1. Create Stats Task

The **Create Stats** task generates **persistent, workload-level statistical models** from raw Kubernetes state and Prometheus metrics.

For every workload (Deployment, StatefulSet, DaemonSet), it computes:

* Container-level CPU and memory usage percentiles
* OOM-aware memory statistics
* Replica counts and workload age
* Init container and sidecar resource behavior
* Constraint signals such as PDBs, HPAs, and eviction safety
* Derived prediction features used later by recommendation strategies

> These stats become the **single source of truth** for optimization decisions.
> This task **never mutates workloads**. It is purely analytical and safe to run frequently.

**Below is how it works:**

1. **Discovers workloads**: Lists all workloads (optionally namespace-scoped) and filters out workloads that already have recent stats to avoid redundant computation.

2. **Detects cluster capabilities**: Checks whether PSI (Pressure Stall Information) metrics are available and adapts CPU modeling accordingly.

3. **Batch-queries Prometheus**
   Executes namespace-scoped batch queries to efficiently fetch container-level and workload-level metrics.

4. **Builds statistical models**

     * Aggregates time-series data into percentiles (P50, P75, P90, P95, P99)
     * Computes OOM-aware memory statistics
     * Generates simple prediction models for CPU and memory
     * Associates predictions with individual containers

5. **Detects constraints**: Identifies workload-level constraints including:

     * CPU-based Horizontal Pod Autoscalers (HPAs)
     * PodDisruptionBudgets (PDBs)
     * Eviction safety and workload characteristics

6. **Persists stats**: Writes the generated stats to cluster storage with timestamps, making them available to recommendation and application tasks.

**Configuration options**

```yaml
env:
  CRUISEKUBE_CONTROLLER_TASKS_CREATESTATS_ENABLED: false
  CRUISEKUBE_CONTROLLER_TASKS_CREATESTATS_SCHEDULE: "15m"
  CRUISEKUBE_CONTROLLER_TASKS_CREATESTATS_METADATA_SKIPMEMORY: "false"
```

* **`..._ENABLED`**
  Enables or disables stats generation.

* **`..._SCHEDULE`**
  Controls how often workload stats are recomputed.

* **`..._METADATA_SKIPMEMORY`**
  Skips memory modeling entirely.
  Useful when memory optimization is intentionally disabled or considered too risky.

</br>

---

</br>

## 2. Fetch Metrics Task

The **Fetch Metrics** task continuously collects **cluster-level and node-level health metrics** and exposes them as Prometheus metrics.

It focuses on **observability and safety**, not optimization logic.

Collected signals include:

* Cluster CPU and memory utilization
* Requested vs allocatable resources
* Node and container PSI (CPU & memory waiting)
* Node load averages
* OOM kill events
* Unschedulable pod counts
* Karpenter consolidation evictions
* Freshness of workload stats

> This task is **read-only** and has no impact on workloads or recommendations.
> It exists to provide visibility and guardrails.

**Below is how it works:**

1. **Executes predefined PromQL queries**: Runs carefully designed Prometheus queries that aggregate metrics across nodes, pods, and containers while excluding GPU nodes.

2. **Computes cluster summaries**: Extracts scalar values such as total CPU usage, memory usage, allocatable capacity, and request pressure.

3. **Computes pressure signals**: Calculates max and P50 values for CPU and memory waiting using PSI metrics.

4. **Tracks reliability signals**:

     * Counts OOM kill events
     * Detects unschedulable pods
     * Monitors Karpenter-driven evictions

5. **Exports metrics**: Publishes all values as Prometheus metrics for dashboards, alerts, and analysis.

6. **Tracks stats freshness**: Computes max, P90, and P50 age of stored workload stats to detect stale optimization data.

<!-- **Configuration options**

```yaml
env:
  CRUISEKUBE_CONTROLLER_TASKS_FETCHMETRICS_ENABLED: false
  CRUISEKUBE_CONTROLLER_TASKS_FETCHMETRICS_SCHEDULE: "1m"
```

* **`CRUISEKUBE_CONTROLLER_TASKS_FETCHMETRICS_ENABLED`**
  Enables or disables metrics collection.

* **`CRUISEKUBE_CONTROLLER_TASKS_FETCHMETRICS_SCHEDULE`**
  Controls how frequently Prometheus is queried. -->

</br>

---

</br>

## 3. Apply Recommendation Task

The **Apply Recommendation** task is the **core execution engine** of CruiseKube.

It consumes workload stats and cluster state to:

* Generate container-level CPU and memory recommendations
* Distribute spare capacity across pods
* Respect workload overrides and safety constraints
* Evict pods when required to enforce resource feasibility
* Apply resource updates to running workloads

> This is the **only task that mutates workloads**.
> It is conservative by design and intended to converge gradually.

**Below is how it works:**

1. **Performs safety and authorization checks**:

     * Skips execution if cluster writes are not authorized
     * Forces dry-run mode on unsupported Kubernetes versions

2. **Builds node-centric views**:

     * Maps pods to nodes
     * Associates each pod with workload stats
     * Computes allocatable vs requested CPU and memory per node

3. **Filters optimizable pods**: Pods are excluded if they are:

      * Guaranteed QoS
      * Too new
      * CPU-autoscaled via HPA
      * Disabled via workload overrides
      * Located in blacklisted namespaces

4. **Runs optimization strategy**:

     * Uses P75 usage as baseline
     * Accounts for peak (Pmax) behavior
     * Distributes spare capacity proportionally
     * Selects eviction candidates using eviction rankings

5. **Applies changes**:

     * Updates CPU requests with safety clamps
     * Applies OOM-aware memory requests and limits
     * Skips unsafe transitions
     * Evicts pods when necessary

6. **Emits outcome metrics**:
   Tracks applied recommendations, evictions, and cluster-level resource spikes.

**Configuration options**

```yaml
env:
  CRUISEKUBE_CONTROLLER_TASKS_APPLYRECOMMENDATION_ENABLED: false
  CRUISEKUBE_CONTROLLER_TASKS_APPLYRECOMMENDATION_SCHEDULE: "5m"
  CRUISEKUBE_CONTROLLER_TASKS_APPLYRECOMMENDATION_METADATA_DRYRUN: "false"
  CRUISEKUBE_CONTROLLER_TASKS_APPLYRECOMMENDATION_METADATA_SKIPMEMORY: "false"
  CRUISEKUBE_CONTROLLER_TASKS_APPLYRECOMMENDATION_METADATA_NODESTATSURL_HOST: "http://localhost:8080"
  CRUISEKUBE_CONTROLLER_TASKS_APPLYRECOMMENDATION_METADATA_OVERRIDESURL_HOST: "http://localhost:8080"
```

* **`..._ENABLED`**
  Enables or disables recommendation application.

* **`..._SCHEDULE`**
  Controls how often recommendations are evaluated and applied.

* **`..._METADATA_DRYRUN`**
  Computes recommendations without applying changes.
  Strongly recommended during rollout.

* **`..._METADATA_SKIPMEMORY`**
  Applies only CPU changes and skips memory updates.

* **`..._METADATA_NODESTATSURL_HOST`**
  Endpoint used to fetch precomputed cluster stats.

* **`..._METADATA_OVERRIDESURL_HOST`**
  Endpoint used to fetch workload overrides and eviction policies.

</br>

---

</br>

## 4. Modify Equal CPU Resources Task

The **Modify Equal CPU Resources** task fixes a subtle but harmful configuration pattern where containers have **equal CPU requests and limits**.

Such containers cannot burst and are prone to throttling.

**Below is how it works:**

1. **Scans workloads**
   Lists all workloads that have running or pending pods.

2. **Inspects container specs**
   Detects containers where CPU request equals CPU limit.

3. **Applies minimal correction**
   Increases the CPU limit by a single milli-CPU unit while keeping the request unchanged.

4. **Patches workload templates**
   Applies JSON patches so future pods inherit the corrected limits.

> This task is intentionally minimal and safe.
> It does **not** change requests or rebalance workloads.

<!-- **Configuration options**

```yaml
env:
  CRUISEKUBE_CONTROLLER_TASKS_MODIFYEQUALCPURESOURCES_ENABLED: false
  CRUISEKUBE_CONTROLLER_TASKS_MODIFYEQUALCPURESOURCES_SCHEDULE: "10m"
```

* **`…_ENABLED`**
  Enables or disables CPU symmetry correction.

* **`…_SCHEDULE`**
  Controls how often workloads are scanned. -->
  
</br>

---

</br>

## 5. Node Load Monitoring Task

The **Node Load Monitoring** task protects cluster stability by **tainting overloaded nodes** to prevent further scheduling.

It acts as a **safety valve** when optimization or workload behavior pushes nodes too hard.

**Below is how it works:**

1. **Lists all nodes**:
   Fetches the complete node inventory from the cluster.

2. **Queries node load metrics**:
   Computes node load as `node_load1 / cpu_capacity` over a rolling lookback window.

3. **Detects overload conditions**:
   Marks nodes as overloaded when load exceeds the configured threshold.

4. **Manages taints**:

   * Adds a `NoSchedule` taint to overloaded nodes
   * Removes the taint once load returns to normal

> This task does **not evict pods**.
> It only prevents new scheduling to protect cluster health.

<!-- **Configuration options**

```yaml
env:
  CRUISEKUBE_CONTROLLER_TASKS_NODELOADMONITORING_ENABLED: false
  CRUISEKUBE_CONTROLLER_TASKS_NODELOADMONITORING_SCHEDULE: "60s"
```

* **`…_ENABLED`**
  Enables or disables node load monitoring.

* **`…_SCHEDULE`**
  Controls how frequently node load is evaluated. -->
