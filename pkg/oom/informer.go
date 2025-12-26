package oom

import (
	"context"
	"fmt"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/truefoundry/cruisekube/pkg/logging"
)

const (
	evictionWatchRetryWait    = 10 * time.Second
	evictionWatchJitterFactor = 0.5
)

func setupPodInformer(
	ctx context.Context,
	kubeClient kubernetes.Interface,
	resourceEventHandler cache.ResourceEventHandler,
	namespace string,
	stopCh <-chan struct{},
) (cache.SharedIndexInformer, error) {
	// Skip watching pending pods (only Running, Unknown, Succeeded, Failed)
	// Similar logic to https://github.com/kubernetes/autoscaler/blob/f5c59050c1872dee2be33472a8e987f9e1fb1424/vertical-pod-autoscaler/pkg/recommender/input/cluster_feeder.go#L166
	selector := fields.ParseSelectorOrDie("status.phase!=" + string(apiv1.PodPending))
	podListWatch := cache.NewListWatchFromClient(
		kubeClient.CoreV1().RESTClient(),
		"pods",
		namespace,
		selector,
	)

	informer := cache.NewSharedIndexInformer(
		podListWatch,
		&apiv1.Pod{},
		time.Hour,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	_, err := informer.AddEventHandler(resourceEventHandler)
	if err != nil {
		return nil, fmt.Errorf("failed to add oom event handler to pod informer: %w", err)
	}

	go informer.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, informer.HasSynced) {
		return nil, fmt.Errorf("failed to sync pod cache for oom observer")
	}

	logging.Infof(ctx, "OOM observer pod informer cache synced successfully")
	return informer, nil
}

func watchEventsWithRetries(
	ctx context.Context,
	kubeClient kubernetes.Interface,
	handler func(context.Context, *apiv1.Event),
	namespace string,
) {
	go func() {
		options := metav1.ListOptions{
			FieldSelector: "reason=Evicted",
		}

		watchEventsOnce := func() {
			watchInterface, err := kubeClient.CoreV1().Events(namespace).Watch(ctx, options)
			if err != nil {
				logging.Errorf(ctx, "Cannot initialize watching events: %v", err)
				return
			}
			defer watchInterface.Stop()
			watchEvents(ctx, watchInterface.ResultChan(), handler)
		}

		for {
			select {
			case <-ctx.Done():
				logging.Infof(ctx, "Stopping OOM event watcher")
				return
			default:
				watchEventsOnce()
				// Wait between attempts, retrying too often breaks API server.
				waitTime := wait.Jitter(evictionWatchRetryWait, evictionWatchJitterFactor)
				logging.Infof(ctx, "OOM event watch attempt finished, retrying in %v", waitTime)
				time.Sleep(waitTime)
			}
		}
	}()
}

func watchEvents(ctx context.Context, eventChan <-chan watch.Event, handler func(context.Context, *apiv1.Event)) {
	for {
		select {
		case <-ctx.Done():
			return
		case watchEvent, ok := <-eventChan:
			if !ok {
				logging.Infof(ctx, "OOM event channel closed")
				return
			}
			if watchEvent.Type == watch.Added {
				if event, ok := watchEvent.Object.(*apiv1.Event); ok {
					handler(ctx, event)
				}
			}
		}
	}
}
