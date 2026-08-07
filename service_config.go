package fabric

import (
	"errors"
	"fmt"
	"time"

	"github.com/euskadi31/wire"
)

// ServiceProviderName is the name of the HCL provider block holding the
// service-level settings.
const ServiceProviderName = "service"

// ServiceConfigSet provides a *ServiceConfig to the wire graph.
var ServiceConfigSet = wire.NewSet(ServiceConfigProvider)

// ServiceConfig is the optional `provider "service"` block:
//
//	provider "service" {
//	  name             = "acme-a-consumer"
//	  version          = "0.0.1"
//	  shutdown_timeout = "30s"
//	  pre_stop_delay   = "5s"
//	}
//
// An absent block means the defaults apply; a malformed one is fatal.
//
// Durations are written as Go duration strings and parsed by Parse into the
// untagged fields below. gohcl decodes through gocty, which maps a
// time.Duration to a bare number and ignores encoding.TextUnmarshaler, so
// decoding directly into a time.Duration would force `30000000000` into the
// configuration file.
type ServiceConfig struct {
	Name    string `hcl:"name,optional"`
	Version string `hcl:"version,optional"`

	// RawShutdownTimeout bounds the whole drain, e.g. "30s".
	RawShutdownTimeout string `hcl:"shutdown_timeout,optional"`

	// RawPreStopDelay is the pause between turning readiness off and tearing
	// anything down, e.g. "5s".
	RawPreStopDelay string `hcl:"pre_stop_delay,optional"`

	// ShutdownTimeout and PreStopDelay carry the parsed values. They have no
	// hcl tag, so gohcl leaves them alone; Parse fills them. Code that builds a
	// ServiceConfig by hand can set them directly and skip Parse.
	ShutdownTimeout time.Duration
	PreStopDelay    time.Duration
}

// Parse converts the raw duration strings into their time.Duration fields. An
// empty string leaves the corresponding field untouched, so defaults survive.
func (c *ServiceConfig) Parse() error {
	if c.RawShutdownTimeout != "" {
		d, err := time.ParseDuration(c.RawShutdownTimeout)
		if err != nil {
			return fmt.Errorf("invalid shutdown_timeout %q: %w", c.RawShutdownTimeout, err)
		}

		c.ShutdownTimeout = d
	}

	if c.RawPreStopDelay != "" {
		d, err := time.ParseDuration(c.RawPreStopDelay)
		if err != nil {
			return fmt.Errorf("invalid pre_stop_delay %q: %w", c.RawPreStopDelay, err)
		}

		c.PreStopDelay = d
	}

	return nil
}

// ServiceConfigProvider decodes the optional `service` block.
func ServiceConfigProvider(cfg *Configuration) (*ServiceConfig, error) {
	c := &ServiceConfig{
		ShutdownTimeout: DefaultShutdownTimeout,
	}

	if err := cfg.ParseProvider(ServiceProviderName, c); err != nil && !errors.Is(err, ErrProviderNotFound) {
		return nil, fmt.Errorf("failed to parse %s config: %w", ServiceProviderName, err)
	}

	if err := c.Parse(); err != nil {
		return nil, fmt.Errorf("failed to parse %s config: %w", ServiceProviderName, err)
	}

	return c, nil
}

// WithConfig applies a ServiceConfig. Options passed after it win, so an
// explicit WithShutdownTimeout still overrides the configuration file.
func WithConfig(cfg *ServiceConfig) ServiceOption {
	return func(s *Service) {
		if cfg == nil {
			return
		}

		if cfg.Name != "" {
			s.name = cfg.Name
		}

		if cfg.Version != "" {
			s.version = cfg.Version
		}

		if cfg.ShutdownTimeout > 0 {
			s.shutdownTimeout = cfg.ShutdownTimeout
		}

		if cfg.PreStopDelay > 0 {
			s.preStopDelay = cfg.PreStopDelay
		}
	}
}
