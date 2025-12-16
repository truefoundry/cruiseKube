package cluster

import (
	"context"
	"fmt"
	"sync"

	"github.com/truefoundry/cruiseKube/pkg/task"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type SingleClusterManager struct {
	clusterClients  map[string]*ClusterClients
	mu              sync.RWMutex
	scheduler       *Scheduler
	registeredTasks map[string]task.Task
}

func NewSingleClusterManager(ctx context.Context, kubeClient *kubernetes.Clientset, dynamicClient dynamic.Interface, promClient v1.API) *SingleClusterManager {
	defaultClients := &ClusterClients{
		KubeClient:       kubeClient,
		DynamicClient:    dynamicClient,
		PrometheusClient: promClient,
		ClusterID:        SingleClusterID,
		Healthy:          true,
	}

	clusterClients := map[string]*ClusterClients{
		SingleClusterID: defaultClients,
	}

	return &SingleClusterManager{
		clusterClients:  clusterClients,
		scheduler:       NewScheduler(),
		registeredTasks: make(map[string]task.Task),
	}
}

func (m *SingleClusterManager) AddTask(task task.Task) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !task.IsEnabled() {
		return
	}

	m.registeredTasks[task.GetName()] = task
}

func (m *SingleClusterManager) GetTask(taskName string) (task.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, exists := m.registeredTasks[taskName]
	if !exists {
		return nil, fmt.Errorf("task %s not found", taskName)
	}

	return task, nil
}

func (m *SingleClusterManager) ScheduleAllTasks() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for taskName, task := range m.registeredTasks {
		m.scheduler.Register(context.Background(), taskName, task.GetSchedule(), task.Run)
	}

	return nil
}

func (m *SingleClusterManager) StartTasks() error {
	m.scheduler.Start(context.Background())
	return nil
}

func (m *SingleClusterManager) RefreshClusters(ctx context.Context) error {
	return nil
}

func (m *SingleClusterManager) GetAllClusters() map[string]*ClusterClients {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ClusterClients)
	for id, clients := range m.clusterClients {
		result[id] = clients
	}

	return result
}

func (m *SingleClusterManager) GetClusterIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.clusterClients))
	for id := range m.clusterClients {
		ids = append(ids, id)
	}

	return ids
}

func (m *SingleClusterManager) GetClusterClients(clusterID string) (*ClusterClients, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	clients, exists := m.clusterClients[clusterID]
	if !exists {
		return nil, fmt.Errorf("cluster %s not found", clusterID)
	}

	return clients, nil
}

func (m *SingleClusterManager) GetPrometheusConnectionInfo(clusterID string) (*PrometheusConnectionInfo, error) {
	return &PrometheusConnectionInfo{
		URL:         "",
		BearerToken: "",
	}, nil
}

func (m *SingleClusterManager) GetClusterMode() ClusterMode {
	return ClusterModeSingle
}
