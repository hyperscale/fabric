package otel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/euskadi31/wire"
	"github.com/hyperscale/fabric"
	otelpkg "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

const traceProviderName = "otel.trace"

var OTelTraceSet = wire.NewSet(TraceFactory, NewTraceProvider)

func TraceFactory(cfg *Config, resources *resource.Resource) (*sdktrace.TracerProvider, error) {
	if !cfg.Trace.Enabled {
		return nil, nil
	}

	ctx := context.Background()

	var (
		exporter sdktrace.SpanExporter
		err      error
	)

	switch cfg.Trace.Exporter {
	case ExporterTypeGRPC:
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to create system cert pool: %w", err)
		}

		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.Trace.GRPC.Endpoint),
			otlptracegrpc.WithTLSCredentials(credentials.NewTLS(&tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    certPool,
			})),
			otlptracegrpc.WithTimeout(cfg.Trace.GRPC.Timeout),
		}

		if cfg.Trace.GRPC.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}

		if cfg.Trace.GRPC.Headers != nil {
			opts = append(opts, otlptracegrpc.WithHeaders(cfg.Trace.GRPC.Headers))
		}

		if cfg.Trace.GRPC.Retry != nil && cfg.Trace.GRPC.Retry.Enabled {
			opts = append(opts, otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
				Enabled:         cfg.Trace.GRPC.Retry.Enabled,
				InitialInterval: cfg.Trace.GRPC.Retry.InitialInterval,
				MaxInterval:     cfg.Trace.GRPC.Retry.MaxInterval,
				MaxElapsedTime:  cfg.Trace.GRPC.Retry.MaxElapsedTime,
			}))
		}

		exporter, err = otlptrace.New(
			ctx,
			otlptracegrpc.NewClient(opts...),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
		}
	case ExporterTypeStdout:
		opts := []stdouttrace.Option{}

		if cfg.Trace.Stdout != nil && cfg.Trace.Stdout.PrettyPrint {
			opts = append(opts, stdouttrace.WithPrettyPrint())
		}

		exporter, err = stdouttrace.New(opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout trace exporter: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown trace exporter type %s", cfg.Trace.Exporter)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(cfg.Trace.BatchTimeout)),
		sdktrace.WithResource(resources),
	)

	otelpkg.SetTracerProvider(tp)

	return tp, nil
}

var _ fabric.BootableProvider = (*TraceProvider)(nil)

type TraceProvider struct {
	cfg *Config
	tp  *sdktrace.TracerProvider
}

func NewTraceProvider(cfg *Config, tp *sdktrace.TracerProvider) *TraceProvider {
	p := &TraceProvider{
		cfg: cfg,
		tp:  tp,
	}

	return p
}

func (p *TraceProvider) Name() string {
	return traceProviderName
}

func (p *TraceProvider) Priority() int {
	return 0
}

func (p *TraceProvider) Start() error {
	return nil
}

func (p *TraceProvider) Stop() error {
	if p.tp == nil {
		return nil // nothing to shutdown
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.ShutdownTimeout)
	defer cancel()

	// nolint: wrapcheck // not nessary
	return p.tp.Shutdown(ctx)
}
