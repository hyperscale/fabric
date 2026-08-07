package mysql

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	sqldriver "github.com/go-sql-driver/mysql"
	"github.com/hyperscale/fabric"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_FormatDSN(t *testing.T) {
	c := &Config{
		Host:     "localhost",
		Port:     3306,
		Username: "user",
		Password: "pass",
		Database: "test",
	}

	assert.Equal(t, "user:pass@tcp(localhost:3306)/test?parseTime=true&maxAllowedPacket=0", c.FormatDSN())
}

// The DSN must survive values that need quoting or escaping, otherwise a
// password containing an @ or a / silently produces a DSN pointing elsewhere.
func TestConfig_FormatDSN_EscapesSpecialCharacters(t *testing.T) {
	c := &Config{
		Host:     "db.internal",
		Port:     3307,
		Username: "user@corp",
		Password: "p@ss/w:rd",
		Database: "my-db",
	}

	dsn := c.FormatDSN()

	// The driver must be able to read back exactly what we wrote.
	parsed, err := sqldriver.ParseDSN(dsn)
	require.NoError(t, err)

	assert.Equal(t, "user@corp", parsed.User)
	assert.Equal(t, "p@ss/w:rd", parsed.Passwd)
	assert.Equal(t, "my-db", parsed.DBName)
	assert.Equal(t, "db.internal:3307", parsed.Addr)
}

func TestConfig_FormatDSN_IPv6Host(t *testing.T) {
	c := &Config{Host: "::1", Port: 3306, Database: "test"}

	assert.Contains(t, c.FormatDSN(), "tcp([::1]:3306)", "an IPv6 host must be bracketed")
}

func TestConfig_FormatDSN_SetsConnectionOptions(t *testing.T) {
	parsed, err := sqldriver.ParseDSN((&Config{Host: "h", Port: 1, Database: "d"}).FormatDSN())
	require.NoError(t, err)

	assert.True(t, parsed.ParseTime, "timestamps must decode into time.Time")
	assert.True(t, parsed.AllowNativePasswords)
	assert.True(t, parsed.CheckConnLiveness)
	assert.Equal(t, time.UTC, parsed.Loc)
	assert.Equal(t, "tcp", parsed.Net)
}

func TestConfigProvider(t *testing.T) {
	cfg, err := fabric.NewConfigurationFromDir("./testdata/valid")
	require.NoError(t, err)

	c, err := ConfigProvider(cfg)

	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "db.example.com", c.Host)
	assert.Equal(t, 3307, c.Port)
	assert.Equal(t, "app", c.Username)
	assert.Equal(t, "s3cret", c.Password)
	assert.Equal(t, "appdb", c.Database)
}

// The mysql block is required: there is no sensible default DSN, so an absent
// block must fail rather than fall back to something that points nowhere.
func TestConfigProvider_MissingBlockIsFatal(t *testing.T) {
	cfg, err := fabric.NewConfigurationFromDir("./testdata/missing")
	require.NoError(t, err)

	c, err := ConfigProvider(cfg)

	require.Error(t, err)
	assert.Nil(t, c)
	assert.ErrorIs(t, err, fabric.ErrProviderNotFound)
}

func TestConfigProvider_InvalidBlockIsFatal(t *testing.T) {
	cfg, err := fabric.NewConfigurationFromDir("./testdata/invalid")
	require.NoError(t, err)

	c, err := ConfigProvider(cfg)

	require.Error(t, err)
	assert.Nil(t, c)
	assert.ErrorIs(t, err, fabric.ErrProviderInvalid)
}

func TestFactory_UnreachableServer(t *testing.T) {
	cfg := &Config{Host: "127.0.0.1", Port: closedPort(t), Username: "u", Database: "d"}

	db, err := Factory(slog.New(slog.DiscardHandler), cfg)

	require.Error(t, err)
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "mysql factory")
}

func TestNewProvider(t *testing.T) {
	db := openUnreachable(t)

	p := NewProvider(db)

	require.NotNil(t, p)
	assert.Same(t, db, p.db)
}

func TestProvider_Name(t *testing.T) {
	assert.Equal(t, ProviderName, NewProvider(openUnreachable(t)).Name())
	assert.Equal(t, "mysql", ProviderName)
}

// The pool must come up before anything that queries it, and go down after.
func TestProvider_Priority(t *testing.T) {
	assert.Equal(t, fabric.PriorityDatabase, NewProvider(openUnreachable(t)).Priority())
	assert.Greater(t, fabric.PriorityDatabase, fabric.PriorityTelemetry)
	assert.Less(t, fabric.PriorityDatabase, fabric.PriorityServer)
}

func TestProvider_ImplementsBootableProvider(t *testing.T) {
	var _ fabric.BootableProvider = (*Provider)(nil)
}

// Start pings, so an unreachable database is a fatal boot error instead of a
// failure on the first query at request time.
func TestProvider_Start_UnreachableDatabaseFails(t *testing.T) {
	p := NewProvider(openUnreachable(t))

	err := p.Start(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mysql ping")
}

func TestProvider_Start_CanceledContext(t *testing.T) {
	p := NewProvider(openUnreachable(t))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := p.Start(ctx)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestProvider_Stop_ClosesThePool(t *testing.T) {
	db := openUnreachable(t)
	p := NewProvider(db)

	require.NoError(t, p.Stop(t.Context()))

	// The pool is really closed, not merely reported as such. database/sql keeps
	// its "database is closed" sentinel unexported, so match on the message.
	require.ErrorContains(t, db.PingContext(t.Context()), "database is closed")
}

// database/sql tolerates a repeated Close, so a Stop that runs twice must not
// turn into a spurious shutdown error.
func TestProvider_Stop_IsIdempotent(t *testing.T) {
	p := NewProvider(openUnreachable(t))

	require.NoError(t, p.Stop(t.Context()))
	require.NoError(t, p.Stop(t.Context()))
}

// Close is not context-aware, so Stop bails out early rather than pretending to
// honor a budget that is already spent.
func TestProvider_Stop_ExpiredContext(t *testing.T) {
	p := NewProvider(openUnreachable(t))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := p.Stop(ctx)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "mysql close")
}

func TestProvider_Stop_DeadlineExceeded(t *testing.T) {
	p := NewProvider(openUnreachable(t))

	ctx, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
	defer cancel()

	time.Sleep(time.Millisecond)

	assert.ErrorIs(t, p.Stop(ctx), context.DeadlineExceeded)
}

// openUnreachable returns a pool pointed at a closed port. sqlx.Open is lazy, so
// no connection is attempted until the test asks for one.
func openUnreachable(t *testing.T) *sqlx.DB {
	t.Helper()

	cfg := &Config{Host: "127.0.0.1", Port: closedPort(t), Username: "u", Database: "d"}

	db, err := sqlx.Open(ProviderName, cfg.FormatDSN())
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	return db
}

// closedPort binds a port and releases it, so connecting to it is refused
// immediately instead of hanging on an unroutable address.
func closedPort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	return port
}

