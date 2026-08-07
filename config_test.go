package fabric

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestConfig struct {
	Name string `hcl:"name"`
	Port int    `hcl:"port"`
}

func TestConfiguration(t *testing.T) {
	t.Setenv("TEST_PORT", "8080")

	cfg, err := NewConfigurationFromDir("./testdata/cfg_with_env_vars/config.hcl")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	var testCfg TestConfig

	// assert.NoError, not assert.Nil: ParseProvider used to return
	// hcl.Diagnostics, whose nil value becomes a non-nil error interface. Only
	// NoError catches a regression back to that.
	require.NoError(t, cfg.ParseProvider("test", &testCfg))
	assert.Equal(t, "test_provider", testCfg.Name)
	assert.Equal(t, 8080, testCfg.Port)
}

func TestConfiguration_HasProvider(t *testing.T) {
	cfg, err := NewConfigurationFromDir("./testdata/cfg_with_env_vars/config.hcl")
	require.NoError(t, err)

	t.Setenv("TEST_PORT", "8080")

	assert.True(t, cfg.HasProvider("test"))
	assert.False(t, cfg.HasProvider("nope"))
}

// An absent block must be distinguishable from a malformed one, so that a
// provider with an optional configuration can fall back to its defaults without
// swallowing a real misconfiguration.
func TestConfiguration_ParseProvider_NotFound(t *testing.T) {
	t.Setenv("TEST_PORT", "8080")

	cfg, err := NewConfigurationFromDir("./testdata/cfg_with_env_vars/config.hcl")
	require.NoError(t, err)

	var testCfg TestConfig

	err = cfg.ParseProvider("missing", &testCfg)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProviderNotFound)
	assert.NotErrorIs(t, err, ErrProviderInvalid)
	assert.Contains(t, err.Error(), `"missing"`)
}

func TestConfiguration_ParseProvider_Invalid(t *testing.T) {
	cfg, err := NewConfigurationFromDir("./testdata/cfg_invalid_provider/config.hcl")
	require.NoError(t, err)

	var testCfg TestConfig

	err = cfg.ParseProvider("test", &testCfg)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProviderInvalid)
	assert.NotErrorIs(t, err, ErrProviderNotFound)

	// The underlying diagnostics stay reachable so callers can report them.
	var diags hcl.Diagnostics

	assert.ErrorAs(t, err, &diags)
	assert.NotEmpty(t, diags)
}
