package broker

type Consumer interface {
	Register() error
	HandleMessage(message Message) error
}
