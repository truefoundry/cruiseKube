package cluster

import (
	"context"
	"time"

	"github.com/truefoundry/cruiseKube/pkg/logging"
)

type TaskFunc func()

type Scheduler struct {
	tasks map[string]*time.Ticker
	quit  chan struct{}
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		tasks: make(map[string]*time.Ticker),
		quit:  make(chan struct{}),
	}
}

func (s *Scheduler) Register(ctx context.Context, name string, schedule string, task func(ctx context.Context) error) {
	duration, err := time.ParseDuration(schedule)
	if err != nil {
		logging.Errorf(ctx, "Failed to parse schedule for task %s: %v", name, err)
		return
	}

	ticker := time.NewTicker(duration)
	s.tasks[name] = ticker

	go func() {
		logging.Infof(ctx, "Launching initial task: %s", name)
		_ = task(context.Background())

		for {
			select {
			case <-ticker.C:
				logging.Infof(ctx, "Launching task: %s", name)
				_ = task(context.Background())
			case <-s.quit:
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *Scheduler) Start(ctx context.Context) {
	logging.Info(ctx, "Scheduler started")
	<-s.quit
}

func (s *Scheduler) Stop(ctx context.Context) {
	logging.Info(ctx, "Stopping scheduler")
	close(s.quit)
}
