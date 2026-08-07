package fabric

import (
	"fmt"
	"slices"
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
//
// The slice is deliberately larger than 12 elements. Below that threshold Go's
// pdqsort falls back to insertion sort, which is stable by accident, so a small
// fixture would pass even against slices.SortFunc and prove nothing.
func TestOrder_StableOnEqualPriority(t *testing.T) {
	rec := newRecorder()

	const n = 40

	want := make([]string, 0, n)
	providers := make([]BootableProvider, 0, n)

	for i := range n {
		name := fmt.Sprintf("p%02d", i)

		want = append(want, name)

		// Every provider shares a priority, which is the situation the shipped
		// providers were all in before the named tiers existed.
		providers = append(providers, newFakeProvider(rec, name, PriorityDefault))
	}

	// Repeated to catch an unstable sort that happens to be right once.
	for range 20 {
		assert.Equal(t, want, names(order(providers)))
	}
}

// Same guarantee, but with several priority tiers so the sort actually has to
// move elements around while preserving registration order within each tier.
func TestOrder_StableAcrossTiers(t *testing.T) {
	rec := newRecorder()

	tiers := []int{PriorityServer, PriorityTelemetry, PriorityDatabase}

	var (
		providers []BootableProvider
		perTier   = map[int][]string{}
	)

	for i := range 45 {
		tier := tiers[i%len(tiers)]
		name := fmt.Sprintf("p%02d", i)

		perTier[tier] = append(perTier[tier], name)
		providers = append(providers, newFakeProvider(rec, name, tier))
	}

	want := slices.Concat(
		perTier[PriorityTelemetry],
		perTier[PriorityDatabase],
		perTier[PriorityServer],
	)

	for range 20 {
		assert.Equal(t, want, names(order(providers)))
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
