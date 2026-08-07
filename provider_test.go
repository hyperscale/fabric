package fabric

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func names(providers []BootableProvider) []string {
	out := make([]string, 0, len(providers))

	for _, provider := range providers {
		out = append(out, provider.Name())
	}

	return out
}

func TestOrder_ByPriority(t *testing.T) {
	rec := newRecorder()

	providers := []BootableProvider{
		newFakeProvider(rec, "server", PriorityServer),
		newFakeProvider(rec, "telemetry", PriorityTelemetry),
		newFakeProvider(rec, "worker", PriorityWorker),
		newFakeProvider(rec, "database", PriorityDatabase),
	}

	assert.Equal(t, []string{"telemetry", "database", "worker", "server"}, names(order(providers)))
}

// Equal priorities are the common case: without a stable sort the relative order
// of providers sharing a priority is whatever the sort happens to produce, and no
// lifecycle test can be written against it.
func TestOrder_StableOnEqualPriority(t *testing.T) {
	rec := newRecorder()

	providers := []BootableProvider{
		newFakeProvider(rec, "c", PriorityDatabase),
		newFakeProvider(rec, "a", PriorityTelemetry),
		newFakeProvider(rec, "b", PriorityTelemetry),
		newFakeProvider(rec, "d", PriorityDatabase),
	}

	// Repeated to catch an unstable sort that happens to be right once.
	for range 50 {
		assert.Equal(t, []string{"a", "b", "c", "d"}, names(order(providers)))
	}
}

// order must never reorder the caller's slice: Service.Start used to sort and
// then reverse s.providers in place, permanently corrupting the registration
// order for any later use.
func TestOrder_DoesNotMutateInput(t *testing.T) {
	rec := newRecorder()

	providers := []BootableProvider{
		newFakeProvider(rec, "server", PriorityServer),
		newFakeProvider(rec, "telemetry", PriorityTelemetry),
	}

	before := names(providers)

	sorted := order(providers)

	assert.Equal(t, before, names(providers), "order mutated its input")
	assert.NotEqual(t, before, names(sorted), "test is vacuous: input was already sorted")
}

func TestOrder_Empty(t *testing.T) {
	assert.Empty(t, order(nil))
}

// The tiers only mean something if they are ordered, and third-party providers
// slot themselves in with arithmetic such as PriorityDatabase + 10.
func TestPriorityConstants_AreOrdered(t *testing.T) {
	require.Less(t, PriorityTelemetry, PriorityDatabase)
	require.Less(t, PriorityDatabase, PriorityBroker)
	require.Less(t, PriorityBroker, PriorityWorker)
	require.Less(t, PriorityWorker, PriorityServer)
	assert.Equal(t, PriorityWorker, PriorityDefault)
}
