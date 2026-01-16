package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/truefoundry/cruisekube/pkg/cluster"
	"github.com/truefoundry/cruisekube/pkg/logging"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type TaskTriggerResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration,omitempty"`
}

func HandleTaskTrigger(c *gin.Context) {
	ctx := c.Request.Context()
	mgr := c.MustGet("clusterManager").(cluster.Manager)
	clusterID := c.Param("clusterID")
	taskName := c.Param("taskName")

	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String("cluster.id", clusterID),
		attribute.String("task.name", taskName),
	)

	logging.Infof(ctx, "Manual task trigger for task '%s' in cluster '%s' by %s", taskName, clusterID, c.ClientIP())

	task, err := mgr.GetTask(taskName)
	if err != nil {
		logging.Errorf(ctx, "Task '%s' not found: %v", taskName, err)
		c.JSON(http.StatusNotFound, TaskTriggerResponse{
			Status: "error",
			Error:  fmt.Sprintf("Task '%s' not found", taskName),
		})
		return
	}

	if !task.IsEnabled() {
		logging.Warnf(ctx, "Task '%s' is disabled", taskName)
		c.JSON(http.StatusBadRequest, TaskTriggerResponse{
			Status: "error",
			Error:  fmt.Sprintf("Task '%s' is disabled", taskName),
		})
		return
	}

	startedAt := time.Now()

	logging.Infof(ctx, "Starting synchronous execution of task '%s'", taskName)
	if err := task.Run(ctx); err != nil {
		duration := time.Since(startedAt)
		logging.Errorf(ctx, "Task '%s' failed after %v: %v", taskName, duration, err)
		c.JSON(http.StatusInternalServerError, TaskTriggerResponse{
			Status:   "error",
			Error:    err.Error(),
			Duration: duration.String(),
		})
		return
	}

	duration := time.Since(startedAt)
	logging.Infof(ctx, "Task '%s' completed successfully in %v", taskName, duration)
	c.JSON(http.StatusOK, TaskTriggerResponse{
		Status:   "success",
		Message:  fmt.Sprintf("Task '%s' completed successfully", taskName),
		Duration: duration.String(),
	})
}
