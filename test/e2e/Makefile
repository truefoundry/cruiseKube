# Variables
CLUSTER_NAME := cruisekube-e2e
NAMESPACE := cruisekube-system
CONTROLLER_IMAGE := cruisekube:latest
T ?= ./tests
TIMEOUT := 400s

.PHONY: all setup clean test

all:
	@echo "Run 'make build-images' to build images"
	@echo "Run 'make setup' to setup environment"
	@echo "Run 'make test' to run tests"
	@echo "Run 'make clean' to clean environment"

setup: kind-up apply-deps
clean: kind-down

test:
	@echo "Running KUTTL tests..."
	@echo "Current context: $$(kubectl config current-context)"
	@kubectl kuttl test --start-kind=false --test $(T)
	@echo "E2E Tests completed!"

.PHONY: build-images
build-images: ## Build controller image
	@echo "Building controller image..."
	@cd ../.. && docker build -t $(CONTROLLER_IMAGE) .
	@echo "Controller image built successfully"

.PHONY: kind-up
kind-up: ## Create Kind cluster and load images
	@echo "Creating Kind cluster '$(CLUSTER_NAME)'..."
	@if ! kind get clusters | grep -q $(CLUSTER_NAME); then \
		kind create cluster --config ./kind-config.yaml; \
		echo "Kind cluster '$(CLUSTER_NAME)' created successfully."; \
	else \
		echo "Kind cluster '$(CLUSTER_NAME)' already exists."; \
	fi
	@echo "Switching to kind cluster context..."
	@kubectl config use-context kind-$(CLUSTER_NAME)
	@echo "Waiting for cluster to be ready..."
	@kubectl wait --for=condition=Ready nodes --all --timeout=120s
	kubectl scale --replicas=1 deployment/coredns -n kube-system
	@echo "Loading controller image into kind cluster..."
	@docker save $(CONTROLLER_IMAGE) | docker exec -i $(CLUSTER_NAME)-control-plane ctr -n k8s.io images import -
	@echo "Image loaded successfully"

.PHONY: kind-down
kind-down: ## Delete Kind cluster
	@echo "Deleting kind cluster '$(CLUSTER_NAME)'..."
	@if kind get clusters | grep -q $(CLUSTER_NAME); then \
		kind delete cluster --name $(CLUSTER_NAME); \
		echo "Kind cluster '$(CLUSTER_NAME)' deleted successfully."; \
	else \
		echo "Kind cluster '$(CLUSTER_NAME)' does not exist."; \
	fi

.PHONY: apply-deps
apply-deps: ## Install all dependencies
	@echo "Creating namespace..."
	@kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@echo "Installing PostgreSQL..."
	@kubectl apply -f ./manifest/postgres.yaml
	@echo "Waiting for PostgreSQL to be ready..."
	@kubectl wait --for=condition=Ready pods -l app=postgres -n $(NAMESPACE) --timeout=180s
	@echo "Building Helm dependencies..."
	@cd ../../charts/cruisekube && helm dependency build
	@echo "Installing CruiseKube via Helm..."
	@helm upgrade --wait --timeout $(TIMEOUT) --install --create-namespace cruisekube ../../charts/cruisekube -n $(NAMESPACE) -f ./manifest/global/values-cruisekube.yaml
	@echo "All dependencies installed successfully."

.PHONY: reload-images
reload-images: ## Reload images into kind and restart deployments
	@echo "Loading images into kind cluster..."
	@docker save $(CONTROLLER_IMAGE) | docker exec -i $(CLUSTER_NAME)-control-plane ctr -n k8s.io images import -
	@echo "Restarting cruisekube controller deployment..."
	@kubectl rollout restart deployment cruisekube-controller -n $(NAMESPACE)
	@echo "Images reloaded and deployment restarted."
