package fabric

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shutdown is the exact reverse of the boot order.
func TestService_Shutdown_ReverseOrder(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	mustRegister(t, svc,
		newFakeProvider(rec, "server", PriorityServer),
		newFakeProvider(rec, "database", PriorityDatabase),
		newFakeProvider(rec, "telemetry", PriorityTelemetry),
	)

	require.NoError(t, svc.Start(t.Context()))
	require.NoError(t, svc.Shutdown(t.Context()))

	assert.Equal(t, []string{
		"telemetry:start",
		"database:start",
		"server:start",
		"server:stop",
		"server:stopped",
		"database:stop",
		"database:stopped",
		"telemetry:stop",
		"telemetry:stopped",
	}, rec.snapshot())
}

// Stop used to push a signal into a capacity-1 channel and return immediately:
// the caller could not observe completion and a second call deadlocked.
func TestService_Shutdown_Idempotent(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	mustRegister(t, svc, newFakeProvider(rec, "a", PriorityDefault))
	require.NoError(t, svc.Start(t.Context()))

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for range 3 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			err := svc.Shutdown(t.Context())

			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}()
	}

	wg.Wait()

	assert.Equal(t, 1, rec.count("a:stop"), "Stop must be called exactly once")
	assert.Len(t, errs, 3)

	for _, err := range errs {
		assert.NoError(t, err)
	}
}

// defer svc.Shutdown(ctx) must be safe even on a path where Start was never
// reached.
func TestService_Shutdown_BeforeStart(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	mustRegister(t, svc, newFakeProvider(rec, "a", PriorityDefault))

	require.NoError(t, svc.Shutdown(t.Context()))
	assert.Empty(t, rec.snapshot(), "nothing was started, nothing must be stopped")
}

// The drain must return within its budget even when a provider ignores its
// context entirely. The previous implementation built a 1s context and never
// passed it to Stop, so a wedged provider hung the process forever.
func TestService_Shutdown_HonorsDeadline(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	mustRegister(t, svc,
		newFakeProvider(rec, "telemetry", PriorityTelemetry),
		newWedgedProvider(t, rec, "wedged", PriorityServer),
	)

	require.NoError(t, svc.Start(t.Context()))

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := svc.Shutdown(ctx)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, 2*time.Second, "Shutdown did not return within its budget")

	// Once the budget is blown the remaining providers fail fast rather than
	// being silently skipped, so the error names them.
	assert.Contains(t, err.Error(), `"wedged"`)
	assert.Contains(t, err.Error(), `"telemetry"`)
}

func TestService_Shutdown_JoinsAllStopErrors(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	errA := errors.New("a failed")
	errB := errors.New("b failed")

	a := newFakeProvider(rec, "a", PriorityTelemetry)
	a.stopErr = errA
	b := newFakeProvider(rec, "b", PriorityServer)
	b.stopErr = errB

	mustRegister(t, svc, a, b)
	require.NoError(t, svc.Start(t.Context()))

	err := svc.Shutdown(t.Context())

	require.Error(t, err)
	assert.ErrorIs(t, err, errA)
	assert.ErrorIs(t, err, errB)
}

// A panicking Stop must not take the process down nor skip the providers that
// still have to be stopped.
func TestService_Shutdown_RecoversPanicInStop(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	panicking := newFakeProvider(rec, "panicking", PriorityServer)
	panicking.onStop = func(context.Context) { panic("boom") }

	mustRegister(t, svc,
		newFakeProvider(rec, "telemetry", PriorityTelemetry),
		panicking,
	)

	require.NoError(t, svc.Start(t.Context()))

	err := svc.Shutdown(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "panic")
	assert.True(t, rec.contains("telemetry:stopped"), "the remaining providers must still be stopped")
}

func TestService_Done_ClosedAfterShutdown(t *testing.T) {
	svc := testService(t)

	mustRegister(t, svc, newFakeProvider(newRecorder(), "a", PriorityDefault))
	require.NoError(t, svc.Start(t.Context()))

	select {
	case <-svc.Done():
		t.Fatal("Done closed before shutdown")
	default:
	}

	require.NoError(t, svc.Shutdown(t.Context()))

	select {
	case <-svc.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed after shutdown")
	}
}

func TestService_Err(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	stopErr := errors.New("stop failed")

	p := newFakeProvider(rec, "a", PriorityDefault)
	p.stopErr = stopErr

	mustRegister(t, svc, p)
	require.NoError(t, svc.Start(t.Context()))
	require.Error(t, svc.Shutdown(t.Context()))

	assert.ErrorIs(t, svc.Err(), stopErr)
}

// Stop receives the shutdown budget, not a context that is already dead.
func TestService_Shutdown_PassesLiveContextToStop(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	var deadline time.Time

	p := newFakeProvider(rec, "a", PriorityDefault)
	p.onStop = func(ctx context.Context) {
		require.NoError(t, ctx.Err(), "Stop received an already-cancelled context")

		deadline, _ = ctx.Deadline()
	}

	mustRegister(t, svc, p)
	require.NoError(t, svc.Start(t.Context()))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	require.NoError(t, svc.Shutdown(ctx))
	assert.False(t, deadline.IsZero(), "Stop received no deadline")
}
