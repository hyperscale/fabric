package fabric

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("boom")

// The boot order is priority first, registration order second. This is the
// contract downstream services write their own lifecycle tests against.
func TestService_Start_OrdersByPriorityThenRegistration(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	mustRegister(t, svc,
		newFakeProvider(rec, "server", PriorityServer),
		newFakeProvider(rec, "db-b", PriorityDatabase),
		newFakeProvider(rec, "telemetry", PriorityTelemetry),
		newFakeProvider(rec, "db-a", PriorityDatabase),
	)

	require.NoError(t, svc.Start(t.Context()))

	assert.Equal(t, []string{
		"telemetry:start",
		"db-b:start",
		"db-a:start",
		"server:start",
	}, rec.snapshot())
}

// Provider N+1 must not begin before provider N is ready. The previous
// implementation ran every Start in its own goroutine and called wg.Done before
// Start, so nothing was actually serialised.
func TestService_Start_IsSequential(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	var inflight, maxSeen atomic.Int32

	for _, name := range []string{"a", "b", "c", "d"} {
		p := newFakeProvider(rec, name, PriorityDefault)
		p.inflight = &inflight
		p.maxSeen = &maxSeen
		p.onStart = func(context.Context) { time.Sleep(2 * time.Millisecond) }

		mustRegister(t, svc, p)
	}

	require.NoError(t, svc.Start(t.Context()))

	assert.Equal(t, int32(1), maxSeen.Load(), "provider Start calls overlapped")
}

// The barrier seen from the inside: when b starts, a must already have finished.
func TestService_Start_BarrierWaitsForReady(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	a := newFakeProvider(rec, "a", PriorityTelemetry)
	b := newFakeProvider(rec, "b", PriorityDatabase)

	var seenFromB []string

	b.onStart = func(context.Context) { seenFromB = rec.snapshot() }

	mustRegister(t, svc, a, b)
	require.NoError(t, svc.Start(t.Context()))

	assert.Contains(t, seenFromB, "a:start")
}

// A failed Start is fatal: boot aborts, the providers that already started are
// unwound in reverse, and the error reaches the caller. It used to be logged and
// swallowed, leaving the process serving on a broken dependency.
func TestService_Start_FailureUnwindsInReverse(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	a := newFakeProvider(rec, "a", PriorityTelemetry)
	b := newFakeProvider(rec, "b", PriorityDatabase)
	b.startErr = errBoom
	c := newFakeProvider(rec, "c", PriorityServer)

	mustRegister(t, svc, a, b, c)

	err := svc.Start(t.Context())

	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), `"b"`)

	assert.Equal(t, []string{
		"a:start",
		"b:start",
		"b:start-failed",
		"a:stop",
		"a:stopped",
	}, rec.snapshot())

	assert.False(t, rec.contains("c:start"), "providers after the failure must not start")
	assert.False(t, rec.contains("b:stop"), "the failing provider must not be stopped")
}

// A panic in Start must unwind like any other boot failure. Left unrecovered it
// would kill the process at the panic frame, leaving every already-started
// provider open: no pool closed, no telemetry flushed.
func TestService_Start_PanicUnwindsInReverse(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	a := newFakeProvider(rec, "a", PriorityTelemetry)
	b := newFakeProvider(rec, "b", PriorityDatabase)
	b.onStart = func(context.Context) { panic("boom in Start") }
	c := newFakeProvider(rec, "c", PriorityServer)

	mustRegister(t, svc, a, b, c)

	var err error

	require.NotPanics(t, func() { err = svc.Start(t.Context()) })

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPanic)
	assert.Contains(t, err.Error(), "boom in Start")
	assert.Contains(t, err.Error(), `"b"`)

	assert.Equal(t, []string{
		"a:start",
		"b:start",
		"a:stop",
		"a:stopped",
	}, rec.snapshot())

	assert.False(t, rec.contains("c:start"))
	assert.False(t, svc.Ready())
}

