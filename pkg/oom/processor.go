package oom

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/repository/storage"
	"github.com/truefoundry/cruisekube/pkg/types"
)

type Processor struct {
	storage    *storage.Storage
	kubeClient kubernetes.Interface
	clusterID  string
	stopCh     chan struct{}
}

func NewProcessor(storageRepo *storage.Storage, kubeClient kubernetes.Interface, clusterID string) *Processor {
	return &Processor{
		storage:    storageRepo,
		kubeClient: kubeClient,
		clusterID:  clusterID,
		stopCh:     make(chan struct{}),
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
		ClusterID:          p.clusterID,
		ContainerID:        oomInfo.ContainerID,
		Timestamp:          oomInfo.Timestamp,
		MemoryLimit:        oomInfo.MemoryLimit,
		MemoryRequest:      oomInfo.MemoryRequest,
		LastObservedMemory: oomInfo.LastObservedMemory,
	}

	if err := p.storage.InsertOOMEvent(event); err != nil {
		logging.Errorf(ctx, "Failed to store OOM event for containerID %s: %v", oomInfo.ContainerID, err)
		return
	}

	logging.Infof(ctx, "OOM event stored: containerID=%s, limit=%d bytes, request=%d bytes, observed=%d bytes",
		oomInfo.ContainerID, oomInfo.MemoryLimit, oomInfo.MemoryRequest, oomInfo.LastObservedMemory)
}
