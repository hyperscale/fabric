package app

import (
	"fmt"

	"github.com/google/wire"
	"github.com/hyperscale/fabric"
	"github.com/hyperscale/fabric/provider/mysql"
	"golang.org/x/exp/slog"
)

var applicationSet = wire.NewSet(
	wire.Struct(new(Options), "*"),
	NewApplication,
)

type Options struct {
	MySQLProvider *mysql.Provider
}

func NewApplication(logger *slog.Logger, opts *Options) (*fabric.Service, error) {
	logger.Debug("Starting Fabric Application")

	s, err := fabric.NewService(
		fabric.Name("acme-a-consumer"),
		fabric.Version("0.0.1"),
		fabric.Logger(logger),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	s.Register(opts.MySQLProvider)

	return s, nil
}
