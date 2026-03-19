package otel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/resource"
)

func TestMetricFactory_Disabled(t *testing.T) {
	cfg := &Config{
		Metric: &MetricConfig{
			Enabled: false,
		},
	}

	mp, err := MetricFactory(cfg, nil)
	require.NoError(t, err)
	assert.Nil(t, mp)
}

func TestMetricFactory_StdoutExporter(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Metric: &MetricConfig{
			Enabled:  true,
			Interval: 10 * time.Second,
			Exporter: ExporterTypeStdout,
			Stdout:   nil,
			GRPC: &GRPCConfig{
				Timeout: 5 * time.Second,
			},
		},
	}

	mp, err := MetricFactory(cfg, res)
	require.NoError(t, err)
	assert.NotNil(t, mp)

	// Cleanup
	err = mp.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestMetricFactory_StdoutExporter_PrettyPrint(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Metric: &MetricConfig{
			Enabled:  true,
			Interval: 10 * time.Second,
			Exporter: ExporterTypeStdout,
			Stdout: &StdoutConfig{
				PrettyPrint: true,
			},
			GRPC: &GRPCConfig{
				Timeout: 5 * time.Second,
			},
		},
	}

	mp, err := MetricFactory(cfg, res)
	require.NoError(t, err)
	assert.NotNil(t, mp)

	// Cleanup
	err = mp.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestMetricFactory_UnknownExporter(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Metric: &MetricConfig{
			Enabled:  true,
			Exporter: ExporterType("unknown"),
		},
	}

	mp, err := MetricFactory(cfg, res)
	require.Error(t, err)
	assert.Nil(t, mp)
	assert.Contains(t, err.Error(), "unknown metric exporter type")
}

func TestNewMetricProvider(t *testing.T) {
	cfg := &Config{
		ShutdownTimeout: 5 * time.Second,
		Metric: &MetricConfig{
			Enabled: false,
		},
	}

	provider := NewMetricProvider(cfg, nil)
	assert.NotNil(t, provider)
}

func TestMetricProvider_Name(t *testing.T) {
	provider := NewMetricProvider(&Config{}, nil)
	assert.Equal(t, "otel.metric", provider.Name())
}

func TestMetricProvider_Priority(t *testing.T) {
	provider := NewMetricProvider(&Config{}, nil)
	assert.Equal(t, 0, provider.Priority())
}

func TestMetricProvider_Start(t *testing.T) {
	provider := NewMetricProvider(&Config{}, nil)
	err := provider.Start()
	assert.NoError(t, err)
}

func TestMetricProvider_Stop_NilProvider(t *testing.T) {
	cfg := &Config{
		ShutdownTimeout: 5 * time.Second,
	}
	provider := NewMetricProvider(cfg, nil)

	err := provider.Stop()
	assert.NoError(t, err)
}

func TestMetricProvider_Stop_WithProvider(t *testing.T) {
	res, _ := resource.New(nil)

	cfg := &Config{
		ShutdownTimeout: 5 * time.Second,
		Metric: &MetricConfig{
			Enabled:  true,
			Interval: 10 * time.Second,
			Exporter: ExporterTypeStdout,
			GRPC: &GRPCConfig{
				Timeout: 5 * time.Second,
			},
		},
	}

	mp, err := MetricFactory(cfg, res)
	require.NoError(t, err)
	require.NotNil(t, mp)

	provider := NewMetricProvider(cfg, mp)

	err = provider.Stop()
	assert.NoError(t, err)
}

func TestMetricFactory_GRPCExporter_WithRetry(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Metric: &MetricConfig{
			Enabled:  true,
			Interval: 10 * time.Second,
			Exporter: ExporterTypeGRPC,
			GRPC: &GRPCConfig{
				Endpoint: "localhost:4317",
				Insecure: true,
				Timeout:  1 * time.Second,
				Retry: &RetryConfig{
					Enabled:         true,
					InitialInterval: 500 * time.Millisecond,
					MaxInterval:     5 * time.Second,
					MaxElapsedTime:  30 * time.Second,
				},
			},
		},
	}

	mp, err := MetricFactory(cfg, res)
	// The factory might succeed (lazy connection) or fail
	if err == nil && mp != nil {
		_ = mp.Shutdown(context.Background())
	}
}
