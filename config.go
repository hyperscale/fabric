package fabric

import (
	"fmt"
	"os"

	"github.com/euskadi31/wire"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hyperscale/fabric/config"
)

var (
	ErrProviderNotFound = fmt.Errorf("provider not found")
)

var ConfigPath string

var ConfigSet = wire.NewSet(NewConfiguration)

type Configuration struct {
	parser    *config.Parser
	providers map[string]*config.Provider
}

func NewConfiguration() (cfg *Configuration, err error) {
	cfg = &Configuration{
		parser: config.NewParser(nil),
	}

	configDir := ConfigPath

	if configDir == "" {
		configDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current dir: %w", err)
		}
	}

	module, diags := cfg.parser.LoadConfigDir(configDir)
	if diags.HasErrors() {
		return nil, fmt.Errorf("error in load config dir: %w", diags)
	}

	cfg.providers = module.Providers

	return cfg, nil
}

func (c *Configuration) ParseProvider(name string, v interface{}) hcl.Diagnostics {
	provider, ok := c.providers[name]
	if !ok {
		return hcl.Diagnostics{
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Provider not found",
			},
		}
	}

	ctx := c.parser.Context()

	if diag := gohcl.DecodeBody(provider.HCL, ctx, v); diag.HasErrors() {
		return diag
	}

	return nil
}