// The point of paying for a recover is keeping the panic diagnosable, so the
// error must carry the stack of the panic site and not merely of the helper
// that recovered it.
func TestService_Start_PanicErrorCarriesStack(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	p := newFakeProvider(rec, "a", PriorityDefault)
	p.onStart = func(context.Context) { panicHere() }

	mustRegister(t, svc, p)

	err := svc.Start(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "panicHere", "the stack does not reach the panic site")
	assert.Contains(t, err.Error(), "service_start_test.go")
}

// panicHere gives the stack assertion above a distinctive frame to look for.
func panicHere() {
	panic("deliberate")
}

func TestService_Start_FailureLeavesServiceUnready(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	p := newFakeProvider(rec, "a", PriorityDefault)
	p.startErr = errBoom

	mustRegister(t, svc, p)

	require.Error(t, svc.Start(t.Context()))
	assert.False(t, svc.Ready())
}

func TestService_Start_FailureJoinsUnwindErrors(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	stopErr := errors.New("stop failed")

	a := newFakeProvider(rec, "a", PriorityTelemetry)
	a.stopErr = stopErr
	b := newFakeProvider(rec, "b", PriorityDatabase)
	b.startErr = errBoom

	mustRegister(t, svc, a, b)

	err := svc.Start(t.Context())

	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, err, stopErr)
}

func TestService_Start_Twice(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	mustRegister(t, svc, newFakeProvider(rec, "a", PriorityDefault))

	require.NoError(t, svc.Start(t.Context()))
	assert.ErrorIs(t, svc.Start(t.Context()), ErrAlreadyStarted)
	assert.Equal(t, 1, rec.count("a:start"))
}

func TestService_Start_AfterShutdown(t *testing.T) {
	svc := testService(t)

	mustRegister(t, svc, newFakeProvider(newRecorder(), "a", PriorityDefault))

	require.NoError(t, svc.Start(t.Context()))
	require.NoError(t, svc.Shutdown(t.Context()))
	assert.ErrorIs(t, svc.Start(t.Context()), ErrAlreadyStopped)
}

func TestService_Start_CancelledContextAborts(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	mustRegister(t, svc, newFakeProvider(rec, "a", PriorityDefault))

	err := svc.Start(ctx)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, rec.snapshot(), "no provider should have started")
}

func TestService_Register_AfterStart(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	mustRegister(t, svc, newFakeProvider(rec, "a", PriorityDefault))
	require.NoError(t, svc.Start(t.Context()))

	assert.ErrorIs(t, svc.Register(newFakeProvider(rec, "b", PriorityDefault)), ErrAlreadyStarted)
}

func TestService_Register_Duplicate(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	err := svc.Register(
		newFakeProvider(rec, "a", PriorityDefault),
		newFakeProvider(rec, "a", PriorityServer),
	)

	require.ErrorIs(t, err, ErrDuplicateProvider)
	assert.Contains(t, err.Error(), `"a"`)
}

func TestService_Register_Nil(t *testing.T) {
	assert.ErrorIs(t, testService(t).Register(nil), ErrNilProvider)
}

func TestService_Start_NoProviders(t *testing.T) {
	svc := testService(t)

	require.NoError(t, svc.Start(t.Context()))
	assert.True(t, svc.Ready())
	require.NoError(t, svc.Shutdown(t.Context()))
}

// The black-box form of the in-place-mutation bug: Service.Start used to sort
// and then reverse the registered slice through an alias, so a second service
// built from the same registrations booted in a different order.
func TestService_TwoServices_SameProviders_SameOrder(t *testing.T) {
	rec := newRecorder()

	providers := []BootableProvider{
		newFakeProvider(rec, "server", PriorityServer),
		newFakeProvider(rec, "telemetry", PriorityTelemetry),
		newFakeProvider(rec, "database", PriorityDatabase),
	}

	runCycle := func() []string {
		rec.mu.Lock()
		rec.events = nil
		rec.mu.Unlock()

		svc := testService(t)
		mustRegister(t, svc, providers...)
		require.NoError(t, svc.Start(t.Context()))
		require.NoError(t, svc.Shutdown(t.Context()))

		return rec.snapshot()
	}

	assert.Equal(t, runCycle(), runCycle())
}
