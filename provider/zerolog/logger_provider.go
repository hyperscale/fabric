// Copyright 2023 Axel Etcheverry. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.

package zerolog

import (
	"fmt"
	"io"
	stdlog "log"
	"os"

	"github.com/google/wire"
	"github.com/hyperscale/fabric"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

func Factory(cfg *Config) (*zerolog.Logger, error) {
	ll, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("failed to parse logger level: %w", err)
	}

	zerolog.SetGlobalLevel(ll)

	zerolog.CallerSkipFrameCount = 3

	logger := zerolog.New(os.Stdout).With().
		Timestamp().
		Caller().
		Logger()

	var output io.Writer

	switch cfg.Stdout {
	case StdOutputStdout:
		output = os.Stdout
	case StdOutputStderr:
		output = os.Stderr
	default:
		output = os.Stdout
	}

	// nolint: exhaustive // OutputFormatAuto is default
	switch cfg.Format {
	case OutputFormatJSON:
		logger = logger.Output(output)
	case OutputFormatConsole:
		logger = logger.Output(zerolog.ConsoleWriter{Out: output})
	default:
		fi, err := os.Stdin.Stat()
		if err != nil {
			logger.Fatal().Err(err).Msg("Stdin.Stat failed")
		}

		if (fi.Mode() & os.ModeCharDevice) != 0 {
			logger = logger.Output(zerolog.ConsoleWriter{Out: output})
		}
	}

	stdlog.SetFlags(0)
	stdlog.SetOutput(logger)

	log.Logger = logger

	return &logger, nil
}
