package fabric

import (
	"sync"

	"github.com/euskadi31/wire"
)

// ReadinessSet provides a *Readiness to the wire graph.
//
// Readiness is a standalone node rather than a method on Service on purpose:
// providers are constructed before the Service exists, so a provider that serves
// a /readyz probe could never be handed the Service's own accessor. Both the
// Service and the probe provider depend on this node instead.
var ReadinessSet = wire.NewSet(NewReadiness)

// Readiness is the service-level readiness gate.
//
// It reports false until every provider has started, and turns false again as
// the very first step of the shutdown, before anything is torn down, so that a
// load balancer can take the instance out of rotation while it can still serve
// in-flight requests.
//
// Only a Service mutates it; every other holder is a reader. It is safe for
// concurrent use.
type Readiness struct {
	mu     sync.RWMutex
	ready  bool
	nextID int
	subs   map[int]chan bool
}

func NewReadiness() *Readiness {
	return &Readiness{
		subs: make(map[int]chan bool),
	}
}

// Ready reports whether the service is ready to serve traffic.
func (r *Readiness) Ready() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.ready
}

// Subscribe returns a channel receiving every readiness change, and a function
// that unsubscribes and closes it. The returned function must be called to
// release the subscription.
//
// The channel is buffered; if a subscriber is not keeping up, a change is
// dropped rather than blocking the service lifecycle. Subscribers that need the
// authoritative value should call Ready after a wake-up.
func (r *Readiness) Subscribe() (<-chan bool, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := r.nextID
	r.nextID++

	ch := make(chan bool, 1)
	r.subs[id] = ch

	return ch, func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		if sub, ok := r.subs[id]; ok {
			delete(r.subs, id)
			close(sub)
		}
	}
}

// set records a readiness change and notifies the subscribers. It is unexported
// so that the package boundary keeps Service the only writer.
func (r *Readiness) set(ready bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ready == ready {
		return
	}

	r.ready = ready

	for _, sub := range r.subs {
		select {
		case sub <- ready:
		default: // slow subscriber: drop rather than stall the lifecycle
		}
	}
}
