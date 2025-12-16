package telemetry

import (
	stdctx "context"

	"github.com/truefoundry/cruisekube/pkg/contextutils"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// StartSpan starts an OTel span and copies contextual attributes like task, cluster, api onto the span.
func StartSpan(ctx stdctx.Context, tracerName, spanName string, kvs ...attribute.KeyValue) (stdctx.Context, oteltrace.Span) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, spanName)
	attrs := contextutils.GetAttributes(ctx)
	for k, v := range attrs {
		span.SetAttributes(attribute.String(k, v))
	}
	return ctx, span
}
