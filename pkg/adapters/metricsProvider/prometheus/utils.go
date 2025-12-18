package prometheus

import (
	"fmt"
	"net/http"
	"time"
)

// BearerTokenRoundTripper implements http.RoundTripper to add Authorization header
type BearerTokenRoundTripper struct {
	BearerToken string
	Proxied     http.RoundTripper
}

func (rt *BearerTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+rt.BearerToken)
	}
	resp, err := rt.Proxied.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute round trip: %w", err)
	}
	return resp, nil
}

// GetPrometheusClientConfig returns optimized default configuration
func GetPrometheusClientConfig(prometheusURL string) *PrometheusClientConfig {
	return &PrometheusClientConfig{
		PrometheusURL:       prometheusURL,
		BearerToken:         "",
		QueryTimeout:        5 * time.Minute,  // Per-query timeout
		MaxConnsPerHost:     100,              // Increased connections per host
		MaxIdleConns:        50,               // Keep connections alive
		IdleConnTimeout:     90 * time.Second, // Keep connections alive longer
		ResponseTimeout:     5 * time.Minute,  // Overall response timeout
		DialTimeout:         10 * time.Second, // Connection establishment timeout
		KeepAlive:           30 * time.Second, // TCP keep-alive
		TLSHandshakeTimeout: 10 * time.Second, // TLS handshake timeout

		// For Provider
		MaxQueryRetries:      3,
		RetryBackoffBase:     5 * time.Second,
		MaxConcurrentQueries: 10,
	}
}
