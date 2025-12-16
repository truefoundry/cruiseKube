package middleware

import (
	"fmt"
	"net/http"

	"github.com/truefoundry/cruisekube/pkg/cluster"
	"github.com/truefoundry/cruisekube/pkg/config"
	"github.com/truefoundry/cruisekube/pkg/contextutils"
	"github.com/truefoundry/cruisekube/pkg/logging"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func Dependencies(mgr cluster.Manager, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("clusterManager", mgr)
		c.Set("appConfig", cfg)
		c.Next()
	}
}

func AuthWebhook() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func AuthAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func CorsMiddleware() gin.HandlerFunc {
	cfg := cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization", "X-Requested-With"},
		AllowCredentials: true,
	}
	return cors.New(cfg)
}

func RequestContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		if fullPath := c.FullPath(); fullPath != "" {
			ctx = contextutils.WithAPI(ctx, fullPath)
		}

		if clusterID := c.Param("clusterID"); clusterID != "" {
			ctx = contextutils.WithCluster(ctx, clusterID)
		}

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func Logger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		ctx := param.Request.Context()
		logging.Infof(ctx, "HTTP %s %s - %d %dbytes %s",
			param.Method,
			param.Path,
			param.StatusCode,
			param.BodySize,
			param.Latency,
		)
		return ""
	})
}

func EnsureClusterExists() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Param("clusterID") == cluster.SingleClusterID {
			c.Next()
			return
		}

		mgr := c.MustGet("clusterManager").(cluster.Manager)
		clusterID := c.Param("clusterID")

		if clusterID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cluster endpoint format"})
			c.Abort()
			return
		}

		if _, err := mgr.GetClusterClients(clusterID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Cluster %s not found", clusterID)})
			c.Abort()
			return
		}

		c.Next()
	}
}

func Common(mgr cluster.Manager, cfg *config.Config) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		Logger(),
		gin.Recovery(),
		CorsMiddleware(),
		Dependencies(mgr, cfg),
		RequestContext(),
	}
}
