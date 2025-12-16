package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/truefoundry/cruiseKube/pkg/contextutils"

	"github.com/truefoundry/cruiseKube/pkg/logging"
	"github.com/truefoundry/cruiseKube/pkg/repository/storage"
	"github.com/truefoundry/cruiseKube/pkg/types"

	"github.com/gin-gonic/gin"
)

func getTemplateFilePath() string {
	// Try to find the template file relative to the current working directory
	paths := []string{
		"web/ui.html",
		"../web/ui.html",
		"../../web/ui.html",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// If not found, try relative to the executable
	executable, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(executable)
		webPath := filepath.Join(execDir, "web", "ui.html")
		if _, err := os.Stat(webPath); err == nil {
			return webPath
		}
	}

	// Default fallback
	return "web/ui.html"
}

func OverridesUIHandler(c *gin.Context) {
	ctx := c.Request.Context()
	clusterID := c.Param("clusterID")
	ctx = contextutils.WithAPI(ctx, c.Request.URL.Path)
	logging.Infof(ctx, "Serving overrides UI for cluster %s", clusterID)

	templatePath := getTemplateFilePath()
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		logging.Errorf(ctx, "Failed to parse template file %s: %v", templatePath, err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	data := struct {
		ClusterID string
	}{
		ClusterID: clusterID,
	}

	c.Header("Content-Type", "text/html")
	if err := tmpl.Execute(c.Writer, data); err != nil {
		logging.Errorf(ctx, "Failed to execute template: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
}

func GeneralUIHandler(c *gin.Context) {
	ctx := c.Request.Context()
	ctx = contextutils.WithAPI(ctx, c.Request.URL.Path)
	logging.Infof(ctx, "Serving general overrides UI")

	templatePath := getTemplateFilePath()
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		logging.Errorf(ctx, "Failed to parse template file %s: %v", templatePath, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to parse template file %s: %v", templatePath, err),
		})
		return
	}

	data := struct {
		ClusterID string
	}{
		ClusterID: "",
	}

	c.Header("Content-Type", "text/html")
	if err := tmpl.Execute(c.Writer, data); err != nil {
		logging.Errorf(ctx, "Failed to execute template: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to execute template: %v", err),
		})
		return
	}
}

func ListWorkloadsHandler(c *gin.Context) {
	ctx := c.Request.Context()
	clusterID := c.Param("clusterID")
	logging.Infof(ctx, "Listing workloads for cluster %s", clusterID)
	stats, err := storage.Stg.GetAllStatsForCluster(clusterID)
	if err != nil {
		logging.Errorf(ctx, "Failed to get stats for cluster %s: %v", clusterID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get workloads for cluster %s: %v", clusterID, err),
		})
		return
	}

	var workloads = make([]types.WorkloadOverrideInfo, 0)
	for _, stat := range stats {
		workloadColumnId := strings.ReplaceAll(stat.WorkloadIdentifier, "/", ":")
		overrides, err := storage.Stg.GetWorkloadOverrides(clusterID, workloadColumnId)
		if err != nil {
			logging.Errorf(ctx, "Failed to get overrides for workload %s: %v", stat.WorkloadIdentifier, err)
		}

		evictionRanking := stat.EvictionRanking
		enabled := true
		if overrides != nil {
			if overrides.EvictionRanking != nil {
				evictionRanking = *overrides.EvictionRanking
			}
			if overrides.Enabled != nil {
				enabled = *overrides.Enabled
			}
		}

		workloadExternalId := strings.ReplaceAll(stat.WorkloadIdentifier, "/", ":")
		workload := types.WorkloadOverrideInfo{
			WorkloadID:      workloadExternalId,
			Name:            stat.Name,
			Namespace:       stat.Namespace,
			Kind:            stat.Kind,
			EvictionRanking: evictionRanking,
			Enabled:         enabled,
		}
		workloads = append(workloads, workload)
	}

	c.Header("Content-Type", "application/json")
	if err := json.NewEncoder(c.Writer).Encode(workloads); err != nil {
		logging.Errorf(ctx, "Failed to encode workloads: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to encode workloads: %v", err),
		})
		return
	}
}
