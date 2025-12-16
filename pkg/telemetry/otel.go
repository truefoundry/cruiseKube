package telemetry

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/truefoundry/cruisekube/pkg/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
)

// Init bootstraps the OpenTelemetry pipeline
// If it does not return an error, make sure to call shutdown for proper cleanup.
func Init(ctx context.Context, cfg config.TelemetryConfig) (func(context.Context) error, error) {
	err := os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", cfg.ExporterOTLPEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to set env var OTEL_EXPORTER_OTLP_ENDPOINT: %w", err)
	}

	err = os.Setenv("OTEL_EXPORTER_OTLP_HEADERS", cfg.ExporterOTLPHeaders)
	if err != nil {
		return nil, fmt.Errorf("failed to set env var OTEL_EXPORTER_OTLP_HEADERS: %w", err)
	}

	err = os.Setenv("OTEL_SERVICE_NAME", cfg.ServiceName)
	if err != nil {
		return nil, fmt.Errorf("failed to set env var OTEL_SERVICE_NAME: %w", err)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create otlp exporter: %w", err)
	}

	sampler := trace.ParentBased(trace.TraceIDRatioBased(cfg.TraceRatio))

	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter, trace.WithBatchTimeout(2*time.Second)),
		trace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tracerProvider)

	return tracerProvider.Shutdown, nil
}
