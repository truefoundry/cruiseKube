package oom

import (
	"context"
	"fmt"

	"github.com/truefoundry/cruisekube/pkg/task"
	"k8s.io/client-go/kubernetes"

	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/repository/storage"
	"github.com/truefoundry/cruisekube/pkg/task/utils"
	"github.com/truefoundry/cruisekube/pkg/types"
)

type Processor struct {
	storage                 *storage.Storage
	kubeClient              kubernetes.Interface
	clusterID               string
	stopCh                  chan struct{}
	applyRecommendationTask *task.ApplyRecommendationTask
}

func NewProcessor(storageRepo *storage.Storage, kubeClient kubernetes.Interface, clusterID string, applyRecommendationTask *task.ApplyRecommendationTask) *Processor {
	return &Processor{
		storage:                 storageRepo,
		kubeClient:              kubeClient,
		clusterID:               clusterID,
		applyRecommendationTask: applyRecommendationTask,
		stopCh:                  make(chan struct{}),
	}
}

func (p *Processor) Start(ctx context.Context, observer *Observer) {
	oomChannel := observer.GetObservedOomsChannel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				logging.Infof(ctx, "OOM processor stopped due to context cancellation")
				return
			case <-p.stopCh:
				logging.Infof(ctx, "OOM processor stopped")
				return
			case oomInfo, ok := <-oomChannel:
				if !ok {
					logging.Infof(ctx, "OOM channel closed, stopping processor")
					return
				}
				p.processOOMEvent(ctx, oomInfo)
			}
		}
	}()

	logging.Infof(ctx, "OOM processor started successfully")
}

func (p *Processor) Stop() {
	close(p.stopCh)
}

func (p *Processor) processOOMEvent(ctx context.Context, oomInfo Info) {
	event := &types.OOMEvent{
		ClusterID:   p.clusterID,
		ContainerID: oomInfo.ContainerID,
		Metadata: types.OOMEventMetadata{
			NodeName:  oomInfo.NodeName,
			PodName:   oomInfo.PodName,
			Namespace: oomInfo.Namespace,
		},
		Timestamp:          oomInfo.Timestamp,
		MemoryLimit:        oomInfo.MemoryLimit,
		MemoryRequest:      oomInfo.MemoryRequest,
		LastObservedMemory: oomInfo.LastObservedMemory,
	}

	if err := p.storage.InsertOOMEvent(event); err != nil {
		logging.Errorf(ctx, "Failed to store OOM event for containerID %s: %v", oomInfo.ContainerID, err)
		return
	}

	logging.Infof(ctx, "OOM event stored: containerID=%s, pod=%s/%s, node=%s, limit=%d bytes, request=%d bytes, observed=%d bytes",
		oomInfo.ContainerID, oomInfo.Namespace, oomInfo.PodName, oomInfo.NodeName, oomInfo.MemoryLimit, oomInfo.MemoryRequest, oomInfo.LastObservedMemory)

	kind, namespace, workloadName, containerName, ok := utils.ParseWorkloadContainerKey(oomInfo.ContainerID)
	if !ok {
		logging.Errorf(ctx, "Failed to parse containerID: %s", oomInfo.ContainerID)
		return
	}

	workloadID := fmt.Sprintf("%s:%s:%s", kind, namespace, workloadName)

	if err := p.storage.UpdateOOMMemoryForContainer(p.clusterID, workloadID, containerName, oomInfo.LastObservedMemory); err != nil {
		logging.Warnf(ctx, "Failed to update OOM memory in stats for %s: %v", oomInfo.ContainerID, err)
	} else {
		logging.Infof(ctx, "Updated OOM memory in stats for workload=%s, container=%s: %d bytes",
			workloadID, containerName, oomInfo.LastObservedMemory)
	}

	if p.applyRecommendationTask == nil {
		logging.Warnf(ctx, "Skipping reactive apply recommendation for node %s: apply recommendation task is not initialized", oomInfo.NodeName)
		return
	}

	logging.Infof(ctx, "Triggering reactive apply recommendation for node %s", oomInfo.NodeName)
	go func(prevCtx context.Context) {
		ctx := context.WithoutCancel(prevCtx)
		if err := p.applyRecommendationTask.RunForNode(ctx, oomInfo.NodeName); err != nil {
			logging.Errorf(ctx, "Failed to apply reactive recommendation for node %s: %v", oomInfo.NodeName, err)
		}
	}(ctx)
}
