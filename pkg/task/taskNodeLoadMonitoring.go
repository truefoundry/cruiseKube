package task

import (
	"context"
	"fmt"

	"github.com/prometheus/common/model"
	"github.com/truefoundry/cruiseKube/pkg/adapters/metricsProvider/prometheus"
	"github.com/truefoundry/cruiseKube/pkg/contextutils"
	"github.com/truefoundry/cruiseKube/pkg/logging"
	"github.com/truefoundry/cruiseKube/pkg/task/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	NodeOverloadTaintKey    = "cruiseKube.truefoundry.com/overloaded"
	NodeOverloadTaintValue  = "true"
	NodeOverloadTaintEffect = corev1.TaintEffectNoSchedule
	LoadThreshold           = 1.0
	NodeLoadLookback        = "2m"
)

type NodeLoadMonitoringTaskConfig struct {
	Name                     string
	Enabled                  bool
	Schedule                 string
	ClusterID                string
	IsClusterWriteAuthorized bool
}

type NodeLoadMonitoringTask struct {
	config        *NodeLoadMonitoringTaskConfig
	kubeClient    *kubernetes.Clientset
	dynamicClient dynamic.Interface
	promClient    *prometheus.PrometheusProvider
}

func NewNodeLoadMonitoringTask(ctx context.Context, kubeClient *kubernetes.Clientset, dynamicClient dynamic.Interface, promClient *prometheus.PrometheusProvider, config *NodeLoadMonitoringTaskConfig) *NodeLoadMonitoringTask {
	return &NodeLoadMonitoringTask{
		config:        config,
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
		promClient:    promClient,
	}
}

func (n *NodeLoadMonitoringTask) GetCoreTask() any {
	return n
}

func (n *NodeLoadMonitoringTask) GetName() string {
	return n.config.Name
}

func (n *NodeLoadMonitoringTask) GetSchedule() string {
	return n.config.Schedule
}

func (n *NodeLoadMonitoringTask) IsEnabled() bool {
	return n.config.Enabled
}

func (n *NodeLoadMonitoringTask) Run(ctx context.Context) error {
	ctx = contextutils.WithTask(ctx, n.config.Name)
	ctx = contextutils.WithCluster(ctx, n.config.ClusterID)

	if !n.config.IsClusterWriteAuthorized {
		logging.Infof(ctx, "Cluster %s is not write authorized, skipping NodeLoadMonitoring task", n.config.ClusterID)
		return nil
	}

	nodes, err := n.getAllNodes(ctx)
	if err != nil {
		logging.Errorf(ctx, "Error getting nodes: %v", err)
		return err
	}

	logging.Infof(ctx, "Found %d nodes to monitor", len(nodes.Items))

	nodeLoadData, err := n.getNodeLoadMetrics(ctx)
	if err != nil {
		logging.Errorf(ctx, "Error getting node load metrics: %v", err)
		return err
	}

	processedNodes := 0
	taintsAdded := 0
	taintsRemoved := 0

	for _, node := range nodes.Items {
		processedNodes++
		loadAvg, exists := nodeLoadData[node.Name]

		isOverloaded := exists && loadAvg > LoadThreshold
		hasOverloadTaint := n.nodeHasOverloadTaint(&node)

		if isOverloaded && !hasOverloadTaint {
			if err := n.addOverloadTaint(ctx, &node); err != nil {
				logging.Errorf(ctx, "Error adding taint to node %s: %v", node.Name, err)
			} else {
				taintsAdded++
				logging.Infof(ctx, "Added overload taint to node %s (load: %.2f%%)", node.Name, loadAvg*100)
			}
		} else if !isOverloaded && hasOverloadTaint {
			if err := n.removeOverloadTaint(ctx, &node); err != nil {
				logging.Errorf(ctx, "Error removing taint from node %s: %v", node.Name, err)
			} else {
				taintsRemoved++
				if exists {
					logging.Infof(ctx, "Removed overload taint from node %s (load: %.2f%%)", node.Name, loadAvg*100)
				} else {
					logging.Infof(ctx, "Removed overload taint from node %s (no load data)", node.Name)
				}
			}
		}
	}

	return nil
}

func (n *NodeLoadMonitoringTask) getAllNodes(ctx context.Context) (*corev1.NodeList, error) {
	nodes, err := n.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}
	return nodes, nil
}

func (n *NodeLoadMonitoringTask) getNodeLoadMetrics(ctx context.Context) (map[string]float64, error) {
	query := `
	    min_over_time(
			(
					max by (node) (max by (node) (node_load1{job="node-exporter"}))
				/
					(max by (node) (kube_node_status_capacity{job="kube-state-metrics",resource="cpu"}))
			)[%s:]
		)
	`

	query = fmt.Sprintf(query, NodeLoadLookback)
	logging.Infof(ctx, "Using query: %s", utils.CompressQueryForLogging(query))

	result, warnings, err := n.promClient.ExecuteQueryWithRetry(ctx, n.config.ClusterID, query, "node-load-monitoring")
	if err != nil {
		return nil, fmt.Errorf("error querying prometheus for node load metrics: %w", err)
	}
	if len(warnings) > 0 {
		logging.Infof(ctx, "Warnings from Prometheus query: %v", warnings)
	}

	return n.parseNodeLoadResults(ctx, result)
}

func (n *NodeLoadMonitoringTask) parseNodeLoadResults(ctx context.Context, result model.Value) (map[string]float64, error) {
	nodeLoadData := make(map[string]float64)

	if result.Type() != model.ValVector {
		return nil, fmt.Errorf("expected vector result, got %s", result.Type())
	}

	vector := result.(model.Vector)
	for _, sample := range vector {
		nodeName := string(sample.Metric["node"])
		if nodeName != "" {
			loadValue := float64(sample.Value)
			nodeLoadData[nodeName] = loadValue
			logging.Infof(ctx, "Node %s load: %.2f%%", nodeName, loadValue*100)
		}
	}

	return nodeLoadData, nil
}

func (n *NodeLoadMonitoringTask) nodeHasOverloadTaint(node *corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == NodeOverloadTaintKey &&
			taint.Value == NodeOverloadTaintValue &&
			taint.Effect == NodeOverloadTaintEffect {
			return true
		}
	}
	return false
}

func (n *NodeLoadMonitoringTask) addOverloadTaint(ctx context.Context, node *corev1.Node) error {
	nodeCopy := node.DeepCopy()

	newTaint := corev1.Taint{
		Key:    NodeOverloadTaintKey,
		Value:  NodeOverloadTaintValue,
		Effect: NodeOverloadTaintEffect,
	}

	nodeCopy.Spec.Taints = append(nodeCopy.Spec.Taints, newTaint)

	_, err := n.kubeClient.CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to add taint to node %s: %w", node.Name, err)
	}

	return nil
}

func (n *NodeLoadMonitoringTask) removeOverloadTaint(ctx context.Context, node *corev1.Node) error {
	nodeCopy := node.DeepCopy()

	var newTaints []corev1.Taint
	for _, taint := range nodeCopy.Spec.Taints {
		if taint.Key != NodeOverloadTaintKey ||
			taint.Value != NodeOverloadTaintValue ||
			taint.Effect != NodeOverloadTaintEffect {
			newTaints = append(newTaints, taint)
		}
	}

	nodeCopy.Spec.Taints = newTaints

	_, err := n.kubeClient.CoreV1().Nodes().Update(ctx, nodeCopy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove taint from node %s: %w", node.Name, err)
	}

	return nil
}
