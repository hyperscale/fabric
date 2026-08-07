package fabric

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadiness_StartsUnready(t *testing.T) {
	assert.False(t, NewReadiness().Ready())
}

func TestReadiness_Set(t *testing.T) {
	r := NewReadiness()

	r.set(true)
	assert.True(t, r.Ready())

	r.set(false)
	assert.False(t, r.Ready())
}

func TestReadiness_Subscribe(t *testing.T) {
	r := NewReadiness()

	ch, unsubscribe := r.Subscribe()
	defer unsubscribe()

	r.set(true)
	assert.True(t, <-ch)

	r.set(false)
	assert.False(t, <-ch)
}

// A no-op set must not wake subscribers, otherwise a probe handler cannot tell a
// real transition from a redundant write.
func TestReadiness_SubscribeIgnoresNoOpSet(t *testing.T) {
	r := NewReadiness()

	ch, unsubscribe := r.Subscribe()
	defer unsubscribe()

	r.set(false) // already false

	select {
	case v := <-ch:
		t.Fatalf("unexpected notification: %v", v)
	default:
	}
}

func TestReadiness_Unsubscribe(t *testing.T) {
	r := NewReadiness()

	ch, unsubscribe := r.Subscribe()

	unsubscribe()
	unsubscribe() // must be idempotent

	_, open := <-ch
	assert.False(t, open, "channel should be closed after unsubscribe")

	r.set(true) // must not panic on a send to the closed channel
}

// A slow subscriber must never stall the lifecycle: the change is dropped and
// the setter returns.
func TestReadiness_SlowSubscriberDoesNotBlock(t *testing.T) {
	r := NewReadiness()

	_, unsubscribe := r.Subscribe()
	defer unsubscribe()

	r.set(true)  // fills the buffer, nobody reads
	r.set(false) // must not block
	r.set(true)

	assert.True(t, r.Ready())
}

// The full readiness contract, frozen: false before boot, true only once every
// provider started, and false again as the very first shutdown step, before
// anything is torn down.
func TestService_Readiness_Transitions(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	// The first provider stopped is the last one booted; readiness must already
	// be false by the time it is asked to stop.
	last := newFakeProvider(rec, "server", PriorityServer)

	var readyDuringStop bool

	last.onStop = func(context.Context) { readyDuringStop = svc.Ready() }

	// Readiness must still be false while the boot is in progress.
	var readyDuringStart bool

	last.onStart = func(context.Context) { readyDuringStart = svc.Ready() }

	mustRegister(t, svc, newFakeProvider(rec, "telemetry", PriorityTelemetry), last)

	assert.False(t, svc.Ready(), "a service must not be ready before it boots")

	require.NoError(t, svc.Start(t.Context()))
	assert.False(t, readyDuringStart, "readiness turned true mid-boot")
	assert.True(t, svc.Ready(), "a booted service must be ready")

	require.NoError(t, svc.Shutdown(t.Context()))
	assert.False(t, readyDuringStop, "readiness must be false before the first Stop")
	assert.False(t, svc.Ready())
}

// A probe provider built by wire receives the gate directly, since it is
// constructed before the Service exists.
func TestService_Readiness_UsesInjectedGate(t *testing.T) {
	gate := NewReadiness()
	svc := testService(t, WithReadiness(gate))

	mustRegister(t, svc, newFakeProvider(newRecorder(), "a", PriorityDefault))

	assert.False(t, gate.Ready())

	require.NoError(t, svc.Start(t.Context()))
	assert.True(t, gate.Ready())
	assert.Same(t, gate, svc.Readiness())

	require.NoError(t, svc.Shutdown(t.Context()))
	assert.False(t, gate.Ready())
}

func TestReadiness_ConcurrentAccess(t *testing.T) {
	r := NewReadiness()

	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(2)

		go func() { defer wg.Done(); r.set(i%2 == 0) }()
		go func() { defer wg.Done(); _ = r.Ready() }()
	}

	wg.Wait()

	require.NotPanics(t, func() { _ = r.Ready() })
}
