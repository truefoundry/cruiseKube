---
title: "CruiseKube Setup Guide - Installation"
description: "Complete installation guide for CruiseKube. Learn how to set up the Kubernetes optimization operator with Helm, configure prerequisites, and start optimizing your workloads."
keywords:
  - CruiseKube installation
  - Kubernetes optimization setup
  - Helm installation guide
  - Kubernetes operator setup
  - workload optimization
  - resource management
---

# Setup

Get started by following below steps:

## Prerequisites

- **Kubernetes Cluster:** You should have a running Kubernetes cluster. You can use any cloud-based or on-premises Kubernetes distribution.
- **kubectl:** Installed and configured to interact with your Kubernetes cluster.
- **Helm:** Installed for managing Kubernetes applications.
- **Prometheus:** You should have a prometheus installed in your cluster.
??? example "Installing Prometheus"
    We will setup a sample prometheus to read metrics from the ingress controller.

    ```bash
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo update
    helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
      --namespace monitoring \
      --create-namespace \
      --set alertmanager.enabled=false \
      --set grafana.enabled=false \
      --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false
    ```

## Install

### **1. Install CruiseKube using helm**

Add the CruiseKube Helm repository:

```bash
helm repo add cruisekube https://truefoundry.github.io/cruisekube
helm repo update
```
