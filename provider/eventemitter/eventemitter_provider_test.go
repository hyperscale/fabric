package eventemitter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactory(t *testing.T) {
	dispatcher, err := Factory()
	require.NoError(t, err)
	assert.NotNil(t, dispatcher)
}

func TestNewProvider(t *testing.T) {
	dispatcher, err := Factory()
	require.NoError(t, err)

	provider := NewProvider(dispatcher)
	assert.NotNil(t, provider)
}

func TestProvider_Name(t *testing.T) {
	dispatcher, _ := Factory()
	provider := NewProvider(dispatcher)

	assert.Equal(t, providerName, provider.Name())
	assert.Equal(t, "eventemitter", provider.Name())
}

func TestProvider_Priority(t *testing.T) {
	dispatcher, _ := Factory()
	provider := NewProvider(dispatcher)

	assert.Equal(t, 0, provider.Priority())
}

func TestProvider_Start(t *testing.T) {
	dispatcher, _ := Factory()
	provider := NewProvider(dispatcher)

	err := provider.Start()
	assert.NoError(t, err)
}

func TestProvider_Stop(t *testing.T) {
	dispatcher, _ := Factory()
	provider := NewProvider(dispatcher)

	err := provider.Stop()
	assert.NoError(t, err)
}

func TestProvider_StopAfterEmit(t *testing.T) {
	dispatcher, _ := Factory()
	provider := NewProvider(dispatcher)

	// Emit an event
	called := false
	dispatcher.Subscribe("test", func() {
		called = true
	})
	dispatcher.Dispatch("test")

	err := provider.Stop()
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestProviderName_Constant(t *testing.T) {
	assert.Equal(t, "eventemitter", providerName)
}
