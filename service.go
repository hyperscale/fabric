package fabric

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/exp/slog"
)

var _ ServiceLifeCycle = (*Service)(nil)

type ServiceOption func(*Service)

func Name(name string) ServiceOption {
	return func(s *Service) {
		s.name = name
	}
}

func Version(version string) ServiceOption {
	return func(s *Service) {
		s.version = version
	}
}

func Logger(logger *slog.Logger) ServiceOption {
	return func(s *Service) {
		s.logger = logger
	}
}

type ServiceLifeCycle interface {
	Register(provider BootableProvider)
	Start() error
	Stop() error
}

type Service struct {
	name        string
	version     string
	signal      chan os.Signal
	logger      *slog.Logger
	providers   []BootableProvider
	startMetric metric.Int64Histogram
	now         time.Time
}

func NewService(opts ...ServiceOption) (*Service, error) {
	s := &Service{
		signal: make(chan os.Signal, 1),
		now:    time.Now().UTC(),
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	for _, opt := range opts {
		opt(s)
	}

	startMetric, err := meter.Int64Histogram(
		"service.start.duration",
		metric.WithUnit("microseconds"),
		metric.WithDescription("Time to start the service"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create service.start.duration metric: %w", err)
	}

	s.startMetric = startMetric

	return s, nil
}

func (s *Service) Register(provider BootableProvider) {
	s.providers = append(s.providers, provider)
}

func (s *Service) Start() error {
	ctx := context.Background()

	logger := s.logger.With(slog.String("service.name", s.name), slog.String("service.version", s.version))

	logger.InfoContext(ctx, "Starting...")

	signal.Notify(s.signal, os.Interrupt, syscall.SIGTERM)

	bootables := s.providers

	By(func(left, right BootableProvider) bool {
		return left.Priority() < right.Priority()
	}).Sort(bootables)

	var wg sync.WaitGroup

	for _, provider := range bootables {
		wg.Add(1)

		go func(p BootableProvider) {
			sl := logger.With(slog.String("provider.name", p.Name()))

			sl.DebugContext(ctx, "Starting provider")

			wg.Done()

			if err := p.Start(); err != nil {
				sl.ErrorContext(ctx, "Start failed", slog.Any("error", err))
			}
		}(provider)

		wg.Wait()
	}

	s.startMetric.Record(
		ctx,
		time.Since(s.now).Microseconds(),
		metric.WithAttributeSet(
			attribute.NewSet(
				attribute.String("service.name", s.name),
				attribute.String("service.version", s.version),
			),
		),
	)

	logger.DebugContext(ctx, "Service started", slog.Duration("duration", time.Since(s.now)))

	<-s.signal

	// Reversing order for closing
	for i := len(bootables)/2 - 1; i >= 0; i-- {
		opp := len(bootables) - 1 - i
		bootables[i], bootables[opp] = bootables[opp], bootables[i]
	}

	logger.InfoContext(ctx, "Shutdown...")

	for _, provider := range bootables {
		sl := logger.With(slog.String("provider.name", provider.Name()))

		sl.DebugContext(ctx, "Stopping provider")

		if err := provider.Stop(); err != nil {
			sl.ErrorContext(ctx, "Stop failed", slog.Any("error", err))
		}
	}

	return nil
}

func (s *Service) Stop() error {
	s.signal <- syscall.SIGTERM

	return nil
}
