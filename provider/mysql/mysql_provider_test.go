package mysql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	c := &Config{
		Host:     "localhost",
		Port:     3306,
		Username: "user",
		Password: "pass",
		Database: "test",
	}

	assert.Equal(t, "user:pass@tcp(localhost:3306)/test?parseTime=true&maxAllowedPacket=0", c.FormatDSN())
}
