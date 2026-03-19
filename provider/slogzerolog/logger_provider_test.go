package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func TestOutputFormat_Constants(t *testing.T) {
	assert.Equal(t, OutputFormat("auto"), OutputFormatAuto)
	assert.Equal(t, OutputFormat("json"), OutputFormatJSON)
	assert.Equal(t, OutputFormat("console"), OutputFormatConsole)
}

func TestStdOutput_Constants(t *testing.T) {
	assert.Equal(t, StdOutput("stdout"), StdOutputStdout)
	assert.Equal(t, StdOutput("stderr"), StdOutputStderr)
}

func TestProviderType_Constants(t *testing.T) {
	assert.Equal(t, ProviderType("slog"), ProviderTypeSlog)
	assert.Equal(t, ProviderType("otel"), ProviderTypeOtel)
}

func TestFactory_Slog_JSONFormat(t *testing.T) {
	cfg := &Config{
		Level:     "info",
		Provider:  ProviderTypeSlog,
		Format:    OutputFormatJSON,
		Stdout:    StdOutputStdout,
		AddSource: false,
	}

	logger, err := Factory(cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestFactory_Slog_ConsoleFormat(t *testing.T) {
	cfg := &Config{
		Level:     "debug",
		Provider:  ProviderTypeSlog,
		Format:    OutputFormatConsole,
		Stdout:    StdOutputStdout,
		AddSource: false,
	}

	logger, err := Factory(cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestFactory_Slog_AutoFormat(t *testing.T) {
	cfg := &Config{
		Level:     "warn",
		Provider:  ProviderTypeSlog,
		Format:    OutputFormatAuto,
		Stdout:    StdOutputStdout,
		AddSource: false,
	}

	logger, err := Factory(cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestFactory_Slog_StderrOutput(t *testing.T) {
	cfg := &Config{
		Level:     "error",
		Provider:  ProviderTypeSlog,
		Format:    OutputFormatJSON,
		Stdout:    StdOutputStderr,
		AddSource: true,
	}

	logger, err := Factory(cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestFactory_Slog_UnknownOutput(t *testing.T) {
	cfg := &Config{
		Level:     "info",
		Provider:  ProviderTypeSlog,
		Format:    OutputFormatJSON,
		Stdout:    StdOutput("unknown"),
		AddSource: false,
	}

	// Should default to stdout
	logger, err := Factory(cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestFactory_Slog_InvalidLevel(t *testing.T) {
	cfg := &Config{
		Level:    "invalid-level",
		Provider: ProviderTypeSlog,
		Format:   OutputFormatJSON,
		Stdout:   StdOutputStdout,
	}

	logger, err := Factory(cfg, nil)
	require.Error(t, err)
	assert.Nil(t, logger)
	assert.Contains(t, err.Error(), "failed to parse logger level")
}

func TestFactory_Otel(t *testing.T) {
	// Create a minimal log provider for testing
	lp := sdklog.NewLoggerProvider()
	defer lp.Shutdown(context.TODO()) //nolint:errcheck

	cfg := &Config{
		Level:     "info",
		Provider:  ProviderTypeOtel,
		Format:    OutputFormatJSON,
		Stdout:    StdOutputStdout,
		AddSource: true,
	}

	logger, err := Factory(cfg, lp)
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestFactory_UnknownProvider(t *testing.T) {
	cfg := &Config{
		Level:    "info",
		Provider: ProviderType("unknown"),
		Format:   OutputFormatJSON,
		Stdout:   StdOutputStdout,
	}

	logger, err := Factory(cfg, nil)
	require.Error(t, err)
	assert.Nil(t, logger)
	assert.Contains(t, err.Error(), "unknown logger provider type")
}

func TestFactory_AllLogLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			cfg := &Config{
				Level:    level,
				Provider: ProviderTypeSlog,
				Format:   OutputFormatJSON,
				Stdout:   StdOutputStdout,
			}

			logger, err := Factory(cfg, nil)
			require.NoError(t, err)
			assert.NotNil(t, logger)
		})
	}
}
