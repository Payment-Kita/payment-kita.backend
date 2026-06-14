package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDatabaseConfig_URL(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "user",
		Password: "pass",
		DBName:   "db",
		SSLMode:  "disable",
	}
	assert.Equal(t, "postgres://user:pass@localhost:5432/db?sslmode=disable", cfg.URL())
}

func TestLoad_ConfigFromEnv(t *testing.T) {
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("JWT_ACCESS_EXPIRY", "30m")
	t.Setenv("EVM_OWNER_PRIVATE_KEY", "0xabc")

	cfg := Load()
	assert.Equal(t, "9090", cfg.Server.Port)
	assert.Equal(t, 6543, cfg.Database.Port)
	assert.Equal(t, 30*time.Minute, cfg.JWT.AccessExpiry)
	assert.Equal(t, "0xabc", cfg.Blockchain.OwnerPrivateKey)
}

func TestLoad_ConfigFallbacks(t *testing.T) {
	t.Setenv("DB_PORT", "not-number")
	t.Setenv("JWT_ACCESS_EXPIRY", "bad-duration")
	t.Setenv("EVM_OWNER_PRIVATE_KEY", "")
	t.Setenv("PRIVATE_KEY", "fallback-key")

	cfg := Load()
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, 15*time.Minute, cfg.JWT.AccessExpiry)
	assert.Equal(t, "fallback-key", cfg.Blockchain.OwnerPrivateKey)
}
