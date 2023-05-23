package fabric

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/instrument"
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

func Logger(logger *zerolog.Logger) ServiceOption {
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
	logger      *zerolog.Logger
	providers   []BootableProvider
	startMetric instrument.Int64Histogram
	now         time.Time
}

func NewService(opts ...ServiceOption) (*Service, error) {
	s := &Service{
		signal: make(chan os.Signal, 1),
		now:    time.Now().UTC(),
	}

	for _, opt := range opts {
		opt(s)
	}

	startMetric, err := meter.Int64Histogram(
		"service.start.duration",
		instrument.WithUnit("microseconds"),
		instrument.WithDescription("Time to start the service"),
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

	logger := s.logger.With().Str("service.name", s.name).Str("service.version", s.version).Logger()

	logger.Info().Msg("Starting...")

	signal.Notify(s.signal, os.Interrupt, syscall.SIGTERM)

	bootables := s.providers

	By(func(left, right BootableProvider) bool {
		return left.Priority() < right.Priority()
	}).Sort(bootables)

	var wg sync.WaitGroup

	for _, provider := range bootables {
		wg.Add(1)

		go func(p BootableProvider) {
			slog := logger.With().Str("provider.name", p.Name()).Logger()

			slog.Debug().Msg("Starting provider")

			wg.Done()

			if err := p.Start(); err != nil {
				slog.Error().Err(err).Msg("Start failed")
			}
		}(provider)

		wg.Wait()
	}

	s.startMetric.Record(
		ctx,
		time.Since(s.now).Microseconds(),
		attribute.String("service.name", s.name),
		attribute.String("service.version", s.version),
	)

	logger.Debug().Dur("duration", time.Since(s.now)).Msg("Service started")

	<-s.signal

	// Reversing order for closing
	for i := len(bootables)/2 - 1; i >= 0; i-- {
		opp := len(bootables) - 1 - i
		bootables[i], bootables[opp] = bootables[opp], bootables[i]
	}

	logger.Info().Msg("Shutdown...")

	for _, provider := range bootables {
		slog := logger.With().Str("provider.name", provider.Name()).Logger()

		slog.Debug().Msg("Stopping provider")

		if err := provider.Stop(); err != nil {
			slog.Error().Err(err).Msg("Stop failed")
		}
	}

	return nil
}

func (s *Service) Stop() error {
	s.signal <- syscall.SIGTERM

	return nil
}
