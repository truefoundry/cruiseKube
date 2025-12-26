package oom

import (
	"context"
	"fmt"
	"strings"
	"time"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/task/utils"
)

type Info struct {
	ContainerID        string
	Timestamp          time.Time
	MemoryLimit        int64
	MemoryRequest      int64
	LastObservedMemory int64
}

type Observer struct {
	observedOomsChannel chan Info
	podInformer         cache.SharedIndexInformer
	stopCh              chan struct{}
	kubeClient          kubernetes.Interface
}

func NewObserver(kubeClient kubernetes.Interface) *Observer {
	return &Observer{
		observedOomsChannel: make(chan Info, 1000),
		stopCh:              make(chan struct{}),
		kubeClient:          kubeClient,
	}
}

func (o *Observer) GetObservedOomsChannel() chan Info {
	return o.observedOomsChannel
}

func (o *Observer) Start(ctx context.Context, kubeClient kubernetes.Interface, namespace string) error {
	podInformer, err := setupPodInformer(ctx, kubeClient, o, namespace, o.stopCh)
	if err != nil {
		return fmt.Errorf("failed to setup pod informer: %w", err)
	}
	o.podInformer = podInformer

	watchEventsWithRetries(ctx, kubeClient, o.onEvictionEvent, namespace)
	logging.Infof(ctx, "OOM observer started successfully")

	return nil
}

func (o *Observer) Stop() error {
	close(o.stopCh)
	close(o.observedOomsChannel)
	return nil
}

func (o *Observer) OnUpdate(oldObj, newObj any) {
	oldPod, ok := oldObj.(*apiv1.Pod)
	if !ok {
		panic("invalid old object type")
	}
	newPod, ok := newObj.(*apiv1.Pod)
	if !ok {
		panic("invalid new object type")
	}

	o.checkContainersForOOM(oldPod, newPod)
}

func (o *Observer) OnAdd(_ any, _ bool) {}
func (o *Observer) OnDelete(_ any)      {}

func (o *Observer) checkContainersForOOM(oldPod, newPod *apiv1.Pod) {
	var allContainerStatuses []apiv1.ContainerStatus
	allContainerStatuses = append(allContainerStatuses, newPod.Status.ContainerStatuses...)
	allContainerStatuses = append(allContainerStatuses, newPod.Status.InitContainerStatuses...)

	for _, containerStatus := range allContainerStatuses {
		if containerStatus.RestartCount > 0 &&
			containerStatus.LastTerminationState.Terminated != nil &&
			containerStatus.LastTerminationState.Terminated.Reason == "OOMKilled" {
			oldStatus := findStatus(containerStatus.Name, append(oldPod.Status.ContainerStatuses, oldPod.Status.InitContainerStatuses...))
			if oldStatus != nil && containerStatus.RestartCount > oldStatus.RestartCount {
				workloadInfo := utils.GetWorkloadInfoFromPod(newPod)
				if workloadInfo == nil {
					continue
				}

				containerID := utils.GetWorkloadContainerKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name, containerStatus.Name)

				containerSpec := findContainerSpec(containerStatus.Name, newPod.Spec.Containers, newPod.Spec.InitContainers)
				if containerSpec != nil {
					var memoryLimit int64
					var memoryRequest int64

					if lim, ok := containerSpec.Resources.Limits[apiv1.ResourceMemory]; ok {
						memoryLimit = lim.Value()
					}
					if req, ok := containerSpec.Resources.Requests[apiv1.ResourceMemory]; ok {
						memoryRequest = req.Value()
					}

					oomInfo := Info{
						ContainerID:        containerID,
						Timestamp:          containerStatus.LastTerminationState.Terminated.FinishedAt.Time,
						MemoryLimit:        memoryLimit,
						MemoryRequest:      memoryRequest,
						LastObservedMemory: memoryLimit,
					}

					o.observedOomsChannel <- oomInfo
				}
			}
		}
	}
}

func (o *Observer) onEvictionEvent(ctx context.Context, event *apiv1.Event) {
	for _, oomInfo := range o.parseEvictionEvent(ctx, event) {
		logging.Infof(ctx, "OOM event observed: containerID=%s, limit=%d bytes, request=%d bytes", oomInfo.ContainerID, oomInfo.MemoryLimit, oomInfo.MemoryRequest)
		// TODO: Confirm if handling Eviction events separately is necessary
		// o.observedOomsChannel <- oomInfo
	}
}

func (o *Observer) parseEvictionEvent(ctx context.Context, event *apiv1.Event) []Info {
	if event.Reason != "Evicted" || event.InvolvedObject.Kind != "Pod" {
		return nil
	}

	extractArray := func(annotationsKey string) []string {
		str, found := event.Annotations[annotationsKey]
		if !found {
			return []string{}
		}
		return strings.Split(str, ",")
	}

	offendingContainers := extractArray("offending_containers")
	offendingContainersUsage := extractArray("offending_containers_usage")
	starvedResource := extractArray("starved_resource")

	if len(offendingContainers) != len(offendingContainersUsage) ||
		len(offendingContainers) != len(starvedResource) {
		return nil
	}

	pod, err := o.kubeClient.CoreV1().Pods(event.InvolvedObject.Namespace).Get(ctx, event.InvolvedObject.Name, metav1.GetOptions{})
	if err != nil {
		return nil
	}

	workloadInfo := utils.GetWorkloadInfoFromPod(pod)
	if workloadInfo == nil {
		return nil
	}

	var results []Info
	for i, container := range offendingContainers {
		if starvedResource[i] != "memory" {
			continue
		}

		memory, err := resource.ParseQuantity(offendingContainersUsage[i])
		if err != nil {
			continue
		}

		containerID := utils.GetWorkloadContainerKey(workloadInfo.Kind, workloadInfo.Namespace, workloadInfo.Name, container)

		var memoryLimit int64
		var memoryRequest int64
		containerSpec := findContainerSpec(container, pod.Spec.Containers, pod.Spec.InitContainers)
		if containerSpec != nil {
			if lim, ok := containerSpec.Resources.Limits[apiv1.ResourceMemory]; ok {
				memoryLimit = lim.Value()
			}
			if req, ok := containerSpec.Resources.Requests[apiv1.ResourceMemory]; ok {
				memoryRequest = req.Value()
			}
		}
		if memoryLimit == 0 {
			memoryLimit = memory.Value()
		}

		actualMemoryUsage := memory.Value()

		oomInfo := Info{
			ContainerID:        containerID,
			Timestamp:          event.CreationTimestamp.Time,
			MemoryLimit:        memoryLimit,
			MemoryRequest:      memoryRequest,
			LastObservedMemory: actualMemoryUsage,
		}
		results = append(results, oomInfo)
	}

	return results
}

func findStatus(name string, containerStatuses []apiv1.ContainerStatus) *apiv1.ContainerStatus {
	for _, containerStatus := range containerStatuses {
		if containerStatus.Name == name {
			return &containerStatus
		}
	}
	return nil
}

func findContainerSpec(name string, containers []apiv1.Container, initContainers []apiv1.Container) *apiv1.Container {
	for _, containerSpec := range containers {
		if containerSpec.Name == name {
			return &containerSpec
		}
	}
	for _, containerSpec := range initContainers {
		if containerSpec.Name == name {
			return &containerSpec
		}
	}
	return nil
}
