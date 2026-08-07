package fabric

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// DefaultShutdownTimeout bounds the whole drain when no other value is
// configured. It matches the usual Kubernetes terminationGracePeriodSeconds.
const DefaultShutdownTimeout = 30 * time.Second

var (
	// ErrAlreadyStarted is returned by Start when the service is already
	// starting or running, and by Register after the service has started.
	ErrAlreadyStarted = errors.New("fabric: service already started")

	// ErrAlreadyStopped is returned by Start after the service has been shut
	// down. A Service is single-use and cannot be restarted.
	ErrAlreadyStopped = errors.New("fabric: service already stopped")

	// ErrNilProvider is returned by Register for a nil provider.
	ErrNilProvider = errors.New("fabric: nil provider")

	// ErrDuplicateProvider is returned by Register when two providers share a
	// name, which would otherwise make logs ambiguous and let the same resource
	// be stopped twice.
	ErrDuplicateProvider = errors.New("fabric: duplicate provider")
)

var _ ServiceLifeCycle = (*Service)(nil)

// ServiceLifeCycle is the contract a Service offers to its embedder.
type ServiceLifeCycle interface {
	Register(providers ...BootableProvider) error
	Start(ctx context.Context) error
	Run(ctx context.Context) error
	Shutdown(ctx context.Context) error
	Ready() bool
	Done() <-chan struct{}
	Err() error
}

type state int

const (
	stateNew state = iota
	stateStarting
	stateRunning
	stateStopping
	stateStopped
)

type ServiceOption func(*Service)

// WithName sets the service name reported in logs and metrics.
func WithName(name string) ServiceOption {
	return func(s *Service) {
		s.name = name
	}
}

// WithVersion sets the service version reported in logs and metrics.
func WithVersion(version string) ServiceOption {
	return func(s *Service) {
		s.version = version
	}
}

// WithLogger sets the logger used for lifecycle events.
func WithLogger(logger *slog.Logger) ServiceOption {
	return func(s *Service) {
		s.logger = logger
	}
}

// WithShutdownTimeout bounds the whole drain: the pre-stop delay, waiting for
// the runnable providers and every Stop call share this single budget. It
// defaults to DefaultShutdownTimeout.
//
// The budget is global rather than per-provider because the orchestrator that
// kills the process also grants a single global grace period. A provider that
// needs its own, shorter cap layers a context.WithTimeout on the context it
// receives in Stop.
func WithShutdownTimeout(d time.Duration) ServiceOption {
	return func(s *Service) {
		s.shutdownTimeout = d
	}
}

// WithPreStopDelay sets how long the shutdown waits after turning readiness off
// and before tearing anything down, so a load balancer has time to observe the
// failing probe and stop routing to this instance. It defaults to zero.
func WithPreStopDelay(d time.Duration) ServiceOption {
	return func(s *Service) {
		s.preStopDelay = d
	}
}

// WithReadiness makes the service drive an externally provided readiness gate,
// typically the one built by ReadinessSet and shared with a probe provider.
func WithReadiness(r *Readiness) ServiceOption {
	return func(s *Service) {
		if r != nil {
			s.readiness = r
		}
	}
}

// Service boots a set of providers in a deterministic order, supervises the
// long-running ones and drains them within a bounded budget.
//
// A Service is single-use: once shut down it cannot be started again.
type Service struct {
	name    string
	version string
	logger  *slog.Logger

	shutdownTimeout time.Duration
	preStopDelay    time.Duration
	readiness       *Readiness

	mu        sync.Mutex
	state     state
	providers []BootableProvider // registration order, never reordered
	names     map[string]struct{}
	booted    []BootableProvider // boot order, only providers whose Start returned nil
	bootErr   error
	drainErr  error

	runCtx    context.Context //nolint:containedctx // lifetime of the run phase, not of a call
	runCancel context.CancelFunc
	runWG     sync.WaitGroup
	runErrMu  sync.Mutex
	runErrs   []error

	halt     chan struct{} // closed when a shutdown must begin
	haltOnce sync.Once

	done      chan struct{} // closed when the drain has finished
	drainOnce sync.Once

	startMetric metric.Int64Histogram
	now         time.Time
}

