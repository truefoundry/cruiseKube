CLUSTER_NAME := cruisekube-cluster

.PHONY: help test serve-docs build-docs fetch-contributors

help:
	@echo "Available targets:"
	@echo "  test              - Run tests"
	@echo "  help              - Show this help message"

test:
	@echo "Running tests..."
	@go test ./...

serve-docs: ## Serve docs
	@command -v mkdocs >/dev/null 2>&1 || { \
	  echo "mkdocs not found - please install it (pip install mkdocs-material)"; exit 1; } ; \
	mkdocs serve
	
build-docs: ## Build docs
	@command -v mkdocs >/dev/null 2>&1 || { \
	  echo "mkdocs not found - please install it (pip install mkdocs-material)"; exit 1; } ; \
	mkdocs build

fetch-contributors: ## Fetch contributors
	python3 docs/scripts/fetch_contributors.py

kind-up: ## Update kind cluster
	@kind create cluster --name $(CLUSTER_NAME) --config=test/kind-config.yaml

install-prom: ## Install prometheus
	@helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
	@helm repo update
	@helm upgrade --install prometheus \
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
        --wait --timeout=150s

kind-down: ## Delete kind cluster
	@kind delete cluster --name $(CLUSTER_NAME)

build-images: ## Build images
	@echo "Building images..."
	@docker build -t cruisekube:latest .
	@kind load docker-image cruisekube:latest --name $(CLUSTER_NAME)
	
ick: install-ck

install-ck: ## Update helm chart
	@echo "Updating helm chart..."
	@helm upgrade --install cruisekube \
        ./charts/cruisekube \
        --namespace cruisekube-system \
		--create-namespace \
        --set cruisekubeController.image.repository=cruisekube \
        --set cruisekubeController.image.tag=latest \
        --set cruisekubeController.image.pullPolicy=IfNotPresent \
		--set cruisekubeController.env.CRUISEKUBE_DEPENDENCIES_INCLUSTER_PROMETHEUSURL="http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090" \
        --set cruisekubeController.env.CRUISEKUBE_CONTROLLER_TASKS_CREATESTATS_ENABLED=true \
        --set cruisekubeWebhook.image.repository=cruisekube \
        --set cruisekubeWebhook.image.tag=latest \
        --set cruisekubeWebhook.image.pullPolicy=IfNotPresent \
        --set cruisekubeWebhook.webhook.statsURL.host="http://cruisekube.cruisekube-system.svc.cluster.local:8080" \
		--set postgresql.enabled=true \
		--set cruisekubeFrontend.enabled=false \
        --wait --timeout=60s 

uick: uninstall-ck
uninstall-ck: ## Delete cruisekube
	@echo "Deleting cruisekube..."
	@helm delete cruisekube -n cruisekube-system

uninstall-prom: ## Delete prometheus
	@echo "Deleting prometheus..."
	@helm delete prometheus -n monitoring

.PHONY: index-helm
index-helm: ## Index helm chart
	helm dependency update ./charts/cruisekube
	helm package charts/cruisekube -d docs/
	helm repo index docs/ --url https://github.com/truefoundry/CruiseKube/releases/download/$(RELEASE-NAME)/ --merge ./docs/index.yaml 
	rm -rf docs/cruisekube-*.tgz
