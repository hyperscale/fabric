package logger

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/hyperscale/fabric"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigProvider_Defaults(t *testing.T) {
	cfg, err := fabric.NewConfigurationFromDir("./testdata/absent")
	require.NoError(t, err)

	c, err := ConfigProvider(cfg)

	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "debug", c.Level)
	assert.Equal(t, OutputFormatAuto, c.Format)
	assert.Equal(t, StdOutputStdout, c.Stdout)
}

func TestConfigProvider_ReadsTheBlock(t *testing.T) {
	cfg, err := fabric.NewConfigurationFromDir("./testdata/valid")
	require.NoError(t, err)

	c, err := ConfigProvider(cfg)

	require.NoError(t, err)
	assert.Equal(t, "warn", c.Level)
	assert.Equal(t, OutputFormatConsole, c.Format)
	assert.Equal(t, StdOutputStderr, c.Stdout)
}

// A block that exists but does not decode must be fatal. It used to fall back to
// the defaults through a `nolint: nilerr` shortcut, so a typo silently changed
// the logging configuration in production with no diagnostic at all.
func TestConfigProvider_InvalidBlockIsFatal(t *testing.T) {
	cfg, err := fabric.NewConfigurationFromDir("./testdata/invalid")
	require.NoError(t, err)

	c, err := ConfigProvider(cfg)

	require.Error(t, err)
	assert.Nil(t, c)
	assert.ErrorIs(t, err, fabric.ErrProviderInvalid)
	assert.NotErrorIs(t, err, fabric.ErrProviderNotFound)
}

func TestFactory_Levels(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"DEBUG": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}

	for level, want := range cases {
		t.Run(level, func(t *testing.T) {
			logger, err := Factory(&Config{Level: level, Format: OutputFormatJSON, Stdout: StdOutputStdout})

			require.NoError(t, err)
			require.NotNil(t, logger)

			assert.True(t, logger.Enabled(t.Context(), want))

			if want > slog.LevelDebug {
				assert.False(t, logger.Enabled(t.Context(), want-1), "a lower level must be filtered out")
			}
		})
	}
}

func TestFactory_InvalidLevel(t *testing.T) {
	logger, err := Factory(&Config{Level: "verbose", Format: OutputFormatJSON})

	require.Error(t, err)
	assert.Nil(t, logger)
	assert.Contains(t, err.Error(), "failed to parse logger level")
}

func TestFactory_Formats(t *testing.T) {
	for _, format := range []OutputFormat{OutputFormatJSON, OutputFormatConsole, OutputFormatAuto, "unknown"} {
		t.Run(string(format), func(t *testing.T) {
			logger, err := Factory(&Config{Level: "debug", Format: format, Stdout: StdOutputStdout})

			require.NoError(t, err)
			require.NotNil(t, logger)
		})
	}
}

func TestFactory_Outputs(t *testing.T) {
	for _, out := range []StdOutput{StdOutputStdout, StdOutputStderr, "unknown"} {
		t.Run(string(out), func(t *testing.T) {
			logger, err := Factory(&Config{Level: "debug", Format: OutputFormatJSON, Stdout: out})

			require.NoError(t, err)
			require.NotNil(t, logger)
		})
	}
}

// Factory installs the logger as the slog default, which is what lets code that
// never received the logger still emit structured records.
func TestFactory_SetsTheDefaultLogger(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	logger, err := Factory(&Config{Level: "debug", Format: OutputFormatJSON, Stdout: StdOutputStdout})
	require.NoError(t, err)

	assert.Equal(t, logger.Handler(), slog.Default().Handler())
}

// The handler rewrites the standard slog keys to the names the log pipeline
// expects, so a change here is a breaking change for downstream queries. It also
// pins that source location is emitted at all, which AddSource controls.
func TestFactory_EmitsRenamedSourceAndTimeKeys(t *testing.T) {
	record := captureJSONRecord(t, &Config{Level: "debug", Format: OutputFormatJSON, Stdout: StdOutputStdout})

	assert.Contains(t, record, "log.origin.file.name")
	assert.Contains(t, record, "@timestamp")
	assert.NotContains(t, record, slog.SourceKey)
	assert.NotContains(t, record, slog.TimeKey)

	assert.Equal(t, "hello", record[slog.MessageKey])
	assert.Equal(t, "world", record["key"])
}

// captureJSONRecord builds a logger writing to a pipe standing in for os.Stdout,
// emits one record and decodes it. The swap must happen before Factory runs,
// because Factory resolves os.Stdout at call time.
func captureJSONRecord(t *testing.T, cfg *Config) map[string]any {
	t.Helper()

	previousLogger := slog.Default()
	previousStdout := os.Stdout

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	t.Cleanup(func() {
		os.Stdout = previousStdout

		slog.SetDefault(previousLogger)
	})

	logger, err := Factory(cfg)
	require.NoError(t, err)

	logger.Info("hello", slog.String("key", "world"))

	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NotEmpty(t, out, "the logger wrote nothing")

	var record map[string]any

	require.NoError(t, json.Unmarshal(out, &record))

	return record
}
