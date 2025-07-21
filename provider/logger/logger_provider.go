// Copyright 2023 Axel Etcheverry. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/euskadi31/wire"
	"github.com/hyperscale/fabric"
)

const ProviderName = "logger"

var Set = wire.NewSet(ConfigProvider, Factory)

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

type Config struct {
	Level  string       `hcl:"level"`
	Format OutputFormat `hcl:"format"`
	Stdout StdOutput    `hcl:"stdout"`
}

func ConfigProvider(cfg *fabric.Configuration) (*Config, error) {
	c := &Config{
		Level:  "debug",
		Format: OutputFormatAuto,
		Stdout: StdOutputStdout,
	}

	if err := cfg.ParseProvider(ProviderName, c); err != nil {
		return c, nil // nolint: nilerr // return default config if parsing failed
	}

	return c, nil
}

func Factory(cfg *Config) (*slog.Logger, error) {
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
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.SourceKey:
				return slog.String("log.origin.file.name", a.Value.String())
			case slog.TimeKey:
				return slog.String("@timestamp", a.Value.String())
			default:
				return a
			}
		},
	}

	var h slog.Handler

	// nolint: exhaustive // OutputFormatAuto is default
	switch cfg.Format {
	case OutputFormatJSON:
		h = slog.NewJSONHandler(output, opts)

	case OutputFormatConsole:
		h = slog.NewTextHandler(output, opts)
	default:
		h = slog.NewJSONHandler(output, opts)
	}

	logger := slog.New(h)

	slog.SetDefault(logger)

	return logger, nil
}
