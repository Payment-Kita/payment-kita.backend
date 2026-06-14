package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration values
type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	Redis      RedisConfig
	RabbitMQ   RabbitMQConfig
	JWT        JWTConfig
	Blockchain BlockchainConfig
	Security   SecurityConfig
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port string
	Env  string
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// URL returns the database connection URL
func (c DatabaseConfig) URL() string {
	return "postgres://" + c.User + ":" + c.Password + "@" + c.Host + ":" + strconv.Itoa(c.Port) + "/" + c.DBName + "?sslmode=" + c.SSLMode
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	URL      string
	Username string
	PASSWORD string
}

// RabbitMQConfig holds RabbitMQ configuration
type RabbitMQConfig struct {
	URL string
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret        string
	AccessExpiry  time.Duration
	RefreshExpiry time.Duration
}

// BlockchainConfig holds blockchain RPC URLs
type BlockchainConfig struct {
	BaseSepoliaRPC  string
	BSCSepoliaRPC   string
	SolanaDevnetRPC string
	OwnerPrivateKey string
}

// SecurityConfig holds security encryption keys
type SecurityConfig struct {
	ApiKeyEncryptionKey  string
	SessionEncryptionKey string
	JweMasterKey         string
}

// Load loads configuration from environment variables
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
			Env:  getEnv("SERVER_ENV", "development"),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvAsInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			DBName:   getEnv("DB_NAME", "paymentkita"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			URL:      getEnv("REDIS_URL", "redis://localhost:6379"),
			Username: getEnv("REDIS_USERNAME", ""),
			PASSWORD: getEnv("REDIS_PASSWORD", ""),
		},
		RabbitMQ: RabbitMQConfig{
			URL: getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		},
		JWT: JWTConfig{
			Secret:        getEnv("JWT_SECRET", "change-this-in-production"),
			AccessExpiry:  getEnvAsDuration("JWT_ACCESS_EXPIRY", 15*time.Minute),
			RefreshExpiry: getEnvAsDuration("JWT_REFRESH_EXPIRY", 7*24*time.Hour),
		},
		Blockchain: BlockchainConfig{
			BaseSepoliaRPC:  getEnv("BASE_SEPOLIA_RPC_URL", "https://sepolia.base.org"),
			BSCSepoliaRPC:   getEnv("BSC_SEPOLIA_RPC_URL", "https://data-seed-prebsc-1-s1.binance.org:8545"),
			SolanaDevnetRPC: getEnv("SOLANA_DEVNET_RPC_URL", "https://api.devnet.solana.com"),
			OwnerPrivateKey: getEnv("EVM_OWNER_PRIVATE_KEY", getEnv("PRIVATE_KEY", "")),
		},
		Security: SecurityConfig{
			ApiKeyEncryptionKey:  getEnv("API_KEY_ENCRYPTION_KEY", "0000000000000000000000000000000000000000000000000000000000000000"), // 32-bytes hex string
			SessionEncryptionKey: getEnv("SESSION_ENCRYPTION_KEY", "0000000000000000000000000000000000000000000000000000000000000000"), // 32-bytes hex string
			JweMasterKey:         getEnv("JWE_MASTER_KEY", "0000000000000000000000000000000000000000000000000000000000000000"),         // 32-bytes hex string
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
