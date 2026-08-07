package app

import (
	"fmt"
	"log/slog"

	"github.com/euskadi31/wire"
	"github.com/hyperscale/fabric"
	"github.com/hyperscale/fabric/provider/mysql"
)

// nolint:unused
var applicationSet = wire.NewSet(
	wire.Struct(new(Options), "*"),
	NewApplication,
)

type Options struct {
	MySQLProvider *mysql.Provider
}

func NewApplication(
	logger *slog.Logger,
	cfg *fabric.ServiceConfig,
	readiness *fabric.Readiness,
	opts *Options,
) (*fabric.Service, error) {
	logger.Debug("Starting Fabric Application")

	s, err := fabric.NewService(
		fabric.WithName("acme-a-consumer"),
		fabric.WithVersion("0.0.1"),
		fabric.WithLogger(logger),
		fabric.WithReadiness(readiness),
		// Last, so the `service` block of config.hcl overrides the defaults
		// above.
		fabric.WithConfig(cfg),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	if err := s.Register(opts.MySQLProvider); err != nil {
		return nil, fmt.Errorf("failed to register providers: %w", err)
	}

	return s, nil
}
