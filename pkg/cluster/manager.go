package cluster

import (
	"context"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/truefoundry/cruiseKube/pkg/task"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	SingleClusterID = "default"
)

type ClusterMode string

const (
	ClusterModeSingle ClusterMode = "single"
)

type Manager interface {
	RefreshClusters(ctx context.Context) error
	GetAllClusters() map[string]*ClusterClients
	GetClusterIDs() []string
	GetClusterClients(clusterID string) (*ClusterClients, error)
	GetPrometheusConnectionInfo(clusterID string) (*PrometheusConnectionInfo, error)
	GetClusterMode() ClusterMode
	AddTask(task task.Task)
	GetTask(taskName string) (task.Task, error)
	ScheduleAllTasks() error
	StartTasks() error
}

type ClusterClients struct {
	KubeClient       *kubernetes.Clientset
	DynamicClient    dynamic.Interface
	PrometheusClient v1.API
	ClusterID        string
	Healthy          bool
}

type PrometheusConnectionInfo struct {
	URL         string
	BearerToken string
}
