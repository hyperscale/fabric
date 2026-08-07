package zerolog

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/hyperscale/fabric"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restoreGlobals puts back the process-wide state zerolog's Factory mutates, so
// one test cannot leak its configuration into the next.
func restoreGlobals(t *testing.T) {
	t.Helper()

	level := zerolog.GlobalLevel()
	skip := zerolog.CallerSkipFrameCount
	logger := log.Logger

	t.Cleanup(func() {
		zerolog.SetGlobalLevel(level)

		zerolog.CallerSkipFrameCount = skip
		log.Logger = logger
	})
}

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
	assert.Equal(t, "error", c.Level)
	assert.Equal(t, OutputFormatJSON, c.Format)
	assert.Equal(t, StdOutputStderr, c.Stdout)
}

// A malformed block is fatal rather than silently degrading to the defaults.
func TestConfigProvider_InvalidBlockIsFatal(t *testing.T) {
	cfg, err := fabric.NewConfigurationFromDir("./testdata/invalid")
	require.NoError(t, err)

	c, err := ConfigProvider(cfg)

	require.Error(t, err)
	assert.Nil(t, c)
	assert.ErrorIs(t, err, fabric.ErrProviderInvalid)
	assert.NotErrorIs(t, err, fabric.ErrProviderNotFound)
}

func TestFactory_SetsGlobalLevel(t *testing.T) {
	cases := map[string]zerolog.Level{
		"trace": zerolog.TraceLevel,
		"debug": zerolog.DebugLevel,
		"info":  zerolog.InfoLevel,
		"warn":  zerolog.WarnLevel,
		"error": zerolog.ErrorLevel,
		"fatal": zerolog.FatalLevel,
	}

	for level, want := range cases {
		t.Run(level, func(t *testing.T) {
			restoreGlobals(t)

			logger, err := Factory(&Config{Level: level, Format: OutputFormatJSON, Stdout: StdOutputStdout})

			require.NoError(t, err)
			require.NotNil(t, logger)
			assert.Equal(t, want, zerolog.GlobalLevel())
		})
	}
}

func TestFactory_InvalidLevel(t *testing.T) {
	restoreGlobals(t)

	logger, err := Factory(&Config{Level: "verbose", Format: OutputFormatJSON})

	require.Error(t, err)
	assert.Nil(t, logger)
	assert.Contains(t, err.Error(), "failed to parse logger level")
}

func TestFactory_Formats(t *testing.T) {
	for _, format := range []OutputFormat{OutputFormatJSON, OutputFormatConsole, OutputFormatAuto, "unknown"} {
		t.Run(string(format), func(t *testing.T) {
			restoreGlobals(t)

			logger, err := Factory(&Config{Level: "debug", Format: format, Stdout: StdOutputStdout})

			require.NoError(t, err)
			require.NotNil(t, logger)
		})
	}
}

func TestFactory_Outputs(t *testing.T) {
	for _, out := range []StdOutput{StdOutputStdout, StdOutputStderr, "unknown"} {
		t.Run(string(out), func(t *testing.T) {
			restoreGlobals(t)

			logger, err := Factory(&Config{Level: "debug", Format: OutputFormatJSON, Stdout: out})

			require.NoError(t, err)
			require.NotNil(t, logger)
		})
	}
}

// Factory adopts the logger as zerolog's package-level logger and redirects the
// standard library's log package into it, so third-party code logging through
// either still lands in the configured stream.
func TestFactory_InstallsGlobalLogger(t *testing.T) {
	restoreGlobals(t)

	logger, err := Factory(&Config{Level: "debug", Format: OutputFormatJSON, Stdout: StdOutputStdout})
	require.NoError(t, err)

	assert.Equal(t, logger.GetLevel(), log.Logger.GetLevel())
	assert.Equal(t, 3, zerolog.CallerSkipFrameCount)
}

// The JSON output must carry a timestamp and a caller, which is the whole point
// of the With().Timestamp().Caller() chain.
func TestFactory_JSONRecordHasTimestampAndCaller(t *testing.T) {
	record := captureJSONRecord(t, &Config{Level: "debug", Format: OutputFormatJSON, Stdout: StdOutputStdout})

	assert.Contains(t, record, zerolog.TimestampFieldName)
	assert.Contains(t, record, zerolog.CallerFieldName)
	assert.Equal(t, "hello", record[zerolog.MessageFieldName])
	assert.Equal(t, "world", record["key"])
}

// A level below the configured one must produce no output at all.
func TestFactory_FiltersBelowConfiguredLevel(t *testing.T) {
	restoreGlobals(t)

	previousStdout := os.Stdout

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	t.Cleanup(func() { os.Stdout = previousStdout })

	logger, err := Factory(&Config{Level: "error", Format: OutputFormatJSON, Stdout: StdOutputStdout})
	require.NoError(t, err)

	logger.Debug().Msg("should be dropped")

	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Empty(t, out, "a debug record was emitted at error level")
}

// captureJSONRecord swaps os.Stdout for a pipe before building the logger,
// because Factory resolves os.Stdout at call time.
func captureJSONRecord(t *testing.T, cfg *Config) map[string]any {
	t.Helper()

	restoreGlobals(t)

	previousStdout := os.Stdout

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	t.Cleanup(func() { os.Stdout = previousStdout })

	logger, err := Factory(cfg)
	require.NoError(t, err)

	logger.Info().Str("key", "world").Msg("hello")

	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NotEmpty(t, out, "the logger wrote nothing")

	var record map[string]any

	require.NoError(t, json.Unmarshal(out, &record))

	return record
}
