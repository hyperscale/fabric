package otel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/euskadi31/wire"
	"github.com/hyperscale/fabric"
	otelpkg "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc/credentials"
)

const metricProviderName = "otel.metric"

var OTelMetricSet = wire.NewSet(MetricFactory, NewMetricProvider)

func MetricFactory(cfg *Config, resources *resource.Resource) (*sdkmetric.MeterProvider, error) {
	if !cfg.Metric.Enabled {
		return nil, nil
	}

	ctx := context.Background()

	var (
		exporter sdkmetric.Exporter
		err      error
	)

	switch cfg.Metric.Exporter {
	case ExporterTypeGRPC:
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to create system cert pool: %w", err)
		}

		opts := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithEndpoint(cfg.Metric.GRPC.Endpoint),
			otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(&tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    certPool,
			})),
			otlpmetricgrpc.WithTimeout(cfg.Metric.GRPC.Timeout),
			// otlpmetricgrpc.WithReconnectionPeriod(cfg.Metric.GRPC.ReconnectionPeriod),
		}

		if cfg.Metric.GRPC.Insecure {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}

		if cfg.Metric.GRPC.Headers != nil {
			opts = append(opts, otlpmetricgrpc.WithHeaders(cfg.Metric.GRPC.Headers))
		}

		if cfg.Metric.GRPC.Retry != nil && cfg.Metric.GRPC.Retry.Enabled {
			opts = append(opts, otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{
				Enabled:         cfg.Metric.GRPC.Retry.Enabled,
				InitialInterval: cfg.Metric.GRPC.Retry.InitialInterval,
				MaxInterval:     cfg.Metric.GRPC.Retry.MaxInterval,
				MaxElapsedTime:  cfg.Metric.GRPC.Retry.MaxElapsedTime,
			}))
		}

		exporter, err = otlpmetricgrpc.New(
			ctx,
			opts...,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
		}
	case ExporterTypeStdout:
		opts := []stdoutmetric.Option{}

		if cfg.Metric.Stdout != nil && cfg.Metric.Stdout.PrettyPrint {
			opts = append(opts, stdoutmetric.WithPrettyPrint())
		}

		exporter, err = stdoutmetric.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout metric exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown metric exporter type %s", cfg.Metric.Exporter)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(resources),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(
			exporter,
			sdkmetric.WithInterval(cfg.Metric.Interval),
			sdkmetric.WithTimeout(cfg.Metric.GRPC.Timeout),
		)),
	)

	otelpkg.SetMeterProvider(mp)

	return mp, nil
}

var _ fabric.BootableProvider = (*MetricProvider)(nil)

type MetricProvider struct {
	cfg *Config
	mp  *sdkmetric.MeterProvider
}

func NewMetricProvider(cfg *Config, mp *sdkmetric.MeterProvider) *MetricProvider {
	p := &MetricProvider{
		cfg: cfg,
		mp:  mp,
	}

	return p
}

func (p *MetricProvider) Name() string {
	return metricProviderName
}

func (p *MetricProvider) Priority() int {
	return 0
}

func (p *MetricProvider) Start() error {
	return nil
}

func (p *MetricProvider) Stop() error {
	if p.mp == nil {
		return nil // nothing to shutdown
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.ShutdownTimeout)
	defer cancel()

	// nolint: wrapcheck // not nessary
	return p.mp.Shutdown(ctx)
}
