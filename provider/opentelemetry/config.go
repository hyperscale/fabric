package otel

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/euskadi31/wire"
	"github.com/hyperscale/fabric"
)

// ProviderName is the name of the HCL provider block this package reads.
const ProviderName = "opentelemetry"

var OTelConfigSet = wire.NewSet(ConfigProvider)

type ExporterType string

const (
	ExporterTypeGRPC   ExporterType = "grpc"
	ExporterTypeStdout ExporterType = "stdout"
)

// Default durations, applied by normalize when the configuration leaves one
// unset. They live here rather than only in the ConfigProvider literal because
// a *present* HCL block replaces the whole struct: gohcl allocates a fresh one,
// so any default written into the literal is discarded the moment an operator
// writes `trace { ... }`.
const (
	defaultShutdownTimeout = 5 * time.Second
	defaultBatchTimeout    = 5 * time.Second
	defaultMetricInterval  = 10 * time.Second
	defaultGRPCTimeout     = 10 * time.Second
)

const defaultGRPCEndpoint = "localhost:4317"

// Every duration below is decoded as a string and parsed afterwards. gohcl
// decodes through gocty, which maps a time.Duration field to a bare number and
// ignores encoding.TextUnmarshaler — so a field typed time.Duration reads
// *nanoseconds*, and the `"1s"` these configuration files are written with does
// not decode at all. Before #127 that failure was swallowed and the whole
// opentelemetry block silently reverted to defaults.
type Config struct {
	Trace                 *TraceConfig  `hcl:"trace,block"`
	Metric                *MetricConfig `hcl:"metric,block"`
	Log                   *LogConfig    `hcl:"log,block"`
	ServiceName           string        `hcl:"service_name,optional"`
	ServiceVersion        string        `hcl:"service_version,optional"`
	DeploymentEnvironment string        `hcl:"deployment_environment,optional"`
	RawShutdownTimeout    string        `hcl:"shutdown_timeout,optional"`

	// Parsed from RawShutdownTimeout; carries no HCL tag of its own.
	ShutdownTimeout time.Duration
}

type MetricConfig struct {
	Enabled     bool          `hcl:"enabled,optional"`
	RawInterval string        `hcl:"interval,optional"`
	Exporter    ExporterType  `hcl:"exporter,optional"` // grpc, stdout
	GRPC        *GRPCConfig   `hcl:"grpc,block"`
	Stdout      *StdoutConfig `hcl:"stdout,block"`

	Interval time.Duration
}

type TraceConfig struct {
	Enabled         bool          `hcl:"enabled,optional"`
	RawBatchTimeout string        `hcl:"batch_timeout,optional"`
	Exporter        ExporterType  `hcl:"exporter,optional"` // grpc, stdout
	GRPC            *GRPCConfig   `hcl:"grpc,block"`
	Stdout          *StdoutConfig `hcl:"stdout,block"`

	BatchTimeout time.Duration
}

type LogConfig struct {
	Enabled  bool          `hcl:"enabled,optional"`
	Exporter ExporterType  `hcl:"exporter,optional"` // grpc, stdout
	GRPC     *GRPCConfig   `hcl:"grpc,block"`
	Stdout   *StdoutConfig `hcl:"stdout,block"`
}

type StdoutConfig struct {
	PrettyPrint bool `hcl:"pretty_print,optional"`
}

type GRPCConfig struct {
	Endpoint   string            `hcl:"endpoint,optional"`
	Insecure   bool              `hcl:"insecure,optional"`
	Headers    map[string]string `hcl:"headers,optional"`
	Retry      *RetryConfig      `hcl:"retry,block"`
	RawTimeout string            `hcl:"timeout,optional"`

	Timeout time.Duration
}

type RetryConfig struct {
	Enabled            bool   `hcl:"enabled,optional"`
	RawInitialInterval string `hcl:"initial_interval,optional"`
	RawMaxInterval     string `hcl:"max_interval,optional"`
	RawMaxElapsedTime  string `hcl:"max_elapsed_time,optional"`

	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxElapsedTime  time.Duration
}

