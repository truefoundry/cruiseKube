# Introduction

## What is CruiseKube?

**CruiseKube** is a Kubernetes-native, continuous resource optimization system that autonomously right-sizes CPU and memory for workloads at **runtime** and **admission time**. It focuses on eliminating persistent over-provisioning while preserving workload reliability and scheduling constraints.

Unlike static requests, manual tuning, or reactive autoscaling, CruiseKube operates as a **closed-loop control system** that observes real workload behavior and incrementally converges resource requests toward optimal values.


## When do you need CruiseKube?

You would need CruiseKube if you are facing any of these issues -

- **Chronic over-provisioning** driven by guesswork, peak-based sizing, and fear of CPU throttling or OOM crashes
- **Cost inefficiency** that node-level bin packing as provided by autoscalers (Cluster Autoscaler/Karpenter) alone cannot fix
- **Operational Load** arising from manual tuning of workloads on kubernetes by developers or DevOps teams

CruiseKube explicitly addresses the **pod-level right-sizing problem**, in a fully hands-off manner.

## Tasks

CruiseKube operates as a closed-loop system through a set of **periodic background tasks**.
Each task has a clearly defined responsibility and can be enabled or disabled independently.

1. <u>**Create Stats Task:**</u>
Builds persistent, workload-level CPU and memory statistics from Kubernetes state and Prometheus metrics.
These stats form the foundation for all optimization decisions and are stored for reuse.

2. <u>**Fetch Metrics Task:**</u>
Collects cluster-wide and node-level health signals such as utilization, pressure, OOMs, and scheduling issues.
Primarily used for observability, dashboards, and safety guardrails.

3. <u>**Apply Recommendation Task:**</u>
Generates and applies CPU and memory recommendations to workloads in a controlled, incremental manner.
This is the core task responsible for actually right-sizing workloads.

4. <u>**Modify Equal CPU Resources Task:**</u>
Fixes containers where CPU request and limit are equal, allowing them to burst and reducing throttling risk.
Applies minimal, safe corrections at the workload spec level.

5. <u>**Node Load Monitoring Task:**</u>
Monitors node load and temporarily taints overloaded nodes to prevent further scheduling.
Acts as a safety mechanism to protect cluster stability during optimization.

Together, these tasks allow CruiseKube to continuously optimize resources **without relying on manual tuning or reactive scaling**.

