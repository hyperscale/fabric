package consumer

import "github.com/hyperscale/fabric/broker"

var _ broker.Consumer = (*EventConsumer)(nil)

type EventConsumer struct {
}

func (c *EventConsumer) Register() error {
	return nil
}

// fabric:broker:topic = "acme.events"
// fabric:broker:group = "acme-a-consumer"
// fabric:broker:queue = "acme-a-consumer"
func (c *EventConsumer) HandleMessage(message broker.Message) error {
	return nil
}

func NewEventConsumer() *EventConsumer {
	return &EventConsumer{}
}
