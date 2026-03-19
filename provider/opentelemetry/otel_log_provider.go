package otel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/euskadi31/wire"
	"github.com/hyperscale/fabric"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc/credentials"
)

const logProviderName = "otel.log"

var OTelLogSet = wire.NewSet(LogFactory, NewLogProvider)

func LogFactory(cfg *Config, resources *resource.Resource) (*sdklog.LoggerProvider, error) {
	if !cfg.Log.Enabled {
		return nil, nil
	}

	ctx := context.Background()

	var (
		exporter sdklog.Exporter
		err      error
	)

	switch cfg.Log.Exporter {
	case ExporterTypeGRPC:
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to create system cert pool: %w", err)
		}

		opts := []otlploggrpc.Option{
			otlploggrpc.WithEndpoint(cfg.Log.GRPC.Endpoint),
			otlploggrpc.WithTLSCredentials(credentials.NewTLS(&tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    certPool,
			})),
			otlploggrpc.WithTimeout(cfg.Log.GRPC.Timeout),
		}

		if cfg.Log.GRPC.Insecure {
			opts = append(opts, otlploggrpc.WithInsecure())
		}

		if cfg.Log.GRPC.Headers != nil {
			opts = append(opts, otlploggrpc.WithHeaders(cfg.Log.GRPC.Headers))
		}

		if cfg.Log.GRPC.Retry != nil && cfg.Log.GRPC.Retry.Enabled {
			opts = append(opts, otlploggrpc.WithRetry(otlploggrpc.RetryConfig{
				Enabled:         cfg.Log.GRPC.Retry.Enabled,
				InitialInterval: cfg.Log.GRPC.Retry.InitialInterval,
				MaxInterval:     cfg.Log.GRPC.Retry.MaxInterval,
				MaxElapsedTime:  cfg.Log.GRPC.Retry.MaxElapsedTime,
			}))
		}

		exporter, err = otlploggrpc.New(
			ctx,
			opts...,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP log exporter: %w", err)
		}
	case ExporterTypeStdout:
		opts := []stdoutlog.Option{}

		if cfg.Log.Stdout != nil && cfg.Log.Stdout.PrettyPrint {
			opts = append(opts, stdoutlog.WithPrettyPrint())
		}

		exporter, err = stdoutlog.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout log exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown log exporter type %s", cfg.Log.Exporter)
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(resources),
	)

	global.SetLoggerProvider(lp)

	return lp, nil
}

var _ fabric.BootableProvider = (*LogProvider)(nil)

type LogProvider struct {
	cfg *Config
	lp  *sdklog.LoggerProvider
}

func NewLogProvider(cfg *Config, lp *sdklog.LoggerProvider) *LogProvider {
	p := &LogProvider{
		cfg: cfg,
		lp:  lp,
	}

	return p
}

func (p *LogProvider) Name() string {
	return logProviderName
}

func (p *LogProvider) Priority() int {
	return 0
}

func (p *LogProvider) Start() error {
	return nil
}

func (p *LogProvider) Stop() error {
	if p.lp == nil {
		return nil // nothing to shutdown
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.ShutdownTimeout)
	defer cancel()

	// nolint: wrapcheck // not nessary
	return p.lp.Shutdown(ctx)
}
