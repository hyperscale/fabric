package fabric

import (
	"errors"
	"fmt"
	"os"

	"github.com/euskadi31/wire"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hyperscale/fabric/config"
)

var (
	// ErrProviderNotFound is returned by ParseProvider when the configuration
	// contains no provider block with the requested name.
	//
	// A provider whose configuration is optional should test for it and fall
	// back to its defaults:
	//
	//	if err := cfg.ParseProvider(name, c); err != nil && !errors.Is(err, fabric.ErrProviderNotFound) {
	//		return nil, err
	//	}
	//
	// Anything else, including ErrProviderInvalid, stays fatal.
	ErrProviderNotFound = errors.New("fabric: provider block not found")

	// ErrProviderInvalid is returned by ParseProvider when a provider block
	// exists but cannot be decoded: an unknown attribute, a missing required
	// attribute or a type mismatch. It always wraps the underlying
	// hcl.Diagnostics, which stay reachable through errors.As.
	//
	// It is deliberately distinct from ErrProviderNotFound so that a typo in an
	// optional block is a hard failure instead of a silent fallback to defaults.
	ErrProviderInvalid = errors.New("fabric: provider block is invalid")
)

var ConfigPath string

var ConfigSet = wire.NewSet(NewConfiguration)

type Configuration struct {
	parser    *config.Parser
	providers map[string]*config.Provider
}

// NewConfiguration loads the configuration from ConfigPath, or from the current
// working directory when ConfigPath is empty.
func NewConfiguration() (*Configuration, error) {
	configDir := ConfigPath

	if configDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current dir: %w", err)
		}

		configDir = cwd
	}

	return NewConfigurationFromDir(configDir)
}

// NewConfigurationFromDir loads the configuration from an explicit directory.
// Unlike NewConfiguration it does not read the ConfigPath global, so tests and
// embedders can load a configuration without mutating process-wide state.
func NewConfigurationFromDir(configDir string) (*Configuration, error) {
	cfg := &Configuration{
		parser: config.NewParser(nil),
	}

	module, diags := cfg.parser.LoadConfigDir(configDir)
	if diags.HasErrors() {
		return nil, fmt.Errorf("error in load config dir: %w", diags)
	}

	cfg.providers = module.Providers

	return cfg, nil
}

// HasProvider reports whether a provider block with the given name exists in the
// loaded configuration.
func (c *Configuration) HasProvider(name string) bool {
	_, ok := c.providers[name]

	return ok
}

// ParseProvider decodes the provider block named name into v.
//
// It returns an error wrapping ErrProviderNotFound when no such block exists,
// and an error wrapping both ErrProviderInvalid and the underlying
// hcl.Diagnostics when the block exists but cannot be decoded. The two cases are
// distinguishable with errors.Is, so that an absent optional block and a
// malformed one can be told apart.
func (c *Configuration) ParseProvider(name string, v any) error {
	provider, ok := c.providers[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrProviderNotFound, name)
	}

	if diags := gohcl.DecodeBody(provider.HCL, c.parser.Context(), v); diags.HasErrors() {
		return fmt.Errorf("%w %q: %w", ErrProviderInvalid, name, diags)
	}

	return nil
}
