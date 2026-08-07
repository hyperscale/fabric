package otel

import (
	"context"

	"github.com/euskadi31/wire"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	// Must match the semconv version used by go.opentelemetry.io/otel/sdk/resource,
	// otherwise resource.WithTelemetrySDK() and WithSchemaURL() below disagree and
	// resource.New fails with "conflicting Schema URL".
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
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
			semconv.DeploymentEnvironmentNameKey.String(cfg.DeploymentEnvironment),
		),
	)
}
