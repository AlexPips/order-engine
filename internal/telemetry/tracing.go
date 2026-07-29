package telemetry

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

const otelSchemaURL = "https://opentelemetry.io/schemas/1.31.0"

// InitTracerProvider creates and configures an OTel TracerProvider with OTLP
// gRPC export. Caller must call tp.Shutdown(ctx) on graceful shutdown.
func InitTracerProvider(ctx context.Context, serviceName, serviceVersion string) (*trace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint("otel-collector:4317"),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			otelSchemaURL,
			attribute.String("service.name", serviceName),
			attribute.String("service.version", serviceVersion),
			attribute.String("telemetry.sdk.language", "go"),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter,
			trace.WithBatchTimeout(5*time.Second),
		),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}

// ShutdownTracerProvider flushes remaining spans and shuts down the provider.
func ShutdownTracerProvider(ctx context.Context, tp *trace.TracerProvider) {
	if tp == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := tp.Shutdown(ctx); err != nil {
		slog.Error("failed to shut down tracer provider", "error", err)
	}
}