// normalize turns the raw duration strings into durations and restores the
// defaults a present HCL block would otherwise have wiped out.
func (c *Config) normalize() error {
	if err := parseDurationInto(c.RawShutdownTimeout, &c.ShutdownTimeout, defaultShutdownTimeout); err != nil {
		return fmt.Errorf("shutdown_timeout: %w", err)
	}

	if c.Trace != nil {
		if err := parseDurationInto(c.Trace.RawBatchTimeout, &c.Trace.BatchTimeout, defaultBatchTimeout); err != nil {
			return fmt.Errorf("trace.batch_timeout: %w", err)
		}

		if err := c.Trace.GRPC.normalize(); err != nil {
			return fmt.Errorf("trace.grpc: %w", err)
		}
	}

	if c.Metric != nil {
		if err := parseDurationInto(c.Metric.RawInterval, &c.Metric.Interval, defaultMetricInterval); err != nil {
			return fmt.Errorf("metric.interval: %w", err)
		}

		if err := c.Metric.GRPC.normalize(); err != nil {
			return fmt.Errorf("metric.grpc: %w", err)
		}
	}

	if c.Log != nil {
		if err := c.Log.GRPC.normalize(); err != nil {
			return fmt.Errorf("log.grpc: %w", err)
		}
	}

	return nil
}

func (g *GRPCConfig) normalize() error {
	if g == nil {
		return nil
	}

	if err := parseDurationInto(g.RawTimeout, &g.Timeout, defaultGRPCTimeout); err != nil {
		return fmt.Errorf("timeout: %w", err)
	}

	if g.Retry == nil {
		return nil
	}

	if err := parseDurationInto(g.Retry.RawInitialInterval, &g.Retry.InitialInterval, 0); err != nil {
		return fmt.Errorf("retry.initial_interval: %w", err)
	}

	if err := parseDurationInto(g.Retry.RawMaxInterval, &g.Retry.MaxInterval, 0); err != nil {
		return fmt.Errorf("retry.max_interval: %w", err)
	}

	if err := parseDurationInto(g.Retry.RawMaxElapsedTime, &g.Retry.MaxElapsedTime, 0); err != nil {
		return fmt.Errorf("retry.max_elapsed_time: %w", err)
	}

	return nil
}

// parseDurationInto writes the parsed raw value into dst, or fallback when raw
// is empty. An unparsable value is an error rather than a silent fallback: an
// operator who wrote "1sec" meant something, and quietly running on a default
// instead is how a misconfiguration reaches production.
func parseDurationInto(raw string, dst *time.Duration, fallback time.Duration) error {
	if raw == "" {
		if *dst == 0 {
			*dst = fallback
		}

		return nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", raw, err)
	}

	*dst = d

	return nil
}

func ConfigProvider(cfg *fabric.Configuration) (*Config, error) {
	serviceName := filepath.Base(os.Args[0])

	c := &Config{
		ServiceName:           serviceName,
		DeploymentEnvironment: "local",
		ShutdownTimeout:       defaultShutdownTimeout,
		Trace: &TraceConfig{
			Enabled:      false,
			Exporter:     ExporterTypeStdout,
			BatchTimeout: defaultBatchTimeout,
			GRPC: &GRPCConfig{
				Endpoint: defaultGRPCEndpoint,
				Insecure: true,
				Timeout:  defaultGRPCTimeout,
			},
		},
		Metric: &MetricConfig{
			Enabled:  false,
			Exporter: ExporterTypeStdout,
			Interval: defaultMetricInterval,
			GRPC: &GRPCConfig{
				Endpoint: defaultGRPCEndpoint,
				Insecure: true,
				Timeout:  defaultGRPCTimeout,
			},
		},
		Log: &LogConfig{
			Enabled:  true,
			Exporter: ExporterTypeStdout,
			GRPC: &GRPCConfig{
				Endpoint: defaultGRPCEndpoint,
				Insecure: true,
				Timeout:  defaultGRPCTimeout,
			},
		},
	}

	// An absent block is the documented way to take the defaults. Anything
	// else — a typo in the block, a value that does not decode — is a real
	// misconfiguration, and swallowing it into a default configuration is how
	// a service boots looking healthy on settings nobody asked for.
	if err := cfg.ParseProvider(ProviderName, c); err != nil && !errors.Is(err, fabric.ErrProviderNotFound) {
		return nil, fmt.Errorf("failed to parse %s config: %w", ProviderName, err)
	}

	if err := c.normalize(); err != nil {
		return nil, fmt.Errorf("invalid opentelemetry config: %w", err)
	}

	return c, nil
}