func NewService(opts ...ServiceOption) (*Service, error) {
	s := &Service{
		logger:          slog.New(slog.NewTextHandler(os.Stdout, nil)),
		shutdownTimeout: DefaultShutdownTimeout,
		readiness:       NewReadiness(),
		names:           make(map[string]struct{}),
		halt:            make(chan struct{}),
		done:            make(chan struct{}),
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

// SignalContext returns a copy of parent that is canceled on SIGINT or SIGTERM.
//
// Fabric never installs process-wide signal handlers itself, so that a Service
// can be embedded in a process that already owns its signals. This helper only
// exists so a main function does not have to import os/signal and syscall.
func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

// Register adds providers to the service, in the order given.
//
// Registration order is what breaks ties between providers sharing a priority,
// so it is part of the boot order contract. Registering after Start returns
// ErrAlreadyStarted.
func (s *Service) Register(providers ...BootableProvider) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != stateNew {
		return ErrAlreadyStarted
	}

	for _, provider := range providers {
		if provider == nil {
			return ErrNilProvider
		}

		name := provider.Name()

		if _, exists := s.names[name]; exists {
			return fmt.Errorf("%w: %q", ErrDuplicateProvider, name)
		}

		s.names[name] = struct{}{}
		s.providers = append(s.providers, provider)
	}

	return nil
}

// Start boots every registered provider sequentially, in ascending priority
// order with registration order breaking ties, and returns once the last one is
// ready. It does not block for the lifetime of the process; use Run for that.
//
// Each provider's Start is awaited before the next one begins, so a provider can
// rely on everything ahead of it in the order being usable.
//
// If a provider fails to start, the providers that already started are stopped
// in reverse order and the error is returned, joined with any error raised while
// unwinding. The service is then stopped and cannot be started again.
//
// On success the readiness gate turns true and the RunnableProvider goroutines
// are launched.
func (s *Service) Start(ctx context.Context) error {
	boot, err := s.beginStart(ctx)
	if err != nil {
		return err
	}

	logger := s.log()

	logger.InfoContext(ctx, "Starting...")

	for _, provider := range boot {
		if err := s.startOne(ctx, logger, provider); err != nil {
			return s.abort(ctx, err)
		}
	}

	s.runRunnables(boot)
	s.setState(stateRunning)
	s.readiness.set(true)
	s.recordStartDuration(ctx)

	logger.InfoContext(ctx, "Service started", slog.Duration("duration", time.Since(s.now)))

	return nil
}

func (s *Service) startOne(ctx context.Context, logger *slog.Logger, provider BootableProvider) error {
	pl := logger.With(slog.String("provider.name", provider.Name()))

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("boot aborted before provider %q: %w", provider.Name(), err)
	}

	pl.DebugContext(ctx, "Starting provider")

	if err := provider.Start(ctx); err != nil {
		return fmt.Errorf("start provider %q: %w", provider.Name(), err)
	}

	s.mu.Lock()
	s.booted = append(s.booted, provider)
	s.mu.Unlock()

	pl.DebugContext(ctx, "Provider started")

	return nil
}

func (s *Service) beginStart(ctx context.Context) ([]BootableProvider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.state {
	case stateNew:
	case stateStarting, stateRunning:
		return nil, ErrAlreadyStarted
	case stateStopping, stateStopped:
		return nil, ErrAlreadyStopped
	}

	s.state = stateStarting
	s.now = time.Now()

	// The run context deliberately outlives the boot context: canceling the
	// context passed to Start must not kill the long-running providers, which
	// are only stopped by the drain.
	s.runCtx, s.runCancel = context.WithCancel(context.WithoutCancel(ctx))

	return order(s.providers), nil
}

