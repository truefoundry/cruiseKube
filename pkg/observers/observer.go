package observers

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type EventType string

const (
	EventTypeOOM EventType = "oom"
)

type Event struct {
	Type      EventType
	Timestamp time.Time
	ClusterID string
	Namespace string
	PodName   string
	Data      any
}

type Observer interface {
	GetEventType() EventType
	GetEventChannel() <-chan Event

	Start(ctx context.Context, kubeClient kubernetes.Interface, namespace string, stopCh <-chan struct{}) error
	Stop() error
	cache.ResourceEventHandler
}

type BaseObserver struct {
	eventType    EventType
	eventChannel chan Event
	stopCh       chan struct{}
}

func NewBaseObserver(eventType EventType, bufferSize int) *BaseObserver {
	if bufferSize <= 0 {
		bufferSize = 1000
	}

	return &BaseObserver{
		eventType:    eventType,
		eventChannel: make(chan Event, bufferSize),
		stopCh:       make(chan struct{}),
	}
}

func (b *BaseObserver) GetEventType() EventType {
	return b.eventType
}

func (b *BaseObserver) GetEventChannel() <-chan Event {
	return b.eventChannel
}

func (b *BaseObserver) SendEvent(event Event) {
	select {
	case b.eventChannel <- event:
	default:
	}
}

func (b *BaseObserver) OnAdd(obj interface{}, isInInitialList bool) {}
func (b *BaseObserver) OnUpdate(oldObj, newObj interface{})         {}
func (b *BaseObserver) OnDelete(obj interface{})                    {}

type OOMData struct {
	ContainerName string
	MemoryRequest int64
	MemoryLimit   int64
	RestartCount  int32
}
