package main

import (
	"database/sql"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"payment-kita.backend/internal/config"
	plog "payment-kita.backend/pkg/logger"
	"payment-kita.backend/pkg/redis"
)

func withMainHooks(t *testing.T) {
	t.Helper()
	origLoadDotenv := loadDotenv
	origLoadCfg := loadCfg
	origInitLog := initLog
	origInitRedis := initRedis
	origOpenDB := openDB
	origNewSessionStore := newSessionStore
	origRunServer := runServer
	origGetStdDB := getStdDB

	t.Cleanup(func() {
		loadDotenv = origLoadDotenv
		loadCfg = origLoadCfg
		initLog = origInitLog
		initRedis = origInitRedis
		openDB = origOpenDB
		newSessionStore = origNewSessionStore
		runServer = origRunServer
		getStdDB = origGetStdDB
	})
}

func baseTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Port: "18080",
			Env:  "development",
		},
		Database: config.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "postgres",
			DBName:   "paymentkita",
			SSLMode:  "disable",
		},
		Redis: config.RedisConfig{
			URL:      "redis://localhost:6379",
			PASSWORD: "",
		},
		JWT: config.JWTConfig{
			Secret:        "secret",
			AccessExpiry:  15 * time.Minute,
			RefreshExpiry: 24 * time.Hour,
		},
		Blockchain: config.BlockchainConfig{
			OwnerPrivateKey: "",
		},
		Security: config.SecurityConfig{
			ApiKeyEncryptionKey:  "0000000000000000000000000000000000000000000000000000000000000000",
			SessionEncryptionKey: "0000000000000000000000000000000000000000000000000000000000000000",
			JweMasterKey:         "0000000000000000000000000000000000000000000000000000000000000000",
		},
	}
}

func TestRunMainProcess_RedisInitError(t *testing.T) {
	withMainHooks(t)

	loadDotenv = func(...string) error { return nil }
	loadCfg = baseTestConfig
	initLog = plog.Init
	initRedis = func(string, string, string) error { return errors.New("redis down") }

	err := runMainProcess()
	if err == nil {
		t.Fatal("expected redis init error")
	}
}

func TestRunMainProcess_DBOpenError(t *testing.T) {
	withMainHooks(t)

	loadDotenv = func(...string) error { return nil }
	loadCfg = baseTestConfig
	initLog = plog.Init
	initRedis = func(string, string, string) error { return nil }
	openDB = func(string) (*gorm.DB, error) { return nil, errors.New("db open failed") }

	err := runMainProcess()
	if err == nil {
		t.Fatal("expected db open error")
	}
}

func TestRunMainProcess_SessionStoreError(t *testing.T) {
	withMainHooks(t)

	loadDotenv = func(...string) error { return nil }
	loadCfg = baseTestConfig
	initLog = plog.Init
	initRedis = func(string, string, string) error { return nil }
	openDB = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:main_session_err?mode=memory&cache=shared"), &gorm.Config{})
	}
	newSessionStore = func(string) (*redis.SessionStore, error) { return nil, errors.New("bad session key") }

	err := runMainProcess()
	if err == nil {
		t.Fatal("expected session store error")
	}
}

func TestRunMainProcess_ServerRunError(t *testing.T) {
	withMainHooks(t)

	loadDotenv = func(...string) error { return nil }
	loadCfg = baseTestConfig
	initLog = plog.Init
	initRedis = func(string, string, string) error { return nil }
	openDB = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:main_server_err?mode=memory&cache=shared"), &gorm.Config{})
	}
	newSessionStore = redis.NewSessionStore
	runServer = func(*gin.Engine, string) error { return errors.New("listen failed") }

	err := runMainProcess()
	if err == nil {
		t.Fatal("expected server run error")
	}
}