// abort unwinds a failed boot: the providers that already started are stopped in
// reverse order and their errors are joined to cause.
func (s *Service) abort(ctx context.Context, cause error) error {
	s.mu.Lock()
	s.bootErr = cause
	s.mu.Unlock()

	// WithoutCancel: the boot context may itself be the reason the boot failed,
	// and a drain context that is already canceled would make every Stop return
	// instantly.
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
	defer cancel()

	return errors.Join(cause, s.shutdown(drainCtx))
}

// Run starts the service, blocks until ctx is canceled, Shutdown is called or a
// runnable provider returns, then drains and returns.
//
// Run never installs signal handlers. Pass a context from SignalContext, or from
// signal.NotifyContext, to shut down on SIGINT and SIGTERM.
func (s *Service) Run(ctx context.Context) error {
	if err := s.Start(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
	case <-s.halt:
	}

	// WithoutCancel: the drain must survive the very cancellation that triggered
	// it, otherwise every Stop would immediately see context.Canceled.
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
	defer cancel()

	return s.shutdown(drainCtx)
}

// Shutdown drains the service and returns once every provider has stopped or the
// budget carried by ctx has expired.
//
// It is safe to call concurrently and more than once: the drain runs once and
// every caller observes the same joined error. Calling Shutdown before Start is
// a no-op returning nil, so `defer svc.Shutdown(ctx)` is always safe.
//
// ctx bounds the whole drain, not each provider: once it expires the remaining
// Stop calls return immediately with ctx.Err().
func (s *Service) Shutdown(ctx context.Context) error {
	return s.shutdown(ctx)
}

func (s *Service) shutdown(ctx context.Context) error {
	s.triggerHalt()

	s.drainOnce.Do(func() {
		err := s.drain(ctx)

		s.mu.Lock()
		s.drainErr = err
		s.mu.Unlock()

		close(s.done)
	})

	<-s.done

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.drainErr
}

func (s *Service) drain(ctx context.Context) error {
	s.mu.Lock()

	if s.state == stateNew {
		s.state = stateStopped // Shutdown before Start: nothing to drain.
		s.mu.Unlock()

		return nil
	}

	s.state = stateStopping
	booted := slices.Clone(s.booted)
	cancelRun := s.runCancel
	s.mu.Unlock()

	logger := s.log()

	// Step 1: stop advertising readiness before anything is torn down, so load
	// balancers take this instance out of rotation while it can still serve.
	s.readiness.set(false)

	logger.InfoContext(ctx, "Shutdown...")

	// Step 2: give the infrastructure time to observe the readiness change.
	s.waitPreStop(ctx)

	// Step 3: ask the long-running providers to return, and wait within budget.
	if cancelRun != nil {
		cancelRun()
	}

	errs := make([]error, 0, len(booted)+1)

	if err := s.waitRunnables(ctx); err != nil {
		errs = append(errs, err)
	}

	errs = append(errs, s.runErrors()...)
	errs = append(errs, s.stopAll(ctx, logger, booted)...)

	s.setState(stateStopped)

	logger.InfoContext(ctx, "Service stopped", slog.Int("errors", len(errs)))

	return errors.Join(errs...)
}

// stopAll stops the providers in the exact reverse of the boot order.
func (s *Service) stopAll(ctx context.Context, logger *slog.Logger, booted []BootableProvider) []error {
	errs := make([]error, 0, len(booted))

	for i := len(booted) - 1; i >= 0; i-- {
		provider := booted[i]

		pl := logger.With(slog.String("provider.name", provider.Name()))

		pl.DebugContext(ctx, "Stopping provider")

		if err := stopWithin(ctx, provider); err != nil {
			pl.ErrorContext(ctx, "Stop failed", slog.Any("error", err))

			errs = append(errs, err)
		}
	}

	return errs
}

