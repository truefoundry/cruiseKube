---
title: "CruiseKube Development Environment"
description: "Set up your development environment for CruiseKube. Learn how to build, test, and contribute to the project with local Kubernetes clusters."
keywords:
  - CruiseKube development
  - development environment setup
  - local development
  - Kubernetes development
  - contributing to CruiseKube
---

# Development Environment

This page describes how to set up and work with the CruiseKube development environment.

## 1. Get required tools
Ensure you have the following tools installed:

- Go 1.21 or later
- Docker
- Kubernetes cluster (minikube or kind recommended for local development)
- Kubectl
- Helm 
- Kind
- Make 

## 2. Clone the repository

Clone the CruiseKube repository from GitHub:

```bash
git clone https://github.com/truefoundry/cruiseKube.git
cd cruiseKube
```

!!! tip "Make sure you check out the documentation and architecture before making your changes."

## 3. Build images

To build the CruiseKube images, run:

```bash
docker build -t cruisekube:latest .
```

## 4. Start Kind cluster

First, create a Kind cluster:

```bash
kind create cluster --name cruisekube --config=test/kind-config.yaml
```

Then load the built image into the cluster:

```bash
kind load docker-image cruisekube:latest --name cruisekube
```

## 5. Install Prometheus

To install Prometheus, run:

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm upgrade --install prometheus \
        prometheus-community/kube-prometheus-stack \
        --namespace monitoring \
        --create-namespace \
        --set prometheus.service.type=NodePort \
        --set prometheus.service.nodePort=30090 \
        --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
        --set prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues=false \
        --set prometheus.prometheusSpec.ruleSelectorNilUsesHelmValues=false \
        --set grafana.enabled=false \
        --set alertmanager.enabled=false \
        --set kubeStateMetrics.enabled=true \
        --set nodeExporter.enabled=true \
        --set prometheusOperator.enabled=true \
        --wait --timeout=600s
```

## 6. Install CruiseKube

To install CruiseKube, run:

```bash
helm upgrade --install cruisekube \
    ./charts/cruisekube \
    --namespace cruisekube \
    --create-namespace \
    --set cruisekubeController.image.repository=cruisekube \
    --set cruisekubeController.image.tag=latest \
    --set cruisekubeController.image.pullPolicy=Never \
    --set cruisekubeController.env.CRUISEKUBE_DEPENDENCIES_INCLUSTER_PROMETHEUSURL="http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090" \
    --set cruisekubeController.env.CRUISEKUBE_CONTROLLER_TASKS_CREATESTATS_ENABLED=true \
    --set cruisekubeWebhook.image.repository=cruisekube \
    --set cruisekubeWebhook.image.tag=latest \
    --set cruisekubeWebhook.image.pullPolicy=Never \
    --set cruisekubeWebhook.webhook.statsURL.host="https://localhost:8080" \
    --set postgresql.enabled=true \
    --set cruisekubeFrontend.enabled=false \
    --wait --timeout=60s
```

> Note: We have disabled the frontend for local development to simplify the setup.

To redeploy your changes after making code modifications, simply rebuild the image and reload it into the cluster:

```bash
docker build -t cruisekube:latest .
kind load docker-image cruisekube:latest --name cruisekube
```

Then restart the CruiseKube controller:

```bash
kubectl rollout restart deployment/cruisekube-controller -n cruisekube
kubectl rollout restart deployment/cruisekube-webhook -n cruisekube
```

## 8. Delete the deployment

To delete the CruiseKube deployment, run:

```bash
helm uninstall cruisekube -n cruisekube
```

