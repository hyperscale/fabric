package otel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExporterType_Constants(t *testing.T) {
	assert.Equal(t, ExporterType("grpc"), ExporterTypeGRPC)
	assert.Equal(t, ExporterType("stdout"), ExporterTypeStdout)
}

func TestConfig_Structures(t *testing.T) {
	t.Run("TraceConfig", func(t *testing.T) {
		cfg := &TraceConfig{
			Enabled:      true,
			BatchTimeout: 10 * time.Second,
			Exporter:     ExporterTypeGRPC,
			GRPC: &GRPCConfig{
				Endpoint: "test:4317",
				Insecure: false,
				Headers:  map[string]string{"key": "value"},
				Timeout:  5 * time.Second,
				Retry: &RetryConfig{
					Enabled:         true,
					InitialInterval: 1 * time.Second,
					MaxInterval:     30 * time.Second,
					MaxElapsedTime:  5 * time.Minute,
				},
			},
			Stdout: &StdoutConfig{
				PrettyPrint: true,
			},
		}

		assert.True(t, cfg.Enabled)
		assert.Equal(t, ExporterTypeGRPC, cfg.Exporter)
		assert.Equal(t, "test:4317", cfg.GRPC.Endpoint)
		assert.True(t, cfg.GRPC.Retry.Enabled)
		assert.True(t, cfg.Stdout.PrettyPrint)
	})

	t.Run("MetricConfig", func(t *testing.T) {
		cfg := &MetricConfig{
			Enabled:  true,
			Interval: 15 * time.Second,
			Exporter: ExporterTypeStdout,
			GRPC: &GRPCConfig{
				Endpoint: "metrics:4317",
				Insecure: true,
			},
		}

		assert.True(t, cfg.Enabled)
		assert.Equal(t, 15*time.Second, cfg.Interval)
		assert.Equal(t, ExporterTypeStdout, cfg.Exporter)
	})

	t.Run("LogConfig", func(t *testing.T) {
		cfg := &LogConfig{
			Enabled:  true,
			Exporter: ExporterTypeGRPC,
			GRPC: &GRPCConfig{
				Endpoint: "logs:4317",
				Insecure: false,
			},
		}

		assert.True(t, cfg.Enabled)
		assert.Equal(t, ExporterTypeGRPC, cfg.Exporter)
		assert.Equal(t, "logs:4317", cfg.GRPC.Endpoint)
	})

	t.Run("RetryConfig", func(t *testing.T) {
		cfg := &RetryConfig{
			Enabled:         true,
			InitialInterval: 500 * time.Millisecond,
			MaxInterval:     30 * time.Second,
			MaxElapsedTime:  2 * time.Minute,
		}

		assert.True(t, cfg.Enabled)
		assert.Equal(t, 500*time.Millisecond, cfg.InitialInterval)
		assert.Equal(t, 30*time.Second, cfg.MaxInterval)
		assert.Equal(t, 2*time.Minute, cfg.MaxElapsedTime)
	})

	t.Run("StdoutConfig", func(t *testing.T) {
		cfg := &StdoutConfig{
			PrettyPrint: true,
		}

		assert.True(t, cfg.PrettyPrint)
	})

	t.Run("GRPCConfig", func(t *testing.T) {
		cfg := &GRPCConfig{
			Endpoint: "otel-collector:4317",
			Insecure: true,
			Headers: map[string]string{
				"Authorization": "Bearer token",
				"X-Custom":      "value",
			},
			Timeout: 15 * time.Second,
		}

		assert.Equal(t, "otel-collector:4317", cfg.Endpoint)
		assert.True(t, cfg.Insecure)
		assert.Equal(t, "Bearer token", cfg.Headers["Authorization"])
		assert.Equal(t, 15*time.Second, cfg.Timeout)
	})
}