func TestRunMainProcess_SuccessPath(t *testing.T) {
	withMainHooks(t)

	loadDotenv = func(...string) error { return nil }
	loadCfg = baseTestConfig
	initLog = plog.Init
	initRedis = func(string, string, string) error { return nil }
	openDB = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:main_success?mode=memory&cache=shared"), &gorm.Config{})
	}
	newSessionStore = redis.NewSessionStore
	runServer = func(*gin.Engine, string) error { return nil }

	if err := runMainProcess(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMainProcess_SuccessPath_WithDotenvLoadError(t *testing.T) {
	withMainHooks(t)

	loadDotenv = func(...string) error { return errors.New("dotenv missing") }
	loadCfg = baseTestConfig
	initLog = plog.Init
	initRedis = func(string, string, string) error { return nil }
	openDB = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:main_success_dotenv_error?mode=memory&cache=shared"), &gorm.Config{})
	}
	newSessionStore = redis.NewSessionStore
	runServer = func(*gin.Engine, string) error { return nil }

	if err := runMainProcess(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultOpenDBAndRunServerWrappers_ExecuteBodies(t *testing.T) {
	withMainHooks(t)

	// Cover default openDB wrapper body (main.go:35-42).
	origOpen := openDB
	defer func() { openDB = origOpen }()
	openDB = func(dsn string) (*gorm.DB, error) {
		return origOpen(dsn)
	}
	_, err := openDB("host=localhost port=-1 user=postgres password=postgres dbname=paymentkita sslmode=disable")
	if err == nil {
		t.Fatal("expected openDB wrapper to fail on invalid DSN")
	}

	// Cover default runServer wrapper body (main.go:44).
	origRun := runServer
	defer func() { runServer = origRun }()
	runServer = func(r *gin.Engine, port string) error {
		return origRun(r, port)
	}
	engine := gin.New()
	err = runServer(engine, "invalid-port")
	if err == nil {
		t.Fatal("expected runServer wrapper to fail on invalid port")
	}
}

func TestRunMainProcess_ProductionModeAndPingWarnPath(t *testing.T) {
	withMainHooks(t)

	loadDotenv = func(...string) error { return nil }
	loadCfg = func() *config.Config {
		cfg := baseTestConfig()
		cfg.Server.Env = "production"
		return cfg
	}
	initLog = plog.Init
	initRedis = func(string, string, string) error { return nil }
	openDB = func(string) (*gorm.DB, error) {
		db, err := gorm.Open(sqlite.Open("file:main_prod_ping_warn?mode=memory&cache=shared"), &gorm.Config{})
		if err != nil {
			return nil, err
		}
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close() // force Ping() error branch
		}
		return db, nil
	}
	newSessionStore = redis.NewSessionStore
	runServer = func(*gin.Engine, string) error { return nil }

	if err := runMainProcess(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gin.Mode() != gin.ReleaseMode {
		t.Fatalf("expected release mode, got %s", gin.Mode())
	}
}

func TestRunMainProcess_GracefulShutdownSignalBranch(t *testing.T) {
	withMainHooks(t)

	loadDotenv = func(...string) error { return nil }
	loadCfg = baseTestConfig
	initLog = plog.Init
	initRedis = func(string, string, string) error { return nil }
	openDB = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:main_graceful_signal?mode=memory&cache=shared"), &gorm.Config{})
	}
	newSessionStore = redis.NewSessionStore
	runServer = func(*gin.Engine, string) error {
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	if err := runMainProcess(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunMainProcess_GetStdDBError(t *testing.T) {
	withMainHooks(t)

	loadDotenv = func(...string) error { return nil }
	loadCfg = baseTestConfig
	initLog = plog.Init
	initRedis = func(string, string, string) error { return nil }
	openDB = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:main_getstdb_error?mode=memory&cache=shared"), &gorm.Config{})
	}
	getStdDB = func(*gorm.DB) (*sql.DB, error) { return nil, errors.New("stdb failed") }
	newSessionStore = redis.NewSessionStore
	runServer = func(*gin.Engine, string) error { return nil }

	err := runMainProcess()
	if err == nil {
		t.Fatal("expected generic database object error")
	}
}
