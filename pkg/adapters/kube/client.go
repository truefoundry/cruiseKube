package kube

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/truefoundry/cruisekube/pkg/contextutils"
	"github.com/truefoundry/cruisekube/pkg/logging"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func NewKubeClient(ctx context.Context, kubeconfigPath string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	// Check if running inside a Kubernetes cluster
	if isRunningInCluster() {
		logging.Infof(ctx, "Detected in-cluster environment, using service account configuration")
		config, err = rest.InClusterConfig()
		if err != nil {
			logging.Errorf(ctx, "Failed to get in-cluster config: %v", err)
			return nil, err
		}
	} else {
		logging.Infof(ctx, "Using kubeconfig file: %s", kubeconfigPath)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}

	config.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt, otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			cluster, ok := contextutils.GetCluster(ctx)
			if !ok {
				return fmt.Sprintf("%s - kube-api-server: unknown", operation)
			}
			return fmt.Sprintf("%s - kube-api-server: %s", operation, cluster)
		}))
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	logging.Infof(ctx, "Kubernetes client initialized successfully")
	return clientset, nil
}

// isRunningInCluster checks if the application is running inside a Kubernetes cluster
// by checking for the presence of service account token files
func isRunningInCluster() bool {
	// Check for service account token and CA cert files
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"); err == nil {
			return true
		}
	}
	return false
}

func NewDynamicClient(ctx context.Context, kubeconfigPath string) (dynamic.Interface, error) {
	var config *rest.Config
	var err error

	// Check if running inside a Kubernetes cluster
	if isRunningInCluster() {
		logging.Infof(ctx, "Detected in-cluster environment, using service account configuration for dynamic client")
		config, err = rest.InClusterConfig()
		if err != nil {
			logging.Errorf(ctx, "Failed to get in-cluster config: %v", err)
			return nil, err
		}
	} else {
		if kubeconfigPath == "" {
			kubeconfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		logging.Infof(ctx, "Using kubeconfig file for dynamic client: %s", kubeconfigPath)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}

	config.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt, otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			cluster, ok := contextutils.GetCluster(ctx)
			if !ok {
				return fmt.Sprintf("%s - kube-api-server: unknown", operation)
			}
			return fmt.Sprintf("%s - kube-api-server: %s", operation, cluster)
		}))
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	logging.Infof(ctx, "Kubernetes dynamic client initialized successfully")
	return dynamicClient, nil
}
