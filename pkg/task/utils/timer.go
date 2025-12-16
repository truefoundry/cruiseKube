package utils

import (
	"context"
	"time"

	"github.com/truefoundry/cruisekube/pkg/logging"
)

func TimeIt(ctx context.Context, name string) func() {
	startTime := time.Now()
	logging.Infof(ctx, "Task %s started", name)
	return func() {
		logging.Infof(ctx, "Task %s completed in %v", name, time.Since(startTime))
	}
}
