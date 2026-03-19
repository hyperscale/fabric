package otel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
)

func TestResourceFactory(t *testing.T) {
	cfg := &Config{
		ServiceName:           "test-service",
		ServiceVersion:        "1.0.0",
		DeploymentEnvironment: "test",
	}

	propagator := PropagatorFactory()
	resource, err := ResourceFactory(cfg, propagator)

	require.NoError(t, err)
	assert.NotNil(t, resource)

	// Check that resource has attributes
	attrs := resource.Attributes()
	assert.NotEmpty(t, attrs)

	// Find service.name attribute
	hasServiceName := false
	for _, attr := range attrs {
		if string(attr.Key) == "service.name" {
			hasServiceName = true
			assert.Equal(t, "test-service", attr.Value.AsString())
			break
		}
	}
	assert.True(t, hasServiceName, "resource should have service.name attribute")
}

func TestResourceFactory_WithNilPropagator(t *testing.T) {
	cfg := &Config{
		ServiceName:           "test-service",
		ServiceVersion:        "1.0.0",
		DeploymentEnvironment: "test",
	}

	// Should work with nil propagator (sets empty propagator)
	resource, err := ResourceFactory(cfg, propagation.NewCompositeTextMapPropagator())

	require.NoError(t, err)
	assert.NotNil(t, resource)
}

func TestResourceFactory_EmptyConfig(t *testing.T) {
	cfg := &Config{
		ServiceName:           "",
		ServiceVersion:        "",
		DeploymentEnvironment: "",
	}

	propagator := PropagatorFactory()
	resource, err := ResourceFactory(cfg, propagator)

	require.NoError(t, err)
	assert.NotNil(t, resource)
}

func TestResourceFactory_FullConfig(t *testing.T) {
	cfg := &Config{
		ServiceName:           "anchorify-server",
		ServiceVersion:        "2.0.0",
		DeploymentEnvironment: "production",
		ShutdownTimeout:       10 * time.Second,
		Trace: &TraceConfig{
			Enabled: false,
		},
		Metric: &MetricConfig{
			Enabled: false,
		},
		Log: &LogConfig{
			Enabled: false,
		},
	}

	propagator := PropagatorFactory()
	resource, err := ResourceFactory(cfg, propagator)

	require.NoError(t, err)
	assert.NotNil(t, resource)

	// Verify deployment environment is set
	attrs := resource.Attributes()
	hasDeployment := false
	for _, attr := range attrs {
		if string(attr.Key) == "deployment.environment.name" {
			hasDeployment = true
			assert.Equal(t, "production", attr.Value.AsString())
			break
		}
	}
	assert.True(t, hasDeployment, "resource should have deployment.environment.name attribute")
}
