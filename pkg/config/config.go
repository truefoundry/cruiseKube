package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

const (
	ApplyRecommendationKey     = "applyrecommendation"
	FetchMetricsKey            = "fetchmetrics"
	CreateStatsKey             = "createstats"
	ModifyEqualCPUResourcesKey = "modifyequalcpuresources"
	NodeLoadMonitoringKey      = "nodeloadmonitoring"
)

type Config struct {
	ControllerMode         ControllerMode         `yaml:"controllerMode" mapstructure:"controllerMode"`
	Dependencies           Dependencies           `yaml:"dependencies" mapstructure:"dependencies"`
	ExecutionMode          ExecutionMode          `yaml:"executionMode" mapstructure:"executionMode"`
	Controller             ControllerConfig       `yaml:"controller" mapstructure:"controller"`
	Server                 ServerConfig           `yaml:"server" mapstructure:"server"`
	Webhook                WebhookConfig          `yaml:"webhook" mapstructure:"webhook"`
	DB                     DatabaseConfig         `yaml:"db" mapstructure:"db"`
	RecommendationSettings RecommendationSettings `yaml:"recommendationSettings" mapstructure:"recommendationSettings"`
	Telemetry              TelemetryConfig        `yaml:"telemetry" mapstructure:"telemetry"`
	Metrics                MetricsConfig          `yaml:"metrics" mapstructure:"metrics"`
	Custom                 map[string]interface{} `yaml:",inline" mapstructure:",remain"`
}

func (c *Config) GetTaskConfig(taskName string) *TaskConfig {
	return c.Controller.Tasks[taskName]
}

type Dependencies struct {
	Local     LocalDeps     `yaml:"local" mapstructure:"local"`
	InCluster InClusterDeps `yaml:"inCluster" mapstructure:"inCluster"`
}

type LocalDeps struct {
	KubeconfigPath string `yaml:"kubeconfigPath" mapstructure:"kubeconfigPath"`
	PrometheusURL  string `yaml:"prometheusURL" mapstructure:"prometheusURL"`
}

type InClusterDeps struct {
	PrometheusURL string `yaml:"prometheusURL" mapstructure:"prometheusURL"`
}

type ControllerConfig struct {
	TargetNamespace         string                 `yaml:"targetNamespace,omitempty" mapstructure:"targetNamespace"`
	TargetClusterID         string                 `yaml:"targetClusterID,omitempty" mapstructure:"targetClusterID"`
	WriteAuthorizedClusters []string               `yaml:"writeAuthorizedClusters" mapstructure:"writeAuthorizedClusters"`
	Tasks                   map[string]*TaskConfig `yaml:"tasks" mapstructure:"tasks"`
}

type URLConfig struct {
	Host            string `yaml:"host" mapstructure:"host"`
	TfyClusterToken string `yaml:"tfyClusterToken" mapstructure:"tfyClusterToken"`
}

type ServerConfig struct {
	Port      string          `yaml:"port" mapstructure:"port"`
	BasicAuth BasicAuthConfig `yaml:"basicAuth" mapstructure:"basicAuth"`
}

type BasicAuthConfig struct {
	Username string `yaml:"username" mapstructure:"username"`
	Password string `yaml:"password" mapstructure:"password"`
}

type WebhookConfig struct {
	Port     string    `yaml:"port" mapstructure:"port"`
	CertsDir string    `yaml:"certsDir" mapstructure:"certsDir"`
	DryRun   bool      `yaml:"dryRun" mapstructure:"dryRun"`
	StatsURL URLConfig `yaml:"statsURL" mapstructure:"statsURL"`
}

type DatabaseConfig struct {
	FilePath string `yaml:"filePath" mapstructure:"filePath"`
}

type RecommendationSettings struct {
	NewWorkloadThresholdHours  int      `yaml:"newWorkloadThresholdHours" mapstructure:"newWorkloadThresholdHours"`
	DisableMemoryApplication   bool     `yaml:"disableMemoryApplication" mapstructure:"disableMemoryApplication"`
	ApplyBlacklistedNamespaces []string `yaml:"applyBlacklistedNamespaces" mapstructure:"applyBlacklistedNamespaces"`
	MaxConcurrentQueries       int      `yaml:"maxConcurrentQueries" mapstructure:"maxConcurrentQueries"`
}

type TelemetryConfig struct {
	Enabled              bool    `yaml:"enabled" mapstructure:"enabled"`
	ExporterOTLPEndpoint string  `yaml:"exporterOTLPEndpoint" mapstructure:"exporterOTLPEndpoint"`
	ExporterOTLPHeaders  string  `yaml:"exporterOTLPHeaders" mapstructure:"exporterOTLPHeaders"`
	ServiceName          string  `yaml:"serviceName" mapstructure:"serviceName"`
	TraceRatio           float64 `yaml:"traceRatio" mapstructure:"traceRatio"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
	Port    string `yaml:"port" mapstructure:"port"`
}

type ControllerMode string

const (
	ClusterModeLocal     ControllerMode = "local"
	ClusterModeInCluster ControllerMode = "inCluster"
)

type ExecutionMode string

const (
	ExecutionModeController ExecutionMode = "controller"
	ExecutionModeWebhook    ExecutionMode = "webhook"
	ExecutionModeBoth       ExecutionMode = "both"
)

func LoadConfig(path string) (*Config, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(bytes, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) IsClusterWriteAuthorized(clusterID string) bool {
	if len(c.Controller.WriteAuthorizedClusters) == 0 {
		return true
	}

	for _, authorizedCluster := range c.Controller.WriteAuthorizedClusters {
		if authorizedCluster == clusterID {
			return true
		}
	}
	return false
}
