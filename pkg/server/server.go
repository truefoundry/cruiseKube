package server

import (
	"github.com/truefoundry/cruisekube/pkg/handlers"

	"github.com/truefoundry/cruisekube/pkg/cluster"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func SetupServerEngine(mgr cluster.Manager, authAPI gin.HandlerFunc, authWebhook gin.HandlerFunc, ensureClusterExists gin.HandlerFunc, middleware ...gin.HandlerFunc) *gin.Engine {
	r := gin.Default()
	r.Use(otelgin.Middleware("cruisekube-api"))
	r.Use(middleware...)

	r.GET("/health", handlers.HandleHealth)

	apiV1Group := r.Group("/api/v1")
	{
		apiV1Group.GET("/", handlers.HandleRoot)
		apiV1Group.GET("/clusters", authAPI, handlers.HandleListClusters)
	}

	clusterGroup := apiV1Group.Group("/clusters/:clusterID", authAPI, ensureClusterExists)
	{
		clusterGroup.GET("/stats", handlers.HandleClusterStats)
		clusterGroup.GET("/workload-analysis", handlers.WorkloadAnalysisHandlerForCluster)
		clusterGroup.GET("/recommendation-analysis", handlers.RecommendationAnalysisHandlerForCluster)
		clusterGroup.Any("/prometheus-proxy/*proxyPath", handlers.HandlePrometheusProxy)
		clusterGroup.GET("/prometheus-query", handlers.HandlePrometheusQuery)
		clusterGroup.GET("/prometheus-config", handlers.GetPrometheusConfigHandler)
		clusterGroup.POST("/killswitch", handlers.KillswitchHandler)
		clusterGroup.GET("/workloads", handlers.ListWorkloadsHandler)
		clusterGroup.GET("/workloads/:workloadID/overrides", handlers.GetWorkloadOverridesHandler)
		clusterGroup.POST("/workloads/:workloadID/overrides", handlers.UpdateWorkloadOverridesHandler)
		clusterGroup.POST("/tasks/:taskName/trigger", handlers.HandleTaskTrigger)
	}

	uiGroup := r.Group("/ui")
	uiGroup.Use(authAPI)
	{
		uiGroup.GET("/app.js", handlers.HandleUIStaticFiles)
		uiGroup.GET("/styles.css", handlers.HandleUIStaticFiles)
		uiGroup.GET("/ui.html", handlers.HandleUIStaticFiles)
		uiGroup.GET("", handlers.GeneralUIHandler)
	}

	webhookGroup := apiV1Group.Group("/webhook/clusters/:clusterID", authWebhook, ensureClusterExists)
	{
		webhookGroup.GET("/stats", handlers.HandleClusterStats)
		webhookGroup.GET("/workloads/:workloadID/overrides", handlers.GetWorkloadOverridesHandler)
	}

	return r
}

func SetupWebhookServerEngine(middleware ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(otelgin.Middleware("cruisekubeWebhook"))
	r.Use(middleware...)

	clusterGroup := r.Group("/clusters/:clusterID")
	{
		clusterGroup.POST("/webhook/mutate", handlers.MutateHandler)
	}

	return r
}

func SetupMetricsServerEngine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/health", handlers.HandleHealth)

	return r
}