func (s *Service) waitPreStop(ctx context.Context) {
	if s.preStopDelay <= 0 {
		return
	}

	timer := time.NewTimer(s.preStopDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

// waitRunnables waits for every RunnableProvider goroutine to return, bounded by
// the shutdown budget.
func (s *Service) waitRunnables(ctx context.Context) error {
	stopped := make(chan struct{})

	go func() {
		s.runWG.Wait()

		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for runnable providers: %w", ctx.Err())
	}
}

// stopWithin calls Stop and guarantees a return by the deadline carried by ctx,
// so a wedged provider cannot hold the drain hostage.
//
// The goroutine of a provider that ignores its context is deliberately leaked:
// the process is about to exit, and a bounded drain is worth more than a clean
// goroutine count.
func stopWithin(ctx context.Context, provider BootableProvider) error {
	done := make(chan error, 1)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				done <- fmt.Errorf("stop provider %q: panic: %v", provider.Name(), rec)
			}
		}()

		if err := provider.Stop(ctx); err != nil {
			done <- fmt.Errorf("stop provider %q: %w", provider.Name(), err)

			return
		}

		done <- nil
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("stop provider %q: %w", provider.Name(), ctx.Err())
	}
}

// runRunnables launches the Run goroutine of every RunnableProvider, in boot
// order, once every provider has started.
//
// Starting them only after the whole boot succeeded is deliberate: launching
// each Run right after its own Start would race a runtime failure against an
// in-progress boot and make the outcome non-deterministic.
func (s *Service) runRunnables(boot []BootableProvider) {
	logger := s.log()

	for _, provider := range boot {
		runnable, ok := provider.(RunnableProvider)
		if !ok {
			continue
		}

		s.runWG.Add(1)

		go func() {
			defer s.runWG.Done()

			pl := logger.With(slog.String("provider.name", runnable.Name()))

			pl.DebugContext(s.runCtx, "Running provider")

			if err := safeRun(s.runCtx, runnable); err != nil && !errors.Is(err, context.Canceled) {
				pl.ErrorContext(s.runCtx, "Run failed", slog.Any("error", err))

				s.recordRunError(fmt.Errorf("run provider %q: %w", runnable.Name(), err))
			}

			// A runnable that returns is fatal: unless the service is already
			// stopping, this is what starts the shutdown.
			s.triggerHalt()
		}()
	}
}

func safeRun(ctx context.Context, runnable RunnableProvider) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic: %v", rec)
		}
	}()

	// nolint: wrapcheck // the caller wraps with the provider name
	return runnable.Run(ctx)
}

func (s *Service) recordRunError(err error) {
	s.runErrMu.Lock()
	defer s.runErrMu.Unlock()

	s.runErrs = append(s.runErrs, err)
}

func (s *Service) runErrors() []error {
	s.runErrMu.Lock()
	defer s.runErrMu.Unlock()

	return slices.Clone(s.runErrs)
}

func (s *Service) triggerHalt() {
	s.haltOnce.Do(func() {
		close(s.halt)
	})
}

// Ready reports whether the service is ready to serve traffic.
func (s *Service) Ready() bool {
	return s.readiness.Ready()
}

// Readiness returns the gate the service drives, for embedders that do not build
// it through wire.
func (s *Service) Readiness() *Readiness {
	return s.readiness
}

// Done is closed once the drain has finished. It never closes if the service is
// never started nor shut down.
func (s *Service) Done() <-chan struct{} {
	return s.done
}

// Err returns the terminal error of the service: the boot error joined with the
// drain error. It is only meaningful once Done is closed.
func (s *Service) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return errors.Join(s.bootErr, s.drainErr)
}

func (s *Service) setState(next state) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = next
}

func (s *Service) log() *slog.Logger {
	return s.logger.With(
		slog.String("service.name", s.name),
		slog.String("service.version", s.version),
	)
}

func (s *Service) recordStartDuration(ctx context.Context) {
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
}
