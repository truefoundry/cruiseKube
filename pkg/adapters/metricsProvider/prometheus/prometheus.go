package prometheus

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/truefoundry/cruisekube/pkg/logging"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type PrometheusClientConfig struct {
	// For Client
	PrometheusURL       string
	BearerToken         string
	QueryTimeout        time.Duration
	MaxConnsPerHost     int
	MaxIdleConns        int
	IdleConnTimeout     time.Duration
	ResponseTimeout     time.Duration
	DialTimeout         time.Duration
	KeepAlive           time.Duration
	TLSHandshakeTimeout time.Duration

	// For Provider
	MaxQueryRetries      int
	RetryBackoffBase     time.Duration
	MaxConcurrentQueries int
}

type PrometheusProvider struct {
	client          v1.API
	config          *PrometheusClientConfig
	querySemaphores sync.Map
}

func NewPrometheusProvider(ctx context.Context, config *PrometheusClientConfig) (*PrometheusProvider, error) {
	// Create optimized HTTP transport
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   config.DialTimeout,
			KeepAlive: config.KeepAlive,
		}).DialContext,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: config.ResponseTimeout,
		DisableCompression:    false, // Enable compression for better performance
	}

	// Wrap transport with bearer token if provided
	var finalTransport http.RoundTripper = transport
	if config.BearerToken != "" {
		finalTransport = &BearerTokenRoundTripper{
			BearerToken: config.BearerToken,
			Proxied:     transport,
		}
	}

	// Create optimized HTTP client
	httpClient := &http.Client{
		Transport: otelhttp.NewTransport(finalTransport),
		Timeout:   config.ResponseTimeout,
	}

	// Create Prometheus API client
	apiClient, err := api.NewClient(api.Config{
		Address: config.PrometheusURL,
		Client:  httpClient,
	})
	if err != nil {
		logging.Error(ctx, "Failed to create Prometheus client", err)
		return nil, err
	}
	client := v1.NewAPI(apiClient)

	logging.Infof(ctx, "Prometheus client initialized with URL: %s", config.PrometheusURL)
	logging.Infof(ctx, "  - Query timeout: %v", config.QueryTimeout)
	logging.Infof(ctx, "  - Max connections per host: %d", config.MaxConnsPerHost)
	logging.Infof(ctx, "  - Max idle connections: %d", config.MaxIdleConns)
	logging.Infof(ctx, "  - Idle connection timeout: %v", config.IdleConnTimeout)

	return &PrometheusProvider{
		client: client,
		config: config,
	}, nil
}

func (p *PrometheusProvider) GetClient() v1.API {
	return p.client
}
