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

const defaultGRPCEndpoint = "localhost:4317"

type Config struct {
	Trace                 *TraceConfig  `hcl:"trace,block"`
	Metric                *MetricConfig `hcl:"metric,block"`
	Log                   *LogConfig    `hcl:"log,block"`
	ServiceName           string        `hcl:"service_name,optional"`
	ServiceVersion        string        `hcl:"service_version,optional"`
	DeploymentEnvironment string        `hcl:"deployment_environment,optional"` // optional deployment environment
	ShutdownTimeout       time.Duration `hcl:"shutdown_timeout,optional"`       // default: 5s
}

type MetricConfig struct {
	Enabled  bool          `hcl:"enabled,optional"`
	Interval time.Duration `hcl:"interval,optional"` // default: 10s
	Exporter ExporterType  `hcl:"exporter,optional"` // grpc, stdout
	GRPC     *GRPCConfig   `hcl:"grpc,block"`
	Stdout   *StdoutConfig `hcl:"stdout,block"` // stdout configuration
}

type TraceConfig struct {
	Enabled      bool          `hcl:"enabled,optional"`
	BatchTimeout time.Duration `hcl:"batch_timeout,optional"` // default: 5s
	Exporter     ExporterType  `hcl:"exporter,optional"`      // grpc, stdout
	GRPC         *GRPCConfig   `hcl:"grpc,block"`
	Stdout       *StdoutConfig `hcl:"stdout,block"` // stdout configuration
}

type LogConfig struct {
	Enabled  bool          `hcl:"enabled,optional"`
	Exporter ExporterType  `hcl:"exporter,optional"` // grpc, stdout
	GRPC     *GRPCConfig   `hcl:"grpc,block"`
	Stdout   *StdoutConfig `hcl:"stdout,block"` // stdout configuration
}

type StdoutConfig struct {
	PrettyPrint bool `hcl:"pretty_print,optional"` // pretty print output
}

type GRPCConfig struct {
	Endpoint string            `hcl:"endpoint"`
	Insecure bool              `hcl:"insecure"`         // use insecure connection
	Headers  map[string]string `hcl:"headers,optional"` // additional headers to send with the request
	Retry    *RetryConfig      `hcl:"retry,block"`      // retry configuration
	Timeout  time.Duration     `hcl:"timeout,optional"` // timeout for gRPC requests, default: 10s
}

type RetryConfig struct {
	Enabled         bool          `hcl:"enabled,optional"`          // enable retry
	InitialInterval time.Duration `hcl:"initial_interval,optional"` // initial interval for retry
	MaxInterval     time.Duration `hcl:"max_interval,optional"`     // maximum interval for retry
	MaxElapsedTime  time.Duration `hcl:"max_elapsed_time,optional"` // maximum elapsed time for retry
}

func ConfigProvider(cfg *fabric.Configuration) (*Config, error) {
	serviceName := filepath.Base(os.Args[0])

	c := &Config{
		ServiceName:           serviceName,
		DeploymentEnvironment: "local",
		ShutdownTimeout:       5 * time.Second, // default timeout for exporters
		Trace: &TraceConfig{
			Enabled:      false,
			Exporter:     ExporterTypeStdout,
			BatchTimeout: 5 * time.Second, // default batch timeout
			GRPC: &GRPCConfig{
				Endpoint: defaultGRPCEndpoint,
				Insecure: true,
				Timeout:  10 * time.Second, // default timeout
			},
		},
		Metric: &MetricConfig{
			Enabled:  false,
			Exporter: ExporterTypeStdout,
			Interval: 10 * time.Second, // default interval
			GRPC: &GRPCConfig{
				Endpoint: defaultGRPCEndpoint,
				Insecure: true,
				Timeout:  10 * time.Second, // default timeout
			},
		},
		Log: &LogConfig{
			Enabled:  true,
			Exporter: ExporterTypeStdout,
			GRPC: &GRPCConfig{
				Endpoint: defaultGRPCEndpoint,
				Insecure: true,
				Timeout:  10 * time.Second, // default timeout
			},
		},
	}

	// An absent block keeps the defaults above; a present but malformed one is
	// fatal, so a typo cannot silently degrade to the default configuration.
	if err := cfg.ParseProvider(ProviderName, c); err != nil && !errors.Is(err, fabric.ErrProviderNotFound) {
		return nil, fmt.Errorf("failed to parse %s config: %w", ProviderName, err)
	}

	return c, nil
}
