package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"github.com/truefoundry/autopilot-oss/pkg/logging"
)

// LoadWithViper loads configuration using Viper from a single config file.
// Overridden by env vars (prefix AUTOPILOT_) and flags bound by caller.
func LoadWithViper(ctx context.Context, configFilePath string) (*Config, error) {
	return LoadWithViperInstance(ctx, viper.New(), configFilePath)
}

// LoadWithViperInstance loads configuration using a provided Viper instance (for flag binding).
func LoadWithViperInstance(ctx context.Context, v *viper.Viper, configFilePath string) (*Config, error) {

	// Set defaults matching the new structure
	v.SetDefault("controllerMode", string(ClusterModeInCluster))
	v.SetDefault("executionMode", string(ExecutionModeBoth))
	v.SetDefault("dependencies.local.kubeconfigPath", "")
	v.SetDefault("dependencies.local.prometheusURL", "")
	v.SetDefault("controller.tasks.applyRecommendation.enabled", true)
	v.SetDefault("controller.tasks.applyRecommendation.schedule", "5m")
	v.SetDefault("controller.tasks.applyRecommendation.nodeStatsURL.host", "localhost:8080")
	v.SetDefault("controller.tasks.applyRecommendation.dryRun", true)
	v.SetDefault("controller.tasks.applyRecommendation.overridesURL.host", "localhost:8080")
	v.SetDefault("recommendationSettings.maxConcurrentQueries", 5)
	v.SetDefault("server.port", "8080")
	v.SetDefault("webhook.port", "8443")
	v.SetDefault("webhook.certsDir", "/certs")
	v.SetDefault("db.filePath", "autopilot.db")
	v.SetDefault("telemetry.enabled", false)
	v.SetDefault("telemetry.traceRatio", 0.1)

	v.SetConfigType("yaml")
	v.SetConfigFile(configFilePath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configFilePath, err)
	}

	v.SetEnvPrefix("AUTOPILOT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	switch c.ExecutionMode {
	case ExecutionModeWebhook:
		return c.ValidateWebhookExecutionMode()
	case ExecutionModeController:
		return c.ValidateControllerExecutionMode()
	case ExecutionModeBoth:
		if err := c.ValidateWebhookExecutionMode(); err != nil {
			return err
		}
		if err := c.ValidateControllerExecutionMode(); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("invalid execution-mode: %s (expected controller|webhook|both)", c.ExecutionMode)
	}
}

func (c *Config) ValidateWebhookExecutionMode() error {
	if c.Webhook.Port == "" || c.Webhook.StatsURL.Host == "" || c.Webhook.CertsDir == "" {
		return fmt.Errorf("webhook.port, webhook.statsURL.host, and webhook.certsDir are required for webhook execution mode")
	}
	return nil
}

func (c *Config) ValidateControllerExecutionMode() error {
	controllerMode := strings.TrimSpace(string(c.ControllerMode))
	switch controllerMode {
	case string(ClusterModeLocal):
		if strings.TrimSpace(c.Dependencies.Local.PrometheusURL) == "" {
			return fmt.Errorf("dependencies.local.prometheusURL is required in local mode")
		}
	case string(ClusterModeInCluster):
		if strings.TrimSpace(c.Dependencies.InCluster.PrometheusURL) == "" {
			return fmt.Errorf("dependencies.inCluster.prometheusURL is required in inCluster mode")
		}
	default:
		logging.Errorf(context.Background(), "invalid controller-mode: %s (expected local|inCluster)", controllerMode)
		return nil
	}

	return nil
}
