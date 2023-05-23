package broker

var _ Broker = (*BrokerManager)(nil)

type Broker interface {
}

type BrokerManager struct {
}

func NewBrokerManager() *BrokerManager {
	return &BrokerManager{}
}

func (m *BrokerManager) RegisterConsumer(consumer Consumer) error {
	return nil
}
