#!/bin/bash

set -e

# Configuration
CLUSTER_NAME="cruisekube-test"
NAMESPACE="cruisekube-system"
PROMETHEUS_NAMESPACE="monitoring"
IMAGE_NAME="cruisekube"
IMAGE_TAG="test"
CONTROLLER_RELEASE_NAME="cruisekubeController"
WEBHOOK_RELEASE_NAME="cruisekubeWebhook"
RELEASE_NAME="cruisekube"
PROMETHEUS_RELEASE_NAME="prometheus"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if ! command -v kind &> /dev/null; then
        log_error "kind is not installed. Please install kind first."
        exit 1
    fi
    
    if ! command -v docker &> /dev/null; then
        log_error "docker is not installed. Please install docker first."
        exit 1
    fi
    
    if ! command -v helm &> /dev/null; then
        log_error "helm is not installed. Please install helm first."
        exit 1
    fi
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed. Please install kubectl first."
        exit 1
    fi
    
    log_success "All prerequisites are installed"
}

create_kind_cluster() {
    log_info "Creating Kind cluster: $CLUSTER_NAME"
    
    if kind get clusters | grep -q "^$CLUSTER_NAME$"; then
        log_warning "Cluster $CLUSTER_NAME already exists. Not touching yet..."
        # kind delete cluster --name $CLUSTER_NAME

    else
    
    cat <<EOF | kind create cluster --name $CLUSTER_NAME --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
  - containerPort: 30090
    hostPort: 9090
    protocol: TCP
EOF
    fi
    log_success "Kind cluster created successfully"
}

build_and_load_image() {
    log_info "Building Docker image: $IMAGE_NAME:$IMAGE_TAG"
    
    # Build the Docker image
    docker build -t $IMAGE_NAME:$IMAGE_TAG .
    
    log_info "Loading image into Kind cluster"
    
    # Load the image into Kind cluster
    kind load docker-image $IMAGE_NAME:$IMAGE_TAG --name $CLUSTER_NAME
    
    log_success "Image built and loaded successfully"
}

setup_namespaces() {
    log_info "Setting up namespaces"
    
    # Set kubectl context to the Kind cluster
    kubectl cluster-info --context kind-$CLUSTER_NAME
    
    # Create cruisekube namespace if it doesn't exist
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    # Create monitoring namespace if it doesn't exist
    kubectl create namespace $PROMETHEUS_NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    log_success "Namespaces setup completed"
}

add_helm_repos() {
    log_info "Adding Helm repositories"
    
    # Add Prometheus community Helm repository
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo update
    
    log_success "Helm repositories added and updated"
}

install_prometheus() {
    log_info "Installing Prometheus (without Grafana and AlertManager)"
    
    # Install only Prometheus from the stack
    helm upgrade --install $PROMETHEUS_RELEASE_NAME \
        prometheus-community/kube-prometheus-stack \
        --namespace $PROMETHEUS_NAMESPACE \
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
    
    log_success "Prometheus installed successfully"
    
    # Wait for Prometheus to be ready
    log_info "Waiting for Prometheus to be ready..."
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=prometheus -n $PROMETHEUS_NAMESPACE --timeout=300s
    
    log_info "Prometheus is accessible at: http://localhost:9090"
}

install_cruisekube_chart() {
    log_info "Installing global cruisekube chart"

    helm upgrade --install $RELEASE_NAME \
        ./charts/cruisekube \
        --namespace $NAMESPACE \
        --set cruisekubeController.image.repository=$IMAGE_NAME \
        --set cruisekubeController.image.tag=$IMAGE_TAG \
        --set cruisekubeController.image.pullPolicy=IfNotPresent \
        --set cruisekubeController.persistence.storageClass=standard \
        --set cruisekubeController.env.cruisekube_DEPENDENCIES_INCLUSTER_PROMETHEUSURL="http://prometheus-kube-prometheus-prometheus.monitoring.svc.cluster.local:9090" \
        --set cruisekubeWebhook.image.repository=$IMAGE_NAME \
        --set cruisekubeWebhook.image.tag=$IMAGE_TAG \
        --set cruisekubeWebhook.image.pullPolicy=IfNotPresent \
        --set cruisekubeWebhook.webhook.statsURL.host="$RELEASE_NAME-cruisekubeWebhook.cruisekube-system.svc.cluster.local:8080" \
        --wait --timeout=300s

    log_success "Global cruisekube chart installed successfully"
}

