package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/truefoundry/cruisekube/pkg/adapters/database/sqlite"
	"github.com/truefoundry/cruisekube/pkg/adapters/kube"
	"github.com/truefoundry/cruisekube/pkg/adapters/metricsProvider/prometheus"
	"github.com/truefoundry/cruisekube/pkg/cluster"
	"github.com/truefoundry/cruisekube/pkg/config"
	"github.com/truefoundry/cruisekube/pkg/contextutils"
	"github.com/truefoundry/cruisekube/pkg/middleware"
	"github.com/truefoundry/cruisekube/pkg/repository/storage"
	"github.com/truefoundry/cruisekube/pkg/server"
	"github.com/truefoundry/cruisekube/pkg/task"
	"github.com/truefoundry/cruisekube/pkg/telemetry"

	"github.com/truefoundry/cruisekube/pkg/logging"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	_ "go.uber.org/automaxprocs"
)

var (
	configFilePath string
	v              = viper.New()
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "cruisekube",
		Short: "Kubernetes resource cruisekube",
		Run:   runcruisekube,
	}

	// Core flags
	rootCmd.PersistentFlags().StringVar(&configFilePath, "config-file-path", "config.yaml", "Path to configuration file")
	rootCmd.PersistentFlags().String("execution-mode", "", "Execution mode: controller|webhook|both")
	rootCmd.PersistentFlags().String("controller-mode", "", "Controller mode: local|inCluster")

	// Dependencies flags
	rootCmd.PersistentFlags().String("kubeconfig-path", "", "Path to kubeconfig file (local mode)")
	rootCmd.PersistentFlags().String("prometheus-url", "", "Prometheus URL")

	// Server/Webhook flags
	rootCmd.PersistentFlags().String("server-port", "", "Server port")
	rootCmd.PersistentFlags().String("webhook-port", "", "Webhook port")
	rootCmd.PersistentFlags().String("webhook-certs-dir", "", "Webhook certificates directory")
	rootCmd.PersistentFlags().String("webhook-stats-url-host", "", "Webhook stats URL host")

	// Database flag
	rootCmd.PersistentFlags().String("db-file-path", "", "Database file path")

	// Apply recommendation flag
	rootCmd.PersistentFlags().Bool("apply-recommendation-dry-run", true, "Apply recommendation dry run")

	// Bind flags to viper
	ctx := context.Background()
	if err := v.BindPFlag("controllerMode", rootCmd.PersistentFlags().Lookup("controller-mode")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("executionMode", rootCmd.PersistentFlags().Lookup("execution-mode")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("dependencies.local.kubeconfigPath", rootCmd.PersistentFlags().Lookup("kubeconfig-path")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("dependencies.local.prometheusURL", rootCmd.PersistentFlags().Lookup("prometheus-url")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("dependencies.inCluster.prometheusURL", rootCmd.PersistentFlags().Lookup("prometheus-url")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("server.port", rootCmd.PersistentFlags().Lookup("server-port")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("webhook.port", rootCmd.PersistentFlags().Lookup("webhook-port")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("webhook.certsDir", rootCmd.PersistentFlags().Lookup("webhook-certs-dir")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("webhook.statsURL.host", rootCmd.PersistentFlags().Lookup("webhook-stats-url-host")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("controller.tasks.applyRecommendation.dryRun", rootCmd.PersistentFlags().Lookup("apply-recommendation-dry-run")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}
	if err := v.BindPFlag("db.filePath", rootCmd.PersistentFlags().Lookup("db-file-path")); err != nil {
		logging.Fatalf(ctx, "Failed to bind flag: %v", err)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runcruisekube(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	cfg, err := config.LoadWithViperInstance(ctx, v, configFilePath)
	if err != nil {
		logging.Fatalf(ctx, "Failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		logging.Fatalf(ctx, "Invalid configuration: %v", err)
	}
	logging.Infof(ctx, "Configuration loaded: controllerMode=%s executionMode=%s", cfg.ControllerMode, cfg.ExecutionMode)

	if cfg.Telemetry.Enabled {
		shutdown, err := telemetry.Init(ctx, cfg.Telemetry)
		if err != nil {
			logging.Fatalf(ctx, "Failed to initialize telemetry: %v", err)
		}

		defer func() {
			if err := shutdown(context.Background()); err != nil {
				logging.Errorf(ctx, "Failed to shutdown telemetry: %v", err)
			}
		}()
	}

	if cfg.Metrics.Enabled {
		metricsEngine := server.SetupMetricsServerEngine()
		metricsPort := cfg.Metrics.Port
		go func() {
			logging.Infof(ctx, "Starting metrics server on :%s", metricsPort)
			if err := metricsEngine.Run(":" + metricsPort); err != nil {
				logging.Fatalf(ctx, "Metrics server failed: %v", err)
			}
		}()
	}

	if cfg.ExecutionMode == config.ExecutionModeWebhook || cfg.ExecutionMode == config.ExecutionModeBoth {
		setupWebhookMode(ctx, cfg)
	}

	if cfg.ExecutionMode == config.ExecutionModeController || cfg.ExecutionMode == config.ExecutionModeBoth {
		setupControllerMode(ctx, cfg)
	}

	if cfg.ExecutionMode == config.ExecutionModeWebhook {
		blockForever()
	}
}

func setupControllerMode(ctx context.Context, cfg *config.Config) {
	var clusterManager cluster.Manager
	var promClient *prometheus.PrometheusProvider

	////////
	// Storage Repo
	////////
	SQLiteAdapter, err := sqlite.NewSQLiteAdapter(cfg.DB.FilePath)
	if err != nil {
		logging.Fatalf(ctx, "Failed to initialize database: %v", err)
	}
	logging.Infof(ctx, "SQLite Adapter initialized")

	storageRepo, err := storage.NewStorageRepo(SQLiteAdapter)
	if err != nil {
		logging.Fatalf(ctx, "Failed to initialize storage: %v", err)
	}
	logging.Infof(ctx, "Storage Repo initialized")
	storage.Stg = storageRepo

	////////
	// Initialize Modes
	////////
	switch cfg.ControllerMode {
	case config.ClusterModeLocal:
		logging.Infof(ctx, "Local cluster mode")
		ctx = contextutils.WithCluster(ctx, "local")
		kubeconfigPath := cfg.Dependencies.Local.KubeconfigPath
		if kubeconfigPath == "" {
			if home := homeDir(); home != "" {
				kubeconfigPath = filepath.Join(home, ".kube", "config")
			}
		}
		kubeClient, err := kube.NewKubeClient(contextutils.WithCluster(ctx, "local"), kubeconfigPath)
		if err != nil {
			logging.Fatalf(ctx, "Failed to create kube client: %v", err)
		}

		dynamicClient, err := kube.NewDynamicClient(contextutils.WithCluster(ctx, "local"), kubeconfigPath)
		if err != nil {
			logging.Fatalf(ctx, "Failed to create dynamic client: %v", err)
		}

		promURL := cfg.Dependencies.Local.PrometheusURL
		defaultPromConfig := prometheus.GetPrometheusClientConfig(promURL)
		promClient, err = prometheus.NewPrometheusProvider(contextutils.WithCluster(ctx, "local"), defaultPromConfig)
		if err != nil {
			logging.Fatalf(ctx, "Failed to create prometheus client: %v", err)
		}

		clusterManager = cluster.NewSingleClusterManager(ctx, kubeClient, dynamicClient, promClient.GetClient())
	case config.ClusterModeInCluster:
		logging.Infof(ctx, "In-cluster mode")
		ctx = contextutils.WithCluster(ctx, "in-cluster")
		kubeClient, err := kube.NewKubeClient(contextutils.WithCluster(ctx, "in-cluster"), "")
		if err != nil {
			logging.Fatalf(ctx, "Failed to create kube client: %v", err)
		}

		dynamicClient, err := kube.NewDynamicClient(contextutils.WithCluster(ctx, "in-cluster"), "")
		if err != nil {
			logging.Fatalf(ctx, "Failed to create dynamic client: %v", err)
		}

		defaultPromConfig := prometheus.GetPrometheusClientConfig(cfg.Dependencies.InCluster.PrometheusURL)
		promClient, err = prometheus.NewPrometheusProvider(contextutils.WithCluster(ctx, "in-cluster"), defaultPromConfig)
		if err != nil {
			logging.Fatalf(ctx, "Failed to create prometheus client: %v", err)
		}

		clusterManager = cluster.NewSingleClusterManager(ctx, kubeClient, dynamicClient, promClient.GetClient())
	default:
		logging.Fatalf(ctx, "Invalid controller mode: %s", cfg.ControllerMode)
	}

	////////
	// Storage Server
	////////
	engine := server.SetupServerEngine(
		clusterManager,
		middleware.AuthAPI(),
		middleware.AuthWebhook(),
		middleware.EnsureClusterExists(),
		middleware.Common(clusterManager, cfg)...)

	serverPort := cfg.Server.Port
	go func() {
		if err := engine.Run(":" + serverPort); err != nil {
			logging.Fatalf(ctx, "HTTP server failed: %v", err)
		}
	}()

	////////
	// Add tasks to cluster manager
	////////
	for ID, cluster := range clusterManager.GetAllClusters() {
		createStatsTaskConfig := cfg.GetTaskConfig(config.CreateStatsKey)
		clusterManager.AddTask(task.NewCreateStatsTask(
			ctx,
			cluster.KubeClient,
			cluster.DynamicClient,
			promClient,
			storageRepo,
			&task.CreateStatsTaskConfig{
				Name:                       ID + "_" + config.CreateStatsKey,
				Enabled:                    createStatsTaskConfig.Enabled,
				Schedule:                   createStatsTaskConfig.Schedule,
				ClusterID:                  ID,
				TargetClusterID:            cfg.Controller.TargetClusterID,
				TargetNamespace:            cfg.Controller.TargetNamespace,
				RecentStatsLookbackMinutes: 1,
				TimeStepSize:               5 * time.Minute,
				MLLookbackWindow:           1 * time.Hour,
			},
			createStatsTaskConfig,
		))

		modifyEqualCPUResourcesTaskConfig := cfg.GetTaskConfig(config.ModifyEqualCPUResourcesKey)
		clusterManager.AddTask(task.NewModifyEqualCPUResourcesTask(
			ctx,
			cluster.KubeClient,
			cluster.DynamicClient,
			promClient,
			&task.ModifyEqualCPUResourcesTaskConfig{
				Name:                     ID + "_" + config.ModifyEqualCPUResourcesKey,
				Enabled:                  modifyEqualCPUResourcesTaskConfig.Enabled,
				Schedule:                 modifyEqualCPUResourcesTaskConfig.Schedule,
				ClusterID:                ID,
				IsClusterWriteAuthorized: cfg.IsClusterWriteAuthorized(ID),
			},
		))

		applyRecommendationTaskConfig := cfg.GetTaskConfig(config.ApplyRecommendationKey)
		clusterManager.AddTask(task.NewApplyRecommendationTask(
			ctx,
			cluster.KubeClient,
			cluster.DynamicClient,
			promClient,
			&task.ApplyRecommendationTaskConfig{
				Name:                     ID + "_" + config.ApplyRecommendationKey,
				Enabled:                  applyRecommendationTaskConfig.Enabled,
				Schedule:                 applyRecommendationTaskConfig.Schedule,
				ClusterID:                ID,
				TargetClusterID:          cfg.Controller.TargetClusterID,
				TargetNamespace:          cfg.Controller.TargetNamespace,
				IsClusterWriteAuthorized: cfg.IsClusterWriteAuthorized(ID),
				BasicAuth:                cfg.Server.BasicAuth,
				RecommendationSettings:   cfg.RecommendationSettings,
			},
			applyRecommendationTaskConfig,
		))

		fetchMetricsTaskConfig := cfg.GetTaskConfig(config.FetchMetricsKey)
		clusterManager.AddTask(task.NewFetchMetricsTask(
			ctx,
			cluster.KubeClient,
			cluster.DynamicClient,
			promClient,
			storageRepo,
			&task.FetchMetricsTaskConfig{
				Name:      ID + "_" + config.FetchMetricsKey,
				Enabled:   fetchMetricsTaskConfig.Enabled,
				Schedule:  fetchMetricsTaskConfig.Schedule,
				ClusterID: ID,
			},
		))

		nodeLoadMonitoringTaskConfig := cfg.GetTaskConfig(config.NodeLoadMonitoringKey)
		clusterManager.AddTask(task.NewNodeLoadMonitoringTask(
			ctx,
			cluster.KubeClient,
			cluster.DynamicClient,
			promClient,
			&task.NodeLoadMonitoringTaskConfig{
				Name:                     ID + "_" + config.NodeLoadMonitoringKey,
				Enabled:                  nodeLoadMonitoringTaskConfig.Enabled,
				Schedule:                 nodeLoadMonitoringTaskConfig.Schedule,
				ClusterID:                ID,
				IsClusterWriteAuthorized: cfg.IsClusterWriteAuthorized(ID),
			},
		))
	}

	if err := clusterManager.ScheduleAllTasks(); err != nil {
		logging.Fatalf(ctx, "Failed to schedule tasks: %v", err)
	}
	if err := clusterManager.StartTasks(); err != nil {
		logging.Fatalf(ctx, "Failed to start tasks: %v", err)
	}
}

func setupWebhookMode(ctx context.Context, cfg *config.Config) {
	webhookPort := cfg.Webhook.Port
	certDir := cfg.Webhook.CertsDir
	webhookEngine := server.SetupWebhookServerEngine(middleware.Common(nil, cfg)...)
	go func() {
		if err := webhookEngine.RunTLS(":"+webhookPort, certDir+"/tls.crt", certDir+"/tls.key"); err != nil {
			logging.Fatalf(ctx, "HTTPS server failed: %v", err)
		}
	}()
}

func blockForever() {
	select {}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}
