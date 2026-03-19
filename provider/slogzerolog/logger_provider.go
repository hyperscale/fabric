package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/euskadi31/wire"
	"github.com/hyperscale/fabric"
	"github.com/rs/zerolog"
	slogzerolog "github.com/samber/slog-zerolog/v2"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

const providerName = "logger"

var LoggerSet = wire.NewSet(ConfigProvider, Factory)

type OutputFormat string

const (
	OutputFormatAuto    OutputFormat = "auto"
	OutputFormatJSON    OutputFormat = "json"
	OutputFormatConsole OutputFormat = "console"
)

type StdOutput string

const (
	StdOutputStdout StdOutput = "stdout"
	StdOutputStderr StdOutput = "stderr"
)

type ProviderType string

const (
	ProviderTypeSlog ProviderType = "slog"
	ProviderTypeOtel ProviderType = "otel"
)

type Config struct {
	Level     string       `hcl:"level"`
	Provider  ProviderType `hcl:"provider"`
	Format    OutputFormat `hcl:"format"`
	Stdout    StdOutput    `hcl:"stdout"`
	AddSource bool         `hcl:"add_source"`
}

func ConfigProvider(cfg *fabric.Configuration) (*Config, error) {
	c := &Config{
		Level:     "debug",
		Provider:  ProviderTypeSlog,
		Format:    OutputFormatAuto,
		Stdout:    StdOutputStdout,
		AddSource: false,
	}

	if err := cfg.ParseProvider(providerName, c); err != nil {
		return c, nil // nolint: nilerr // return default config if parsing failed
	}

	return c, nil
}

func factorySlog(cfg *Config) (*slog.Logger, error) {
	var ll slog.Level

	err := ll.UnmarshalText([]byte(cfg.Level))
	if err != nil {
		return nil, fmt.Errorf("failed to parse logger level: %w", err)
	}

	var output io.Writer

	switch cfg.Stdout {
	case StdOutputStdout:
		output = os.Stdout
	case StdOutputStderr:
		output = os.Stderr
	default:
		output = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level:     ll,
		AddSource: cfg.AddSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.SourceKey:
				return slog.String("log.origin.file.name", a.Value.String())
			case slog.TimeKey:
				return slog.String("@timestamp", a.Value.String())
			}

			return a
		},
	}

	var h slog.Handler

	// nolint: exhaustive // OutputFormatAuto is default
	switch cfg.Format {
	case OutputFormatJSON:
		h = slog.NewJSONHandler(output, opts)

	case OutputFormatConsole:
		// h = slog.NewTextHandler(output, opts)
		zerologLogger := zerolog.New(zerolog.ConsoleWriter{
			Out: output,
		})

		h = slogzerolog.Option{
			Level:       opts.Level,
			Logger:      &zerologLogger,
			AddSource:   opts.AddSource,
			ReplaceAttr: opts.ReplaceAttr,
		}.NewZerologHandler()
	default:
		h = slog.NewJSONHandler(output, opts)
	}

	return slog.New(h), nil
}

func factoryOtel(cfg *Config, provider *sdklog.LoggerProvider) (*slog.Logger, error) {
	opts := []otelslog.Option{
		otelslog.WithLoggerProvider(provider),
		otelslog.WithSource(cfg.AddSource),
	}

	return otelslog.NewLogger("", opts...), nil
}

func Factory(cfg *Config, provider *sdklog.LoggerProvider) (*slog.Logger, error) {
	var (
		logger *slog.Logger
		err    error
	)

	switch cfg.Provider {
	case ProviderTypeSlog:
		logger, err = factorySlog(cfg)
	case ProviderTypeOtel:
		logger, err = factoryOtel(cfg, provider)
	default:
		return nil, fmt.Errorf("unknown logger provider type %s", cfg.Provider)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	slog.SetDefault(logger)

	return logger, nil
}
