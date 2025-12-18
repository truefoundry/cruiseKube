package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/truefoundry/cruisekube/pkg/contextutils"
	"github.com/truefoundry/cruisekube/pkg/logging"
	"github.com/truefoundry/cruisekube/pkg/metrics"
	"github.com/truefoundry/cruisekube/pkg/types"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// TODO: This client should be getting generated and not hardcoded
type RecommenderServiceClient struct {
	host         string
	httpClient   *http.Client
	username     string
	password     string
	clusterToken string
}

type ClientConfig struct {
	Host         string
	Username     string
	Password     string
	ClusterToken string
	Timeout      time.Duration
}

type HealthResponse struct {
	Status string `json:"status"`
}

type RootResponse struct {
	Message   string            `json:"message"`
	Endpoints map[string]string `json:"endpoints"`
}

type ClustersResponse struct {
	Clusters    []ClusterInfo `json:"clusters"`
	Count       int           `json:"count"`
	ClusterMode string        `json:"cluster_mode"`
}

type ClusterInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	StatsAvailable bool   `json:"stats_available"`
}

type PrometheusProxyRequest struct {
	Method      string
	ProxyPath   string
	QueryParams url.Values
	Headers     map[string]string
	Body        io.Reader
}

func NewRecommenderServiceClient(config ClientConfig) *RecommenderServiceClient {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &RecommenderServiceClient{
		host: strings.TrimSuffix(config.Host, "/"),
		httpClient: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
			Timeout:   timeout,
		},
		username:     config.Username,
		password:     config.Password,
		clusterToken: config.ClusterToken,
	}
}

func NewRecommenderServiceClientWithBasicAuth(host, username, password string) *RecommenderServiceClient {
	return NewRecommenderServiceClient(ClientConfig{
		Host:     host,
		Username: username,
		Password: password,
	})
}

func NewRecommenderServiceClientWithClusterToken(host, clusterToken string) *RecommenderServiceClient {
	return NewRecommenderServiceClient(ClientConfig{
		Host:         host,
		ClusterToken: clusterToken,
	})
}

