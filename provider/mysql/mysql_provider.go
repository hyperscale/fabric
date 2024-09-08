package mysql

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	sqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/wire"
	"github.com/hyperscale/fabric"
	"github.com/jmoiron/sqlx"
)

const (
	ProviderName = "mysql"
)

var Set = wire.NewSet(ConfigProvider, Factory, NewProvider)

type Config struct {
	Host     string `hcl:"host"`
	Port     int    `hcl:"port"`
	Username string `hcl:"username"`
	Password string `hcl:"password"`
	Database string `hcl:"database"`
}

func (c *Config) FormatDSN() string {
	sqlCfg := sqldriver.Config{
		Loc:                  time.UTC,
		Net:                  "tcp",
		Addr:                 net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		User:                 c.Username,
		Passwd:               c.Password,
		DBName:               c.Database,
		AllowNativePasswords: true,
		CheckConnLiveness:    true,
		ParseTime:            true,
	}

	return sqlCfg.FormatDSN()
}

func ConfigProvider(cfg *fabric.Configuration) (*Config, error) {
	c := &Config{}
	if err := cfg.ParseProvider(ProviderName, c); err != nil {
		return nil, fmt.Errorf("failed to parse mysql config: %w", err)
	}

	return c, nil
}

func Factory(logger *slog.Logger, cfg *Config) (*sqlx.DB, error) {
	db, err := sqlx.Connect(
		ProviderName,
		cfg.FormatDSN(),
	)
	if err != nil {
		logger.Error("MySQLFactory failed", slog.Any("error", err))

		return nil, fmt.Errorf("mysql factory: %w", err)
	}

	return db, nil // *sqlx.DB
}

var _ fabric.BootableProvider = (*Provider)(nil)

type Provider struct {
	db *sqlx.DB
}

func NewProvider(db *sqlx.DB) *Provider {
	p := &Provider{
		db: db,
	}

	return p
}

func (p *Provider) Name() string {
	return ProviderName
}

func (p *Provider) Priority() int {
	return 0
}

func (p *Provider) Start() error {
	return nil
}

func (p *Provider) Stop() error {
	// nolint: wrapcheck // not nessary
	return p.db.Close()
}
