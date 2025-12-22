package observers

import (
	"context"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	v1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/truefoundry/cruisekube/pkg/logging"
)

const (
	evictionWatchRetryWait    = 10 * time.Second
	evictionWatchJitterFactor = 0.5
)

func SetupPodInformer(
	ctx context.Context,
	kubeClient kubernetes.Interface,
	resourceEventHandler cache.ResourceEventHandler,
	namespace string,
	stopCh <-chan struct{},
) v1lister.PodLister {
	// Skip watching pending pods (Only Running, Unknown, Succeeded, Failed)
	selector := fields.ParseSelectorOrDie("status.phase!=" + string(apiv1.PodPending))
	podListWatch := cache.NewListWatchFromClient(
		kubeClient.CoreV1().RESTClient(),
		"pods",
		namespace,
		selector,
	)

	indexer, controller := cache.NewIndexerInformer(
		podListWatch,
		&apiv1.Pod{},
		time.Hour,
		resourceEventHandler,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	podLister := v1lister.NewPodLister(indexer)

	go controller.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, controller.HasSynced) {
		logging.Errorf(ctx, "Failed to sync Pod cache during initialization")
	} else {
		logging.Infof(ctx, "Pod informer cache synced successfully")
	}

	return podLister
}

type EventWatcherFunc func(*apiv1.Event)

func WatchEventsWithRetries(
	ctx context.Context,
	kubeClient kubernetes.Interface,
	fieldSelector string,
	handler EventWatcherFunc,
	namespace string,
) {
	go func() {
		options := metav1.ListOptions{
			FieldSelector: fieldSelector,
		}

		watchEventsOnce := func() {
			watchInterface, err := kubeClient.CoreV1().Events(namespace).Watch(ctx, options)
			if err != nil {
				logging.Errorf(ctx, "Cannot initialize watching events with selector %s: %v", fieldSelector, err)
				return
			}
			defer watchInterface.Stop()
			watchEvents(ctx, watchInterface.ResultChan(), handler)
		}

		for {
			select {
			case <-ctx.Done():
				logging.Infof(ctx, "Stopping event watcher for selector: %s", fieldSelector)
				return
			default:
				watchEventsOnce()
				// Wait between attempts
				waitTime := wait.Jitter(evictionWatchRetryWait, evictionWatchJitterFactor)
				logging.Infof(ctx, "Event watch finished for selector %s, retrying in %v", fieldSelector, waitTime)
				time.Sleep(waitTime)
			}
		}
	}()
}

func watchEvents(ctx context.Context, eventChan <-chan watch.Event, handler EventWatcherFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		case watchEvent, ok := <-eventChan:
			if !ok {
				logging.Infof(ctx, "Event channel closed")
				return
			}
			if watchEvent.Type == watch.Added {
				if event, ok := watchEvent.Object.(*apiv1.Event); ok {
					handler(event)
				}
			}
		}
	}
}
