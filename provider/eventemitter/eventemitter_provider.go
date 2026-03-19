package eventemitter

import (
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
	return 0
}

func (p *Provider) Start() error {
	return nil
}

func (p *Provider) Stop() error {
	p.dispatcher.Wait()

	return nil
}
