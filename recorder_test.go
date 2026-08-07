package fabric

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
)

// testService builds a Service with a quiet logger and the given options.
func testService(t *testing.T, opts ...ServiceOption) *Service {
	t.Helper()

	opts = append([]ServiceOption{
		WithName("test"),
		WithVersion("0.0.0"),
		WithLogger(slog.New(slog.DiscardHandler)),
	}, opts...)

	svc, err := NewService(opts...)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	return svc
}

func mustRegister(t *testing.T, svc *Service, providers ...BootableProvider) {
	t.Helper()

	if err := svc.Register(providers...); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

// recorder is an ordered, concurrency-safe event log. The lifecycle tests assert
// against an exact []string of its contents, which is what freezes the startup
// and shutdown partial order.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func newRecorder() *recorder {
	return &recorder{}
}

func (r *recorder) record(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, fmt.Sprintf(format, args...))
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]string, len(r.events))
	copy(out, r.events)

	return out
}

func (r *recorder) contains(event string) bool {
	for _, got := range r.snapshot() {
		if got == event {
			return true
		}
	}

	return false
}

// count returns how many times event was recorded, so a test can assert that a
// hook fired exactly once even under concurrent callers.
func (r *recorder) count(event string) int {
	n := 0

	for _, got := range r.snapshot() {
		if got == event {
			n++
		}
	}

	return n
}

// fakeProvider is a BootableProvider whose every lifecycle call is recorded and
// whose behaviour is fully configurable, so a test can express "this provider
// fails to start", "this one hangs in Stop" or "assert X from inside Start".
type fakeProvider struct {
	name     string
	priority int
	rec      *recorder

	startErr error
	stopErr  error

	// blockStop makes Stop block until the channel is closed. A nil channel
	// never unblocks, which is how the drain-deadline test wedges a provider.
	blockStop chan struct{}

	// onStart and onStop run inside Start and Stop, before the error is
	// returned, so a test can assert on the state of the world at that point.
	onStart func(ctx context.Context)
	onStop  func(ctx context.Context)

	// inflight is shared by every provider of a test and proves that Start calls
	// never overlap.
	inflight *atomic.Int32
	maxSeen  *atomic.Int32
}

func newFakeProvider(rec *recorder, name string, priority int) *fakeProvider {
	return &fakeProvider{name: name, priority: priority, rec: rec}
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Priority() int { return f.priority }

func (f *fakeProvider) Start(ctx context.Context) error {
	if f.inflight != nil {
		now := f.inflight.Add(1)
		defer f.inflight.Add(-1)

		for {
			prev := f.maxSeen.Load()
			if now <= prev || f.maxSeen.CompareAndSwap(prev, now) {
				break
			}
		}
	}

	f.rec.record("%s:start", f.name)

	if f.onStart != nil {
		f.onStart(ctx)
	}

	if f.startErr != nil {
		f.rec.record("%s:start-failed", f.name)

		return f.startErr
	}

	return nil
}

func (f *fakeProvider) Stop(ctx context.Context) error {
	f.rec.record("%s:stop", f.name)

	if f.onStop != nil {
		f.onStop(ctx)
	}

	if f.blockStop != nil {
		select {
		case <-f.blockStop:
		case <-ctx.Done():
			// Deliberately ignored: a provider that honors its context would
			// return here, but the point of the wedged-provider test is a
			// provider that does not. blockStop is closed by the test cleanup.
		}
	}

	if f.stopErr != nil {
		return f.stopErr
	}

	f.rec.record("%s:stopped", f.name)

	return nil
}

// wedgedProvider is a fakeProvider whose Stop ignores its context entirely. It
// exists to prove the service still returns within its shutdown budget.
type wedgedProvider struct {
	*fakeProvider

	release chan struct{}
}

func newWedgedProvider(t *testing.T, rec *recorder, name string, priority int) *wedgedProvider {
	t.Helper()

	w := &wedgedProvider{
		fakeProvider: newFakeProvider(rec, name, priority),
		release:      make(chan struct{}),
	}

	t.Cleanup(func() { close(w.release) })

	return w
}

func (w *wedgedProvider) Stop(_ context.Context) error {
	w.rec.record("%s:stop", w.name)

	<-w.release

	w.rec.record("%s:stopped", w.name)

	return nil
}

// fakeRunnable adds a long-running Run to fakeProvider.
type fakeRunnable struct {
	*fakeProvider

	runErr error

	// runFor, when non-nil, makes Run return as soon as it is closed instead of
	// waiting for its context, which simulates a server dying under the service.
	runFor chan struct{}

	panicInRun bool
}

func newFakeRunnable(rec *recorder, name string, priority int) *fakeRunnable {
	return &fakeRunnable{fakeProvider: newFakeProvider(rec, name, priority)}
}

func (f *fakeRunnable) Run(ctx context.Context) error {
	f.rec.record("%s:run", f.name)

	if f.panicInRun {
		panic("boom")
	}

	select {
	case <-ctx.Done():
		f.rec.record("%s:run-exit", f.name)

		return f.runErr
	case <-f.runFor:
		f.rec.record("%s:run-exit-early", f.name)

		return f.runErr
	}
}

