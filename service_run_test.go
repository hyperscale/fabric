package fabric

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_Run_ReturnsBootError(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	p := newFakeProvider(rec, "a", PriorityDefault)
	p.startErr = errBoom

	mustRegister(t, svc, p)

	assert.ErrorIs(t, svc.Run(t.Context()), errBoom)
}

func TestService_Run_DrainsOnContextCancel(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	mustRegister(t, svc,
		newFakeProvider(rec, "telemetry", PriorityTelemetry),
		newFakeProvider(rec, "server", PriorityServer),
	)

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() { done <- svc.Run(ctx) }()

	waitFor(t, func() bool { return svc.Ready() })

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}

	assert.Equal(t, []string{
		"telemetry:start",
		"server:start",
		"server:stop",
		"server:stopped",
		"telemetry:stop",
		"telemetry:stopped",
	}, rec.snapshot())
}

// Regression guard for the drain context. Run's shutdown budget must be derived
// with context.WithoutCancel: building it from the very context that SIGTERM
// just cancelled would hand every Stop an already-dead context, and the drain
// would complete instantly without draining anything.
func TestService_Run_DrainContextIsNotAlreadyCancelled(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	stopErrs := make(chan error, 1)

	p := newFakeProvider(rec, "a", PriorityDefault)
	p.onStop = func(ctx context.Context) { stopErrs <- ctx.Err() }

	mustRegister(t, svc, p)

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() { done <- svc.Run(ctx) }()

	waitFor(t, func() bool { return svc.Ready() })

	cancel()
	require.NoError(t, <-done)

	assert.NoError(t, <-stopErrs, "Stop received a context cancelled by the shutdown trigger")
}

// A RunnableProvider gets its Run called after every provider has started, and
// its context is cancelled before any Stop begins.
func TestService_Runnable_RunsThenStops(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	runnable := newFakeRunnable(rec, "server", PriorityServer)

	mustRegister(t, svc,
		newFakeProvider(rec, "database", PriorityDatabase),
		runnable,
	)

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() { done <- svc.Run(ctx) }()

	waitFor(t, func() bool { return rec.contains("server:run") })

	cancel()
	require.NoError(t, <-done)

	assert.Equal(t, []string{
		"database:start",
		"server:start",
		"server:run",
		"server:run-exit",
		"server:stop",
		"server:stopped",
		"database:stop",
		"database:stopped",
	}, rec.snapshot())
}

// A runnable that returns early is a fatal runtime error: the service shuts
// itself down rather than staying up with a dead component.
func TestService_Runnable_EarlyReturnTriggersShutdown(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	runnable := newFakeRunnable(rec, "server", PriorityServer)
	runnable.runFor = make(chan struct{})
	runnable.runErr = errBoom

	mustRegister(t, svc,
		newFakeProvider(rec, "database", PriorityDatabase),
		runnable,
	)

	done := make(chan error, 1)

	go func() { done <- svc.Run(t.Context()) }()

	waitFor(t, func() bool { return rec.contains("server:run") })

	close(runnable.runFor) // the server dies under the service

	select {
	case err := <-done:
		require.Error(t, err)
		assert.ErrorIs(t, err, errBoom)
	case <-time.After(5 * time.Second):
		t.Fatal("a runnable returning early did not trigger the shutdown")
	}

	assert.True(t, rec.contains("database:stopped"))
	assert.True(t, rec.contains("server:stopped"))
}

func TestService_Runnable_PanicIsRecovered(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	runnable := newFakeRunnable(rec, "server", PriorityServer)
	runnable.panicInRun = true

	mustRegister(t, svc, runnable)

	done := make(chan error, 1)

	go func() { done <- svc.Run(t.Context()) }()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "panic")
	case <-time.After(5 * time.Second):
		t.Fatal("a panicking Run did not trigger the shutdown")
	}

	assert.True(t, rec.contains("server:stopped"))
}

// A runnable exiting because its context was cancelled is the normal path and
// must not be reported as a failure.
func TestService_Runnable_ContextCanceledIsNotAnError(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	runnable := newFakeRunnable(rec, "server", PriorityServer)
	runnable.runErr = context.Canceled

	mustRegister(t, svc, runnable)

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() { done <- svc.Run(ctx) }()

	waitFor(t, func() bool { return rec.contains("server:run") })

	cancel()
	assert.NoError(t, <-done)
}

// Shutdown from another goroutine must unblock a Run that is waiting on its
// context.
func TestService_Run_UnblockedByShutdown(t *testing.T) {
	rec := newRecorder()
	svc := testService(t)

	mustRegister(t, svc, newFakeProvider(rec, "a", PriorityDefault))

	done := make(chan error, 1)

	go func() { done <- svc.Run(t.Context()) }()

	waitFor(t, func() bool { return svc.Ready() })

	require.NoError(t, svc.Shutdown(t.Context()))

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

func TestSignalContext(t *testing.T) {
	ctx, stop := SignalContext(t.Context())
	defer stop()

	require.NoError(t, ctx.Err())
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("condition not met within 5s")
}
