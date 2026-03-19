package otel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/resource"
)

func TestTraceFactory_Disabled(t *testing.T) {
	cfg := &Config{
		Trace: &TraceConfig{
			Enabled: false,
		},
	}

	tp, err := TraceFactory(cfg, nil)
	require.NoError(t, err)
	assert.Nil(t, tp)
}

func TestTraceFactory_StdoutExporter(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Trace: &TraceConfig{
			Enabled:      true,
			BatchTimeout: 5 * time.Second,
			Exporter:     ExporterTypeStdout,
			Stdout:       nil,
		},
	}

	tp, err := TraceFactory(cfg, res)
	require.NoError(t, err)
	assert.NotNil(t, tp)

	// Cleanup
	err = tp.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestTraceFactory_StdoutExporter_PrettyPrint(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Trace: &TraceConfig{
			Enabled:      true,
			BatchTimeout: 5 * time.Second,
			Exporter:     ExporterTypeStdout,
			Stdout: &StdoutConfig{
				PrettyPrint: true,
			},
		},
	}

	tp, err := TraceFactory(cfg, res)
	require.NoError(t, err)
	assert.NotNil(t, tp)

	// Cleanup
	err = tp.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestTraceFactory_UnknownExporter(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Trace: &TraceConfig{
			Enabled:  true,
			Exporter: ExporterType("unknown"),
		},
	}

	tp, err := TraceFactory(cfg, res)
	require.Error(t, err)
	assert.Nil(t, tp)
	assert.Contains(t, err.Error(), "unknown trace exporter type")
}

func TestNewTraceProvider(t *testing.T) {
	cfg := &Config{
		ShutdownTimeout: 5 * time.Second,
		Trace: &TraceConfig{
			Enabled: false,
		},
	}

	provider := NewTraceProvider(cfg, nil)
	assert.NotNil(t, provider)
}

func TestTraceProvider_Name(t *testing.T) {
	provider := NewTraceProvider(&Config{}, nil)
	assert.Equal(t, "otel.trace", provider.Name())
}

func TestTraceProvider_Priority(t *testing.T) {
	provider := NewTraceProvider(&Config{}, nil)
	assert.Equal(t, 0, provider.Priority())
}

func TestTraceProvider_Start(t *testing.T) {
	provider := NewTraceProvider(&Config{}, nil)
	err := provider.Start()
	assert.NoError(t, err)
}

func TestTraceProvider_Stop_NilProvider(t *testing.T) {
	cfg := &Config{
		ShutdownTimeout: 5 * time.Second,
	}
	provider := NewTraceProvider(cfg, nil)

	err := provider.Stop()
	assert.NoError(t, err)
}

func TestTraceProvider_Stop_WithProvider(t *testing.T) {
	res, _ := resource.New(nil)

	cfg := &Config{
		ShutdownTimeout: 5 * time.Second,
		Trace: &TraceConfig{
			Enabled:      true,
			BatchTimeout: 1 * time.Second,
			Exporter:     ExporterTypeStdout,
		},
	}

	tp, err := TraceFactory(cfg, res)
	require.NoError(t, err)
	require.NotNil(t, tp)

	provider := NewTraceProvider(cfg, tp)

	err = provider.Stop()
	assert.NoError(t, err)
}

func TestTraceFactory_GRPCExporter_WithRetry(t *testing.T) {
	res, err := resource.New(nil)
	require.NoError(t, err)

	cfg := &Config{
		Trace: &TraceConfig{
			Enabled:      true,
			BatchTimeout: 5 * time.Second,
			Exporter:     ExporterTypeGRPC,
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

	tp, err := TraceFactory(cfg, res)
	// The factory might succeed (lazy connection) or fail
	if err == nil && tp != nil {
		_ = tp.Shutdown(context.Background())
	}
}
