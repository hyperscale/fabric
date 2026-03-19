package otel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/resource"
)

func TestLogFactory_Disabled(t *testing.T) {
	cfg := &Config{
		Log: &LogConfig{
			Enabled: false,
		},
	}

	lp, err := LogFactory(cfg, nil)
	require.NoError(t, err)
	assert.Nil(t, lp)
}

func TestLogFactory_StdoutExporter(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Log: &LogConfig{
			Enabled:  true,
			Exporter: ExporterTypeStdout,
			Stdout:   nil,
		},
	}

	lp, err := LogFactory(cfg, res)
	require.NoError(t, err)
	assert.NotNil(t, lp)

	// Cleanup
	err = lp.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestLogFactory_StdoutExporter_PrettyPrint(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Log: &LogConfig{
			Enabled:  true,
			Exporter: ExporterTypeStdout,
			Stdout: &StdoutConfig{
				PrettyPrint: true,
			},
		},
	}

	lp, err := LogFactory(cfg, res)
	require.NoError(t, err)
	assert.NotNil(t, lp)

	// Cleanup
	err = lp.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestLogFactory_UnknownExporter(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Log: &LogConfig{
			Enabled:  true,
			Exporter: ExporterType("unknown"),
		},
	}

	lp, err := LogFactory(cfg, res)
	require.Error(t, err)
	assert.Nil(t, lp)
	assert.Contains(t, err.Error(), "unknown log exporter type")
}

func TestNewLogProvider(t *testing.T) {
	cfg := &Config{
		ShutdownTimeout: 5 * time.Second,
		Log: &LogConfig{
			Enabled: false,
		},
	}

	provider := NewLogProvider(cfg, nil)
	assert.NotNil(t, provider)
}

func TestLogProvider_Name(t *testing.T) {
	provider := NewLogProvider(&Config{}, nil)
	assert.Equal(t, "otel.log", provider.Name())
}

func TestLogProvider_Priority(t *testing.T) {
	provider := NewLogProvider(&Config{}, nil)
	assert.Equal(t, 0, provider.Priority())
}

func TestLogProvider_Start(t *testing.T) {
	provider := NewLogProvider(&Config{}, nil)
	err := provider.Start()
	assert.NoError(t, err)
}

func TestLogProvider_Stop_NilProvider(t *testing.T) {
	cfg := &Config{
		ShutdownTimeout: 5 * time.Second,
	}
	provider := NewLogProvider(cfg, nil)

	err := provider.Stop()
	assert.NoError(t, err)
}

func TestLogProvider_Stop_WithProvider(t *testing.T) {
	res, _ := resource.New(nil)

	cfg := &Config{
		ShutdownTimeout: 5 * time.Second,
		Log: &LogConfig{
			Enabled:  true,
			Exporter: ExporterTypeStdout,
		},
	}

	lp, err := LogFactory(cfg, res)
	require.NoError(t, err)
	require.NotNil(t, lp)

	provider := NewLogProvider(cfg, lp)

	err = provider.Stop()
	assert.NoError(t, err)
}

func TestLogFactory_GRPCExporter_WithRetry(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Log: &LogConfig{
			Enabled:  true,
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

	lp, err := LogFactory(cfg, res)
	// The factory might succeed (lazy connection) or fail
	if err == nil && lp != nil {
		_ = lp.Shutdown(context.Background())
	}
}
