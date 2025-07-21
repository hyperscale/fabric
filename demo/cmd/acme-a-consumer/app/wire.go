//go:build wireinject
// +build wireinject

package app

import (
	"github.com/euskadi31/wire"
	"github.com/hyperscale/fabric"
	"github.com/hyperscale/fabric/provider/logger"
	"github.com/hyperscale/fabric/provider/mysql"
)

func New() (*fabric.Service, error) {
	panic(wire.Build(fabric.ConfigSet, logger.Set, mysql.Set, applicationSet))
}
