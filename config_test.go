package fabric

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestConfig struct {
	Name string `hcl:"name"`
	Port int    `hcl:"port"`
}

func TestConfiguration(t *testing.T) {
	os.Setenv("TEST_PORT", "8080")

	ConfigPath = "./testdata/cfg_with_env_vars/config.hcl"

	cfg, err := NewConfiguration()
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	var testCfg TestConfig

	err = cfg.ParseProvider("test", &testCfg)
	assert.Nil(t, err)
	assert.Equal(t, "test_provider", testCfg.Name)
	assert.Equal(t, 8080, testCfg.Port)
}
