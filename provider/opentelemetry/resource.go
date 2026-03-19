package otel

import (
	"context"

	"github.com/euskadi31/wire"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

var OTelResourceSet = wire.NewSet(ResourceFactory)

func ResourceFactory(cfg *Config, prop propagation.TextMapPropagator) (*resource.Resource, error) {
	otel.SetTextMapPropagator(prop)

	// nolint:wrapcheck
	return resource.New(
		context.Background(),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithContainer(),
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithHost(),
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
			semconv.DeploymentEnvironmentName(cfg.DeploymentEnvironment),
		),
	)
}