func (c *RecommenderServiceClient) makeRequest(ctx context.Context, method, endpoint string, body interface{}, result interface{}) error {
	var err error
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
		}
		if clusterId, ok := contextutils.GetCluster(ctx); ok {
			metrics.WebhookControllerAPICallsTotal.WithLabelValues(clusterId, status, endpoint).Inc()
		}
	}()

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	fullURL := c.host + endpoint
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.clusterToken != "" {
		logging.Infof(ctx, "Setting cluster token")
		req.Header.Set("x-cluster-token", c.clusterToken)
	} else if c.username != "" && c.password != "" {
		logging.Infof(ctx, "Setting basic auth")
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

func (c *RecommenderServiceClient) makeRawRequest(ctx context.Context, method, endpoint string, headers map[string]string, body io.Reader) (*http.Response, error) {
	fullURL := c.host + endpoint
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	if c.clusterToken != "" {
		req.Header.Set("x-cluster-token", c.clusterToken)
	} else if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	return resp, nil
}

func (c *RecommenderServiceClient) Health(ctx context.Context) (*HealthResponse, error) {
	var result HealthResponse
	err := c.makeRequest(ctx, "GET", "/health", nil, &result)
	return &result, err
}

func (c *RecommenderServiceClient) Root(ctx context.Context) (*RootResponse, error) {
	var result RootResponse
	err := c.makeRequest(ctx, "GET", "/api/v1/", nil, &result)
	return &result, err
}

func (c *RecommenderServiceClient) ListClusters(ctx context.Context) (*ClustersResponse, error) {
	var result ClustersResponse
	err := c.makeRequest(ctx, "GET", "/api/v1/clusters", nil, &result)
	return &result, err
}

func (c *RecommenderServiceClient) GetClusterStats(ctx context.Context, clusterID string) (*types.StatsResponse, error) {
	var result types.StatsResponse
	endpoint := fmt.Sprintf("/api/v1/clusters/%s/stats", clusterID)
	err := c.makeRequest(ctx, "GET", endpoint, nil, &result)
	return &result, err
}

func (c *RecommenderServiceClient) GetWorkloadAnalysis(ctx context.Context, clusterID string) ([]types.WorkloadAnalysisItem, error) {
	var result []types.WorkloadAnalysisItem
	endpoint := fmt.Sprintf("/api/v1/clusters/%s/workload-analysis", clusterID)
	err := c.makeRequest(ctx, "GET", endpoint, nil, &result)
	return result, err
}

func (c *RecommenderServiceClient) GetRecommendationAnalysis(ctx context.Context, clusterID string) (interface{}, error) {
	var result interface{}
	endpoint := fmt.Sprintf("/api/v1/clusters/%s/recommendation-analysis", clusterID)
	err := c.makeRequest(ctx, "GET", endpoint, nil, &result)
	return result, err
}

func (c *RecommenderServiceClient) PrometheusProxy(ctx context.Context, clusterID string, proxyReq PrometheusProxyRequest) (*http.Response, error) {
	endpoint := fmt.Sprintf("/api/v1/clusters/%s/prometheus-proxy/%s", clusterID, strings.TrimPrefix(proxyReq.ProxyPath, "/"))

	if len(proxyReq.QueryParams) > 0 {
		endpoint += "?" + proxyReq.QueryParams.Encode()
	}

	return c.makeRawRequest(ctx, proxyReq.Method, endpoint, proxyReq.Headers, proxyReq.Body)
}

func (c *RecommenderServiceClient) Killswitch(ctx context.Context, clusterID string, dryRun bool) (*types.KillswitchResponse, error) {
	var result types.KillswitchResponse
	endpoint := fmt.Sprintf("/api/v1/clusters/%s/killswitch", clusterID)
	if dryRun {
		endpoint += "?dry_run=true"
	}
	err := c.makeRequest(ctx, "POST", endpoint, nil, &result)
	return &result, err
}

func (c *RecommenderServiceClient) ListWorkloads(ctx context.Context, clusterID string) ([]types.WorkloadOverrideInfo, error) {
	var result []types.WorkloadOverrideInfo
	endpoint := fmt.Sprintf("/api/v1/clusters/%s/workloads", clusterID)
	err := c.makeRequest(ctx, "GET", endpoint, nil, &result)
	return result, err
}

func (c *RecommenderServiceClient) GetWorkloadOverrides(ctx context.Context, clusterID, workloadID string) (*types.Overrides, error) {
	var result types.Overrides
	endpoint := fmt.Sprintf("/api/v1/clusters/%s/workloads/%s/overrides", clusterID, workloadID)
	err := c.makeRequest(ctx, "GET", endpoint, nil, &result)
	return &result, err
}

func (c *RecommenderServiceClient) UpdateWorkloadOverrides(ctx context.Context, clusterID, workloadID string, overrides *types.Overrides) error {
	endpoint := fmt.Sprintf("/api/v1/clusters/%s/workloads/%s/overrides", clusterID, workloadID)
	return c.makeRequest(ctx, "POST", endpoint, overrides, nil)
}

func (c *RecommenderServiceClient) WebhookGetClusterStats(ctx context.Context, clusterID string) (*types.StatsResponse, error) {
	var result types.StatsResponse
	endpoint := fmt.Sprintf("/api/v1/webhook/clusters/%s/stats", clusterID)
	err := c.makeRequest(ctx, "GET", endpoint, nil, &result)
	return &result, err
}

func (c *RecommenderServiceClient) WebhookGetWorkloadOverrides(ctx context.Context, clusterID, workloadID string) (*types.Overrides, error) {
	var result types.Overrides
	endpoint := fmt.Sprintf("/api/v1/webhook/clusters/%s/workloads/%s/overrides", clusterID, workloadID)
	err := c.makeRequest(ctx, "GET", endpoint, nil, &result)
	return &result, err
}

func (c *RecommenderServiceClient) SetHost(host string) {
	c.host = strings.TrimSuffix(host, "/")
}

func (c *RecommenderServiceClient) SetAuth(username, password string) {
	c.username = username
	c.password = password
	c.clusterToken = ""
}

func (c *RecommenderServiceClient) SetClusterToken(token string) {
	c.clusterToken = token
	c.username = ""
	c.password = ""
}

func (c *RecommenderServiceClient) GetHost() string {
	return c.host
}
