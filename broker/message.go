package broker

import "context"

type Message interface {
	ID() string
	Body() []byte
	Headers() map[string]string
	Attributes() map[string]string
	Ack() error
	Context() context.Context
	WithContext(ctx context.Context) Message
}
