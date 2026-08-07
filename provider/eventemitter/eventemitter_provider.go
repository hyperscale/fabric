package eventemitter

import (
	"context"
	"fmt"

	"github.com/euskadi31/go-eventemitter"
	"github.com/euskadi31/wire"
	"github.com/hyperscale/fabric"
)

const (
	providerName = "eventemitter"
)

var Set = wire.NewSet(Factory, wire.Bind(new(eventemitter.EventEmitter), new(*eventemitter.Emitter)), NewProvider)

func Factory() (*eventemitter.Emitter, error) {
	dispatcher := eventemitter.New()

	return dispatcher, nil
}

var _ fabric.BootableProvider = (*Provider)(nil)

type Provider struct {
	dispatcher eventemitter.EventEmitter
}

func NewProvider(dispatcher eventemitter.EventEmitter) *Provider {
	p := &Provider{
		dispatcher: dispatcher,
	}

	return p
}

func (p *Provider) Name() string {
	return providerName
}

func (p *Provider) Priority() int {
	return fabric.PriorityBroker
}

func (p *Provider) Start(_ context.Context) error {
	return nil
}

// Stop drains the in-flight events. dispatcher.Wait is not context-aware, so it
// runs in its own goroutine and ctx bounds the wait: a listener that never
// returns cannot hold the whole shutdown hostage.
func (p *Provider) Stop(ctx context.Context) error {
	drained := make(chan struct{})

	go func() {
		p.dispatcher.Wait()

		close(drained)
	}()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("eventemitter drain: %w", ctx.Err())
	}
}
