package oom

import (
	"context"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/truefoundry/cruisekube/pkg/config"
	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/repository/storage"
	"github.com/truefoundry/cruisekube/pkg/task/utils"
	"github.com/truefoundry/cruisekube/pkg/types"
)

type Processor struct {
	storage    *storage.Storage
	kubeClient kubernetes.Interface
	clusterID  string
	stopCh     chan struct{}
	cfg        *config.Config
}

func NewProcessor(storageRepo *storage.Storage, kubeClient kubernetes.Interface, clusterID string, cfg *config.Config) *Processor {
	return &Processor{
		storage:    storageRepo,
		kubeClient: kubeClient,
		clusterID:  clusterID,
		cfg:        cfg,
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
	kind, namespace, workloadName, containerName, ok := utils.ParseWorkloadContainerKey(oomInfo.ContainerID)
	if !ok {
		logging.Errorf(ctx, "Failed to parse containerID: %s", oomInfo.ContainerID)
		return
	}

	workloadID := utils.GetWorkloadKey(kind, namespace, workloadName)

	latestEvent, err := p.storage.GetLatestOOMEventForContainer(p.clusterID, oomInfo.ContainerID, oomInfo.PodName)
	if err != nil {
		logging.Warnf(ctx, "Failed to check latest OOM event: %v", err)
	} else if latestEvent != nil {
		timeSinceLastOOM := time.Since(latestEvent.Timestamp)
		cooldownDuration := time.Duration(p.cfg.RecommendationSettings.OOMCooldownMinutes) * time.Minute

		if timeSinceLastOOM < cooldownDuration {
			remainingCooldown := cooldownDuration - timeSinceLastOOM
			logging.Infof(ctx, "OOM cooldown active for pod %s/%s (container %s): last OOM was %.1f minutes ago, skipping eviction (%.1f minutes remaining)",
				oomInfo.Namespace, oomInfo.PodName, oomInfo.ContainerID, timeSinceLastOOM.Minutes(), remainingCooldown.Minutes())
			return
		}
	}

	event := &types.OOMEvent{
		ClusterID:          p.clusterID,
		ContainerID:        oomInfo.ContainerID,
		PodName:            oomInfo.PodName,
		NodeName:           oomInfo.NodeName,
		Namespace:          oomInfo.Namespace,
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

	if err := p.storage.UpdateOOMMemoryForContainer(p.clusterID, workloadID, containerName, oomInfo.LastObservedMemory); err != nil {
		logging.Warnf(ctx, "Failed to update OOM memory in stats for %s: %v, skipping eviction", oomInfo.ContainerID, err)
		return
	}
	logging.Infof(ctx, "Updated OOM memory in stats for workload=%s, container=%s: %d bytes",
		workloadID, containerName, oomInfo.LastObservedMemory)

	if p.cfg.RecommendationSettings.DisableMemoryApplication {
		logging.Infof(ctx, "Memory application is disabled in configuration, skipping eviction for pod %s/%s", oomInfo.Namespace, oomInfo.PodName)
		return
	}

	pod, err := p.kubeClient.CoreV1().Pods(oomInfo.Namespace).Get(ctx, oomInfo.PodName, metav1.GetOptions{})
	if err != nil {
		logging.Errorf(ctx, "Failed to get pod %s/%s for eviction: %v", oomInfo.Namespace, oomInfo.PodName, err)
		return
	}

	if pod.Annotations[utils.ExcludedAnnotation] == "true" {
		logging.Infof(ctx, "Pod %s/%s has cruisekube excluded annotation, skipping eviction", oomInfo.Namespace, oomInfo.PodName)
		return
	}

	workloadInfo := utils.GetWorkloadInfoFromPod(pod)
	if workloadInfo != nil {
		workloadID := strings.ReplaceAll(utils.GetWorkloadKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name), "/", ":")

		workloadOverrides, err := p.storage.GetWorkloadOverrides(p.clusterID, workloadID)
		if err != nil {
			logging.Warnf(ctx, "Failed to fetch workload overrides for %s, proceeding with eviction: %v", workloadID, err)
		} else if workloadOverrides.Enabled != nil && !*workloadOverrides.Enabled {
			logging.Infof(ctx, "Workload %s is disabled via overrides, skipping eviction", workloadID)
			return
		}
	}

	logging.Infof(ctx, "Evicting pod %s/%s after OOM", oomInfo.Namespace, oomInfo.PodName)
	evicted, errStr := utils.EvictPod(ctx, p.kubeClient, pod)
	if !evicted || errStr != "" {
		logging.Errorf(ctx, "Failed to evict pod %s/%s: %v", oomInfo.Namespace, oomInfo.PodName, errStr)
		return
	}

	logging.Infof(ctx, "Successfully evicted pod %s/%s after OOM. New pod will be created with updated memory recommendations via webhook", oomInfo.Namespace, oomInfo.PodName)
}