create_service_monitors() {
    log_info "Creating ServiceMonitors for Prometheus scraping"
    
    # Create ServiceMonitor for controller
    cat <<EOF | kubectl apply -f -
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: cruisekube-controller
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: cruisekube-controller
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: cruisekube-controller
  endpoints:
  - port: metrics
    path: /metrics
    interval: 30s
EOF

    # Create ServiceMonitor for webhook
    cat <<EOF | kubectl apply -f -
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: cruisekube-webhook
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: cruisekube-webhook
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: cruisekube-webhook
  endpoints:
  - port: metrics
    path: /metrics
    interval: 30s
    scheme: https
    tlsConfig:
      insecureSkipVerify: true
EOF
    
    log_success "ServiceMonitors created successfully"
}

verify_installation() {
    log_info "Verifying installation..."
    
    echo ""
    log_info "Checking Helm releases in $NAMESPACE:"
    helm list -n $NAMESPACE
    
    echo ""
    log_info "Checking Helm releases in $PROMETHEUS_NAMESPACE:"
    helm list -n $PROMETHEUS_NAMESPACE
    
    echo ""
    log_info "Checking cruisekube pods:"
    kubectl get pods -n $NAMESPACE
    
    echo ""
    log_info "Checking Prometheus pods:"
    kubectl get pods -n $PROMETHEUS_NAMESPACE | grep prometheus
    
    echo ""
    log_info "Checking services:"
    kubectl get services -n $NAMESPACE
    kubectl get services -n $PROMETHEUS_NAMESPACE | grep prometheus
    
    echo ""
    log_info "Checking webhook configuration:"
    kubectl get mutatingwebhookconfigurations | grep cruisekube || log_warning "No webhook configuration found"
    
    echo ""
    log_info "Checking ServiceMonitors:"
    kubectl get servicemonitors -n $NAMESPACE || log_warning "ServiceMonitors not available"
    
    echo ""
    log_info "Checking controller logs (last 20 lines):"
    kubectl logs -n $NAMESPACE -l app.kubernetes.io/name=cruisekube-controller --tail=20 || log_warning "Controller logs not available yet"
    
    echo ""
    log_info "Checking webhook logs (last 20 lines):"
    kubectl logs -n $NAMESPACE -l app.kubernetes.io/name=cruisekube-webhook --tail=20 || log_warning "Webhook logs not available yet"
}

cleanup() {
    log_info "Cleaning up..."
    
    if [ "$1" = "--cleanup" ]; then
        log_warning "Uninstalling Helm releases..."
        helm uninstall $CONTROLLER_RELEASE_NAME -n $NAMESPACE || true
        helm uninstall $WEBHOOK_RELEASE_NAME -n $NAMESPACE || true
        helm uninstall $PROMETHEUS_RELEASE_NAME -n $PROMETHEUS_NAMESPACE || true
        
        log_warning "Deleting Kind cluster..."
        kind delete cluster --name $CLUSTER_NAME || true
        
        log_success "Cleanup completed"
        exit 0
    fi
}

show_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --cleanup    Clean up the test environment (uninstall charts and delete cluster)"
    echo "  --help       Show this help message"
    echo ""
    echo "This script will:"
    echo "  1. Check prerequisites (kind, docker, helm, kubectl)"
    echo "  2. Create a Kind cluster named '$CLUSTER_NAME'"
    echo "  3. Build and load the cruisekube Docker image"
    echo "  4. Install Prometheus (without Grafana/AlertManager) for metrics collection"
    echo "  5. Install both cruisekubeController and cruisekubeWebhook charts"
    echo "  6. Create ServiceMonitors for Prometheus scraping"
    echo "  7. Verify the installation"
}

main() {
    # Parse command line arguments
    case "${1:-}" in
        --cleanup)
            cleanup --cleanup
            ;;
        --help)
            show_usage
            exit 0
            ;;
        "")
            # Continue with normal execution
            ;;
        *)
            log_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
    
    log_info "Starting cruisekube charts testing with Prometheus..."
    
    check_prerequisites
    create_kind_cluster
    build_and_load_image
    setup_namespaces
    add_helm_repos
    install_prometheus
    install_cruisekube_chart
    create_service_monitors
    verify_installation
    
    log_success "All done! Your cruisekube charts with Prometheus are now running in Kind cluster '$CLUSTER_NAME'"
    
    echo ""
    log_info "Access URLs:"
    echo "  Prometheus: http://localhost:9090"
    echo ""
    log_info "Useful commands:"
    echo "  kubectl get pods -n $NAMESPACE"
    echo "  kubectl get pods -n $PROMETHEUS_NAMESPACE"
    echo "  kubectl logs -n $NAMESPACE -f deployment/cruisekubeController"
    echo "  kubectl logs -n $NAMESPACE -f deployment/cruisekubeWebhook"
    echo "  helm list -n $NAMESPACE"
    echo "  helm list -n $PROMETHEUS_NAMESPACE"
    echo ""
    log_info "To clean up, run: $0 --cleanup"
}

# Run main function
main "$@"
