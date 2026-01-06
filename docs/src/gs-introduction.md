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

## Architecture

```mermaid
flowchart LR
    %% Actor
    Human((Human))

    %% Kubernetes Cluster Boundary
    subgraph K8s[Kubernetes Cluster]
        direction LR

        %% Frontend
        Frontend[Frontend]

        %% Controller
        subgraph Controller
            direction TB
            Stats[Statistics Engine]
            Runtime[Runtime Optimizer]
        end

        %% API Server
        APIServer[kube-api-server]

        %% Webhook
        subgraph Webhook
            Admission[Admission Optimizer]
        end

        %% Data & Metrics
        Database[(Database)]
        Prometheus[Prometheus]
    end

    %% User Flow
    Human --> Frontend
    Frontend --> Controller

    %% Control Plane Flow
    Controller --> APIServer
    APIServer <--> Webhook

    %% Data Flow
    Controller --> Database
    Webhook --> Database
    Controller --> Prometheus
```

Read more about the architecture in the [Architecture](arch-overview.md) section.
