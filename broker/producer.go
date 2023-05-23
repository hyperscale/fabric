package broker

type Producer interface {
	Publish(topic string, message Message) error
	Start() error
	Stop() error
}
