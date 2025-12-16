package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/truefoundry/cruiseKube/pkg/cluster"
	"github.com/truefoundry/cruiseKube/pkg/logging"
	"github.com/truefoundry/cruiseKube/pkg/repository/storage"
	"github.com/truefoundry/cruiseKube/pkg/types"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func HandleUIStaticFiles(c *gin.Context) {
	ctx := c.Request.Context()

	// Get the filename from the URL path
	urlPath := c.Request.URL.Path
	fileName := filepath.Base(urlPath)

	paths := []string{
		filepath.Join("web", fileName),
	}

	var content []byte
	var err error
	for _, path := range paths {
		// #nosec G304 - paths are controlled static file paths
		content, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		logging.Errorf(ctx, "Failed to read static file %s: %v", fileName, err)
		c.String(http.StatusNotFound, "File not found")
		return
	}

	contentType := "text/plain"
	switch {
	case strings.HasSuffix(fileName, ".js"):
		contentType = "application/javascript"
	case strings.HasSuffix(fileName, ".css"):
		contentType = "text/css"
	case strings.HasSuffix(fileName, ".html"):
		contentType = "text/html"
	case strings.HasSuffix(fileName, ".png"):
		contentType = "image/png"
	case strings.HasSuffix(fileName, ".jpg"), strings.HasSuffix(fileName, ".jpeg"):
		contentType = "image/jpeg"
	case strings.HasSuffix(fileName, ".gif"):
		contentType = "image/gif"
	case strings.HasSuffix(fileName, ".svg"):
		contentType = "image/svg+xml"
	case strings.HasSuffix(fileName, ".ico"):
		contentType = "image/x-icon"
	}

	c.Data(http.StatusOK, contentType, content)
}

func HandleRoot(c *gin.Context) {
	c.Data(http.StatusOK, "application/json",
		[]byte(`{
			"message": "cruiseKube API Server",
			"endpoints": {
				"/clusters": "Lists all available clusters",
				"/clusters/{clusterId}/stats": "Serves stats file for specific cluster",
				"/clusters/{clusterId}/workload-analysis": "Generates workload analysis for specific cluster",
				"/clusters/{clusterId}/prometheus-proxy": "Proxies requests to cluster's Prometheus instance",
				"/clusters/{clusterId}/killswitch": "Deletes MutatingWebhookConfiguration objects and kills pods with resource differences (POST only)",
				"/clusters/{clusterId}/webhook/mutate": "Mutating admission webhook for pod resource adjustment",
				"/health": "Health check endpoint"
			}
		}`),
	)
}

func HandleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

func HandleListClusters(c *gin.Context) {
	ctx := c.Request.Context()
	mgr := c.MustGet("clusterManager").(cluster.Manager)

	logging.Infof(ctx, "Serving cluster list to %s", c.ClientIP())

	clusterIDs := mgr.GetClusterIDs()

	clusters := make([]map[string]any, len(clusterIDs))
	for i, clusterID := range clusterIDs {
		statsExists, err := storage.Stg.ClusterStatsExists(clusterID)
		if err != nil {
			logging.Errorf(ctx, "Failed to check if cluster stats exists for %s: %v", clusterID, err)
			continue
		}
		clusters[i] = map[string]any{
			"id":              clusterID,
			"name":            clusterID,
			"stats_available": statsExists,
		}
	}

	response := map[string]any{
		"clusters":     clusters,
		"count":        len(clusters),
		"cluster_mode": mgr.GetClusterMode(),
	}

	c.JSON(http.StatusOK, response)
}

func HandleClusterStats(c *gin.Context) {
	ctx := c.Request.Context()
	clusterID := c.Param("clusterID")

	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("cluster", clusterID))

	logging.Infof(ctx, "Serving stats for cluster %s to %s", clusterID, c.ClientIP())
	c.Header("Content-Type", "application/json")
	var statsResponse types.StatsResponse
	if err := storage.Stg.ReadClusterStats(clusterID, &statsResponse); err != nil {
		logging.Errorf(ctx, "Failed to read cluster stats for %s: %v", clusterID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to read cluster stats for %s: %v", clusterID, err),
		})
		return
	}
	c.JSON(http.StatusOK, statsResponse)
}

func HandlePrometheusProxy(c *gin.Context) {
	ctx := c.Request.Context()
	mgr := c.MustGet("clusterManager").(cluster.Manager)
	clusterID := c.Param("clusterID")

	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("cluster.id", clusterID))

	logging.Infof(ctx, "Proxying prometheus request for cluster %s from %s", clusterID, c.ClientIP())

	connInfo, err := mgr.GetPrometheusConnectionInfo(clusterID)
	if err != nil {
		logging.Errorf(ctx, "Failed to get prometheus connection info for cluster %s: %v", clusterID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get prometheus connection info for cluster %s: %v", clusterID, err),
		})
		return
	}

	targetURL, err := url.Parse(connInfo.URL)
	if err != nil {
		logging.Infof(ctx, "Failed to parse prometheus URL for cluster %s: %v", clusterID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to parse prometheus URL for cluster %s: %v", clusterID, err),
		})
		return
	}

	targetURL.Path = strings.Join([]string{targetURL.Path, strings.TrimPrefix(c.Request.URL.Path, fmt.Sprintf("/api/v1/clusters/%s/prometheus-proxy", clusterID))}, "")
	targetURL.RawQuery = c.Request.URL.RawQuery

	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL.String(), c.Request.Body)
	if err != nil {
		logging.Infof(ctx, "Failed to create proxy request for cluster %s: %v", clusterID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create proxy request for cluster %s: %v", clusterID, err),
		})
		return
	}

	for header, values := range c.Request.Header {
		for _, value := range values {
			proxyReq.Header.Add(header, value)
		}
	}

	proxyReq.Header.Set("Authorization", "Bearer "+connInfo.BearerToken)

	client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	resp, err := client.Do(proxyReq)
	if err != nil {
		logging.Infof(ctx, "Failed to proxy request to prometheus for cluster %s: %v", clusterID, err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("Failed to proxy request to prometheus for cluster %s: %v", clusterID, err),
		})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for header, values := range resp.Header {
		for _, value := range values {
			c.Header(header, value)
		}
	}

	c.Status(resp.StatusCode)
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		logging.Infof(ctx, "Failed to copy response body for cluster %s: %v", clusterID, err)
	}
}
