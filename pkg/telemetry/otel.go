package telemetry

import (
	"context"
	"os"
	"time"

	"github.com/truefoundry/cruiseKube/pkg/config"

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
		return nil, err
	}

	err = os.Setenv("OTEL_EXPORTER_OTLP_HEADERS", cfg.ExporterOTLPHeaders)
	if err != nil {
		return nil, err
	}

	err = os.Setenv("OTEL_SERVICE_NAME", cfg.ServiceName)
	if err != nil {
		return nil, err
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	sampler := trace.ParentBased(trace.TraceIDRatioBased(cfg.TraceRatio))

	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter, trace.WithBatchTimeout(2*time.Second)),
		trace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tracerProvider)

	return tracerProvider.Shutdown, nil
}
