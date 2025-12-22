package oom

import (
	"context"
	"strings"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	v1lister "k8s.io/client-go/listers/core/v1"

	"github.com/truefoundry/cruisekube/pkg/observers"
)

type Observer struct {
	*observers.BaseObserver
	podLister v1lister.PodLister
}

func NewObserver() *Observer {
	return &Observer{
		BaseObserver: observers.NewBaseObserver(observers.EventTypeOOM, 1000),
	}
}

func (o *Observer) Start(ctx context.Context, kubeClient kubernetes.Interface, namespace string, stopCh <-chan struct{}) error {
	o.podLister = observers.SetupPodInformer(ctx, kubeClient, o, namespace, stopCh)
	observers.WatchEventsWithRetries(ctx, kubeClient, "reason=Evicted", o.onEvictionEvent, namespace)
	return nil
}

func (o *Observer) Stop() error {
	return nil
}

func (o *Observer) OnUpdate(oldObj, newObj any) {
	oldPod, ok := oldObj.(*apiv1.Pod)
	if !ok {
		return
	}
	newPod, ok := newObj.(*apiv1.Pod)
	if !ok {
		return
	}

	o.checkContainersForOOM(oldPod, newPod, newPod.Spec.Containers)
	o.checkContainersForOOM(oldPod, newPod, newPod.Spec.InitContainers)
}

func (o *Observer) checkContainersForOOM(oldPod, newPod *apiv1.Pod, containers []apiv1.Container) {
	for _, containerStatus := range newPod.Status.ContainerStatuses {
		if containerStatus.RestartCount > 0 &&
			containerStatus.LastTerminationState.Terminated != nil &&
			containerStatus.LastTerminationState.Terminated.Reason == "OOMKilled" {

			oldStatus := findStatus(containerStatus.Name, oldPod.Status.ContainerStatuses)
			if oldStatus != nil && containerStatus.RestartCount > oldStatus.RestartCount {
				containerSpec := findSpec(containerStatus.Name, containers)
				if containerSpec != nil {
					var memRequest, memLimit int64
					if req, ok := containerSpec.Resources.Requests[apiv1.ResourceMemory]; ok {
						memRequest = req.Value()
					}
					if lim, ok := containerSpec.Resources.Limits[apiv1.ResourceMemory]; ok {
						memLimit = lim.Value()
					}

					event := observers.Event{
						Type:      observers.EventTypeOOM,
						Timestamp: containerStatus.LastTerminationState.Terminated.FinishedAt.Time,
						ClusterID: "", // Will be set by processor
						Namespace: newPod.Namespace,
						PodName:   newPod.Name,
						Data: observers.OOMData{
							ContainerName: containerStatus.Name,
							MemoryRequest: memRequest,
							MemoryLimit:   memLimit,
							RestartCount:  containerStatus.RestartCount,
						},
					}

					o.SendEvent(event)
				}
			}
		}
	}
}

func (o *Observer) onEvictionEvent(event *apiv1.Event) {
	for _, oomEvent := range parseEvictionEvent(event) {
		o.SendEvent(oomEvent)
	}
}

func parseEvictionEvent(event *apiv1.Event) []observers.Event {
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

	var results []observers.Event
	for i, container := range offendingContainers {
		if starvedResource[i] != "memory" {
			continue
		}

		memory, err := resource.ParseQuantity(offendingContainersUsage[i])
		if err != nil {
			continue
		}

		event := observers.Event{
			Type:      observers.EventTypeOOM,
			Timestamp: event.CreationTimestamp.Time,
			ClusterID: "",
			Namespace: event.InvolvedObject.Namespace,
			PodName:   event.InvolvedObject.Name,
			Data: observers.OOMData{
				ContainerName: container,
				MemoryRequest: memory.Value(),
				MemoryLimit:   0, // Not available in eviction event
				RestartCount:  0,
			},
		}
		results = append(results, event)
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

func findSpec(name string, containers []apiv1.Container) *apiv1.Container {
	for _, containerSpec := range containers {
		if containerSpec.Name == name {
			return &containerSpec
		}
	}
	return nil
}
