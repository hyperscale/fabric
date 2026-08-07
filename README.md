# Fabric

Fabric boots a set of providers in a deterministic order, supervises the
long-running ones, and drains them within a bounded budget.

```go
func run() error {
	ctx, stop := fabric.SignalContext(context.Background())
	defer stop()

	svc, err := fabric.NewService(
		fabric.WithName("acme-a-consumer"),
		fabric.WithVersion("0.0.1"),
		fabric.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	if err := svc.Register(traceProvider, dbProvider, httpProvider); err != nil {
		return err
	}

	return svc.Run(ctx)
}
```

## The provider contract

```go
type BootableProvider interface {
	Name() string
	Priority() int
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
```

**Start acquires, Run serves.** `Start` must return as soon as the provider is
ready to be used by whatever starts after it — it must never block for the
lifetime of the process. A provider that owns a long-running loop implements the
optional `RunnableProvider`:

```go
type RunnableProvider interface {
	BootableProvider

	Run(ctx context.Context) error
}
```

An HTTP provider therefore calls `net.Listen` in `Start`, so a port conflict is a
fatal boot error caught before anything serves, and `srv.Serve(ln)` in `Run`.
`Run` blocks until its context is canceled; returning early is treated as a fatal
runtime error and shuts the whole service down.

## Boot order

The order is a **documented total order**, safe to write tests against:

1. ascending `Priority()`;
2. ties broken by **registration order** (the sort is stable, and the registered
   slice is never mutated);
3. shutdown is the **exact reverse** of the order in which providers actually
   started.

Named tiers, spaced by 100 so you can slot your own between them (for example
`fabric.PriorityDatabase + 10`):

| Constant | Value | Starts | Stops |
| --- | --- | --- | --- |
| `PriorityTelemetry` | 100 | first | last |
| `PriorityDatabase` | 200 | | |
| `PriorityBroker` | 300 | | |
| `PriorityWorker` | 400 (`PriorityDefault`) | | |
| `PriorityServer` | 500 | last | first |

Telemetry starts first and stops last, so it observes the whole lifecycle of
everything else. Servers start last, so they only accept traffic once every
dependency is ready, and stop first, so they stop accepting traffic before their
dependencies go away.

## Lifecycle

`Start` runs providers **one at a time**: provider *N+1* begins only after
provider *N* returned. The first `Start` error is **fatal** — it aborts the boot,
stops the providers that already started in reverse order, and is returned to the
caller joined with any unwind error. The provider that failed is not stopped,
since it never started.

Shutdown, in order:

1. readiness turns **false**, before anything is torn down;
2. the pre-stop delay elapses, so a load balancer can observe the failing probe;
3. the run context is canceled and the `Run` goroutines are awaited;
4. every provider is stopped in reverse boot order.

Every step shares one budget. Once it expires, the remaining `Stop` calls return
immediately with `context.DeadlineExceeded` and are named in the joined error —
`Shutdown` is guaranteed to return by the deadline even against a provider that
ignores its context.

A provider that ignores its context also keeps its `Stop` goroutine alive after
the deadline: Fabric cannot kill it, so it is deliberately abandoned. That is
acceptable because a drain precedes process exit, but it means a test draining a
wedged provider should release it in a `t.Cleanup`.

A panic in `Start`, `Run` or `Stop` is recovered and converted into an error
wrapping `ErrPanic`, carrying the panic value and the stack of the panic site. A
panicking `Start` therefore unwinds like any other boot failure instead of
killing the process with every already-started provider left open.

| Method | Behavior |
| --- | --- |
| `Start(ctx)` | boots and returns; does **not** block for the process lifetime |
| `Run(ctx)` | `Start`, then block until `ctx` is canceled, `Shutdown` is called or a runnable returns, then drain |
| `Shutdown(ctx)` | blocking, idempotent, safe to call concurrently; a no-op before `Start` |
| `Ready()` | readiness state |
| `Done()` | closed once the drain finished |
| `Err()` | boot error joined with drain error |

A `Service` is **single-use**: once shut down it cannot be restarted.

**Fabric never installs signal handlers.** A library that owns process-wide
signals cannot be embedded in a process that already owns its own.
`fabric.SignalContext` is sugar over `signal.NotifyContext` so `main` stays short.

## Readiness

`Readiness` is false until every provider started, and false again as the *first*
shutdown step. It is a standalone value rather than a `Service` method, because
providers are constructed before the `Service` exists — a `/readyz` provider
could never be handed a `Service` accessor. Build it with `fabric.ReadinessSet`
and pass it to both the probe provider and `fabric.WithReadiness`.

Serve probes from a dedicated admin provider at `PriorityTelemetry + 1`, not from
the application server at `PriorityServer`: otherwise probes are unreachable
during most of the boot and all of the drain, exactly when they matter.

## Configuration

`ParseProvider` distinguishes an absent block from a malformed one, so an
optional block can fall back to defaults without swallowing a real
misconfiguration:

```go
if err := cfg.ParseProvider(name, c); err != nil && !errors.Is(err, fabric.ErrProviderNotFound) {
	return nil, err
}
```

`ErrProviderInvalid` wraps the underlying `hcl.Diagnostics`, still reachable with
`errors.As`.

The optional `service` block configures the drain:

```hcl
provider "service" {
  name             = "acme-a-consumer"
  version          = "0.0.1"
  shutdown_timeout = "30s"  # default
  pre_stop_delay   = "0s"   # default
}
```

The budget is global rather than per-provider, matching the single
`terminationGracePeriodSeconds` an orchestrator grants. A provider needing a
tighter cap layers a `context.WithTimeout` on the context it receives in `Stop`.

## Development

This repository is a Go workspace, so `./...` only matches the root module. Use
the Makefile, which resolves the module list from `go.work`:

```sh
make build
make test
make lint
```

## License

Fabric is licensed under [the MIT license](LICENSE.md).
