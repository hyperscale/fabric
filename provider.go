package fabric

import (
	"cmp"
	"context"
	"slices"
)

// Priority controls the boot order of providers. Lower values start earlier and
// stop later: the shutdown order is the exact reverse of the boot order.
//
// Values are spaced by 100 so an application can slot its own providers between
// the well-known tiers, for example PriorityDatabase + 10 for a cache that must
// be available before the workers but after the database.
const (
	// PriorityTelemetry is for tracing, metric and logging providers. They start
	// first and stop last, so they observe the whole lifecycle of every other
	// provider.
	PriorityTelemetry = 100

	// PriorityDatabase is for datastores: SQL, Redis, object storage.
	PriorityDatabase = 200

	// PriorityBroker is for message brokers and in-process event buses.
	PriorityBroker = 300

	// PriorityWorker is for background workers and consumers.
	PriorityWorker = 400

	// PriorityServer is for network servers. They start last, so they only accept
	// traffic once every dependency is ready, and stop first, so they stop
	// accepting traffic before their dependencies go away.
	PriorityServer = 500

	// PriorityDefault is what a provider returns when it has no opinion.
	PriorityDefault = PriorityWorker
)

// BootableProvider is a component whose lifecycle is managed by a Service.
//
// A Service calls Start on every provider sequentially, in ascending Priority
// order, and Stop in the exact reverse of the order in which they started. A
// provider is expected to be usable from the moment its Start returns nil until
// its Stop is called.
type BootableProvider interface {
	// Name identifies the provider in logs, metrics and errors. It should match
	// the provider block name used in the HCL configuration. Two providers
	// registered on the same Service may not share a name.
	Name() string

	// Priority returns the boot order of the provider. See the Priority
	// constants.
	Priority() int

	// Start acquires the resources the provider needs and returns as soon as the
	// provider is ready to be used by the providers that start after it.
	//
	// Start must not block for the lifetime of the process. A provider that owns
	// a long-running loop implements RunnableProvider and serves from Run.
	//
	// ctx is the boot context; it is not valid after Start returns, so do not
	// retain it.
	//
	// A non-nil error aborts the boot: the providers that already started are
	// stopped in reverse order and Service.Start returns the error.
	Start(ctx context.Context) error

	// Stop releases the resources acquired by Start. ctx carries the shutdown
	// deadline shared by every provider, and Stop is expected to honor it.
	//
	// Stop is called at most once per Service, and only for a provider whose
	// Start returned nil.
	Stop(ctx context.Context) error
}

// RunnableProvider is an optional extension of BootableProvider for components
// that own a long-running loop, such as an HTTP server or a broker consumer.
//
// A Service calls Run in a dedicated goroutine once every provider has started,
// in the same ascending Priority order. Run must block until ctx is cancelled
// and then return. Returning before ctx is cancelled is treated as a fatal
// runtime error and triggers a graceful shutdown of the whole service.
//
// Once ctx is cancelled, returning nil or an error wrapping context.Canceled is
// the normal exit and is not reported as a failure.
//
// The rule is "Start acquires, Run serves": an HTTP provider calls net.Listen in
// Start, so that a port conflict is a fatal boot error caught before anything
// serves, and srv.Serve(ln) in Run.
type RunnableProvider interface {
	BootableProvider

	Run(ctx context.Context) error
}

// HealthChecker is an optional extension of BootableProvider for a provider that
// can report its own health, independently of the service-level readiness gate.
//
// A Service never calls Health; it exists for probe handlers that want to report
// per-dependency health.
type HealthChecker interface {
	// Health returns nil when the provider is healthy.
	Health(ctx context.Context) error
}

// order returns providers sorted by ascending priority.
//
// The sort is stable, so providers sharing a priority keep their registration
// order, which makes the boot order a documented total order rather than
// whatever the sort happened to produce. The result is a new slice: the
// registration order is never mutated, so the same Service can be inspected
// before and after a boot and two Services built from the same registrations
// always boot identically.
func order(providers []BootableProvider) []BootableProvider {
	sorted := slices.Clone(providers)

	slices.SortStableFunc(sorted, func(left, right BootableProvider) int {
		return cmp.Compare(left.Priority(), right.Priority())
	})

	return sorted
}
