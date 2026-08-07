package fabric

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceConfigProvider(t *testing.T) {
	cfg, err := NewConfigurationFromDir("./testdata/cfg_service/config.hcl")
	require.NoError(t, err)

	sc, err := ServiceConfigProvider(cfg)
	require.NoError(t, err)

	assert.Equal(t, "probe", sc.Name)
	assert.Equal(t, "1.2.3", sc.Version)
	assert.Equal(t, 45*time.Second, sc.ShutdownTimeout)
	assert.Equal(t, 3*time.Second, sc.PreStopDelay)
}

// An absent service block is not an error: the defaults apply.
func TestServiceConfigProvider_AbsentBlock(t *testing.T) {
	t.Setenv("TEST_PORT", "8080")

	cfg, err := NewConfigurationFromDir("./testdata/cfg_with_env_vars/config.hcl")
	require.NoError(t, err)

	sc, err := ServiceConfigProvider(cfg)
	require.NoError(t, err)

	assert.Equal(t, DefaultShutdownTimeout, sc.ShutdownTimeout)
	assert.Zero(t, sc.PreStopDelay)
}

func TestServiceConfig_Parse_Invalid(t *testing.T) {
	c := &ServiceConfig{RawShutdownTimeout: "thirty seconds"}

	require.Error(t, c.Parse())
}

func TestWithConfig(t *testing.T) {
	svc := testService(t, WithConfig(&ServiceConfig{
		Name:            "from-config",
		Version:         "9.9.9",
		ShutdownTimeout: 12 * time.Second,
		PreStopDelay:    4 * time.Second,
	}))

	assert.Equal(t, "from-config", svc.name)
	assert.Equal(t, "9.9.9", svc.version)
	assert.Equal(t, 12*time.Second, svc.shutdownTimeout)
	assert.Equal(t, 4*time.Second, svc.preStopDelay)
}

// A later option must win over the configuration file.
func TestWithConfig_LaterOptionsWin(t *testing.T) {
	svc := testService(t,
		WithConfig(&ServiceConfig{ShutdownTimeout: 12 * time.Second}),
		WithShutdownTimeout(3*time.Second),
	)

	assert.Equal(t, 3*time.Second, svc.shutdownTimeout)
}

func TestWithConfig_Nil(t *testing.T) {
	svc := testService(t, WithConfig(nil))

	assert.Equal(t, DefaultShutdownTimeout, svc.shutdownTimeout)
}

// The pre-stop delay must actually delay the teardown, and readiness must
// already be false while it elapses.
func TestService_PreStopDelay(t *testing.T) {
	rec := newRecorder()
	svc := testService(t, WithPreStopDelay(80*time.Millisecond))

	var readyDuringStop bool

	p := newFakeProvider(rec, "a", PriorityDefault)
	p.onStop = func(context.Context) { readyDuringStop = svc.Ready() }

	mustRegister(t, svc, p)
	require.NoError(t, svc.Start(t.Context()))

	start := time.Now()
	require.NoError(t, svc.Shutdown(t.Context()))
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 80*time.Millisecond)
	assert.False(t, readyDuringStop)
}
