package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/truefoundry/autopilot-oss/pkg/cluster"
	"github.com/truefoundry/autopilot-oss/pkg/config"
	"github.com/truefoundry/autopilot-oss/pkg/logging"
	"go.opentelemetry.io/otel/attribute"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func GetPrometheusConfigHandler(c *gin.Context) {
	ctx := c.Request.Context()
	clusterID := c.Param("clusterID")

	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("cluster", clusterID))

	logging.Infof(ctx, "Getting Prometheus config for cluster %s", clusterID)

	cfg := config.GetConfigFromGinContext(c)
	mgr := c.MustGet("clusterManager").(cluster.Manager)

	var prometheusURL string
	switch cfg.ControllerMode {
	case config.ClusterModeLocal:
		prometheusURL = cfg.Dependencies.Local.PrometheusURL
	case config.ClusterModeInCluster:
		prometheusURL = cfg.Dependencies.InCluster.PrometheusURL
	default:
		logging.Errorf(ctx, "Unknown controller mode: %s", cfg.ControllerMode)
		c.JSON(500, gin.H{
			"url":       "",
			"connected": false,
			"error":     fmt.Sprintf("Unknown controller mode: %s", cfg.ControllerMode),
		})
		return
	}

	clients, err := mgr.GetClusterClients(clusterID)
	if err != nil {
		logging.Errorf(ctx, "Failed to get cluster clients for %s: %v", clusterID, err)
		c.JSON(500, gin.H{
			"url":       prometheusURL,
			"connected": false,
			"error":     fmt.Sprintf("Failed to get cluster clients: %v", err),
		})
		return
	}

	connected := false
	var connectionError string

	if clients.PrometheusClient != nil {
		_, err := clients.PrometheusClient.Buildinfo(ctx)
		if err != nil {
			connectionError = fmt.Sprintf("Connection test failed: %v", err)
			logging.Errorf(ctx, "Prometheus connection test failed for cluster %s: %v", clusterID, err)
		} else {
			connected = true
			logging.Infof(ctx, "Prometheus connection test successful for cluster %s", clusterID)
		}
	} else {
		connectionError = "Prometheus client not available"
		logging.Errorf(ctx, "Prometheus client not available for cluster %s", clusterID)
	}

	response := gin.H{
		"url":       prometheusURL,
		"connected": connected,
	}
	if connectionError != "" {
		response["error"] = connectionError
	}

	c.JSON(200, response)
}

