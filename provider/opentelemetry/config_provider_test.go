package otel

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperscale/fabric"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadConfiguration(t *testing.T, content string) *fabric.Configuration {
	t.Helper()

	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.hcl"), []byte(content), 0o600))

	cfg, err := fabric.NewConfigurationFromDir(dir)
	require.NoError(t, err)

	return cfg
}

func TestConfigProvider_AbsentBlockKeepsDefaults(t *testing.T) {
	c, err := ConfigProvider(loadConfiguration(t, ""))
	require.NoError(t, err)

	assert.Equal(t, defaultShutdownTimeout, c.ShutdownTimeout)
	assert.Equal(t, defaultBatchTimeout, c.Trace.BatchTimeout)
	assert.Equal(t, defaultGRPCTimeout, c.Trace.GRPC.Timeout)
	assert.Equal(t, defaultMetricInterval, c.Metric.Interval)
	assert.Equal(t, defaultGRPCTimeout, c.Metric.GRPC.Timeout)
	assert.Equal(t, defaultGRPCTimeout, c.Log.GRPC.Timeout)
	assert.Equal(t, defaultGRPCEndpoint, c.Trace.GRPC.Endpoint)
}

func TestConfigProvider_ParsesDurationStrings(t *testing.T) {
	c, err := ConfigProvider(loadConfiguration(t, `
provider "opentelemetry" {
  shutdown_timeout = "3s"

  trace {
    enabled       = true
    exporter      = "grpc"
    batch_timeout = "2s"

    grpc {
      endpoint = "collector:4317"
      timeout  = "7s"

      retry {
        enabled          = true
        initial_interval = "1s"
        max_interval     = "10s"
        max_elapsed_time = "1m"
      }
    }
  }

  metric {
    enabled  = true
    interval = "30s"

    grpc {
      timeout = "4s"
    }
  }

  log {
    enabled = true

    grpc {
      timeout = "6s"
    }
  }
}
`))
	require.NoError(t, err)

	assert.Equal(t, 3*time.Second, c.ShutdownTimeout)
	assert.Equal(t, 2*time.Second, c.Trace.BatchTimeout)
	assert.Equal(t, "collector:4317", c.Trace.GRPC.Endpoint)
	assert.Equal(t, 7*time.Second, c.Trace.GRPC.Timeout)
	assert.Equal(t, &RetryConfig{
		Enabled:            true,
		RawInitialInterval: "1s",
		RawMaxInterval:     "10s",
		RawMaxElapsedTime:  "1m",
		InitialInterval:    time.Second,
		MaxInterval:        10 * time.Second,
		MaxElapsedTime:     time.Minute,
	}, c.Trace.GRPC.Retry)
	assert.Equal(t, 30*time.Second, c.Metric.Interval)
	assert.Equal(t, 4*time.Second, c.Metric.GRPC.Timeout)
	assert.Equal(t, 6*time.Second, c.Log.GRPC.Timeout)
}

// A present block replaces the default struct wholesale, so every duration it
// leaves out must fall back to its default instead of staying at zero.
func TestConfigProvider_PresentBlockRestoresDefaults(t *testing.T) {
	c, err := ConfigProvider(loadConfiguration(t, `
provider "opentelemetry" {
  trace {
    enabled = true

    grpc {
      endpoint = "collector:4317"
    }
  }

  metric {
    enabled = true
  }
}
`))
	require.NoError(t, err)

	assert.Equal(t, defaultShutdownTimeout, c.ShutdownTimeout)
	assert.Equal(t, defaultBatchTimeout, c.Trace.BatchTimeout)
	assert.Equal(t, defaultGRPCTimeout, c.Trace.GRPC.Timeout)
	assert.Equal(t, defaultMetricInterval, c.Metric.Interval)
	assert.Equal(t, defaultGRPCTimeout, c.Metric.GRPC.Timeout)
}

func TestConfigProvider_InvalidDuration(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "shutdown timeout",
			content: `
provider "opentelemetry" {
  shutdown_timeout = "5sec"
}
`,
			expected: "shutdown_timeout",
		},
		{
			name: "trace batch timeout",
			content: `
provider "opentelemetry" {
  trace {
    batch_timeout = "soon"
  }
}
`,
			expected: "trace.batch_timeout",
		},
		{
			name: "metric interval",
			content: `
provider "opentelemetry" {
  metric {
    interval = "10"
  }
}
`,
			expected: "metric.interval",
		},
		{
			name: "log grpc timeout",
			content: `
provider "opentelemetry" {
  log {
    grpc {
      timeout = "x"
    }
  }
}
`,
			expected: "log.grpc: timeout",
		},
		{
			name: "trace grpc retry",
			content: `
provider "opentelemetry" {
  trace {
    grpc {
      retry {
        max_interval = "x"
      }
    }
  }
}
`,
			expected: "trace.grpc: retry.max_interval",
		},
		{
			name: "metric grpc retry",
			content: `
provider "opentelemetry" {
  metric {
    grpc {
      retry {
        initial_interval = "x"
      }
    }
  }
}
`,
			expected: "metric.grpc: retry.initial_interval",
		},
		{
			name: "log grpc retry",
			content: `
provider "opentelemetry" {
  log {
    grpc {
      retry {
        max_elapsed_time = "x"
      }
    }
  }
}
`,
			expected: "log.grpc: retry.max_elapsed_time",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := ConfigProvider(loadConfiguration(t, c.content))
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), "invalid opentelemetry config: "+c.expected)
		})
	}
}

func TestConfigProvider_MalformedBlock(t *testing.T) {
	cfg, err := ConfigProvider(loadConfiguration(t, `
provider "opentelemetry" {
  unknown = true
}
`))
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.ErrorIs(t, err, fabric.ErrProviderInvalid)
}
