package cluster

import (
	"context"
	"time"

	"github.com/truefoundry/cruisekube/pkg/logging"
)

type taskEntry struct {
	ticker *time.Ticker
	lock   chan struct{} // semaphore, size 1
}

type Scheduler struct {
	tasks map[string]*taskEntry
	quit  chan struct{}
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		tasks: make(map[string]*taskEntry),
		quit:  make(chan struct{}),
	}
}

func (s *Scheduler) ScheduleTask(
	ctx context.Context,
	name string,
	schedule string,
	task func(ctx context.Context) error,
) {
	duration, err := time.ParseDuration(schedule)
	if err != nil {
		logging.Errorf(ctx, "Failed to parse schedule for task %s: %v", name, err)
		return
	}

	entry := &taskEntry{
		ticker: time.NewTicker(duration),
		lock:   make(chan struct{}, 1), // semaphore
	}

	if _, exists := s.tasks[name]; exists {
		logging.Errorf(ctx, "Task %s already exists", name)
		return
	}
	s.tasks[name] = entry

	go func() {
		// Run once immediately
		s.executeTask(ctx, name, entry, task)

		for {
			select {
			case <-entry.ticker.C:
				s.executeTask(ctx, name, entry, task)

			case <-s.quit:
				entry.ticker.Stop()
				return
			}
		}
	}()
}

func (s *Scheduler) executeTask(
	ctx context.Context,
	name string,
	entry *taskEntry,
	task func(ctx context.Context) error,
) {
	// Try to acquire semaphore
	select {
	case entry.lock <- struct{}{}:
		// acquired
	default:
		logging.Debugf(ctx, "Task %s is already running, skipping", name)
		return
	}

	defer func() {
		<-entry.lock // release
	}()

	logging.Infof(ctx, "Launching task: %s", name)

	if err := task(ctx); err != nil {
		logging.Errorf(ctx, "Failed to run task %s: %v", name, err)
	}
}

func (s *Scheduler) Wait(ctx context.Context) {
	logging.Info(ctx, "Scheduler started")
	<-s.quit
}

func (s *Scheduler) Stop(ctx context.Context) {
	logging.Info(ctx, "Stopping scheduler")
	close(s.quit)
}
