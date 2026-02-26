package main

import (
	"btc-giftcard/config"
	"btc-giftcard/internal/database"
	"btc-giftcard/pkg/cache"
	"btc-giftcard/pkg/logger"
	"context"
	"fmt"
	"os"
	"time"

	"path/filepath"
	"runtime"

	"github.com/jinzhu/copier"
	"go.uber.org/zap"
)

var Cfg config.ApiConfig

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Initialize logger
	if err := logger.Init(logger.GetEnv()); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Sync()

	_, filename, _, _ := runtime.Caller(0)

	root := filepath.Dir(filename)
	configPath := config.Path(root).Join("config.toml", "..", "..")

	if err := config.Load(configPath, &Cfg); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info("Server starting", zap.Int("port", 8080))
	logger.Debug("Debug mode enabled")
	logger.Warn("This is a warning", zap.String("reason", "testing"))

	// Initialize cache with automatic field mapping
	var redisCfg cache.Config
	if err := copier.Copy(&redisCfg, &Cfg.Redis); err != nil {
		return fmt.Errorf("failed to copy cache config: %w", err)
	}
	if err := cache.Init(redisCfg); err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Test Set
	cache.Set(ctx, "test_key", "hello world", 5*time.Minute)

	// Test Get
	val, _ := cache.Get(ctx, "test_key")
	logger.Info("Retrieved from cache", zap.String("value", val))

	// Test SetNX (lock mechanism)
	locked, _ := cache.SetNX(ctx, "lock:card:123", "processing", 30*time.Second)
	logger.Info("Lock acquired", zap.Bool("success", locked))

	// Try again - should fail
	locked, _ = cache.SetNX(ctx, "lock:card:123", "processing", 30*time.Second)
	logger.Info("Lock acquired again", zap.Bool("success", locked)) // false

	// Test Incr (rate limiting)
	count, _ := cache.Incr(ctx, "attempts:192.168.1.1")
	logger.Info("Attempt count", zap.Int64("count", count))

	logger.Info("Server started successfully")

	// Initialize database with automatic field mapping
	var dbCfg database.Config
	if err := copier.Copy(&dbCfg, &Cfg.Database); err != nil {
		return fmt.Errorf("failed to copy database config: %w", err)
	}
	db, err := database.NewDB(dbCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize database connection: %w", err)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	logger.Info("Database connected and verified successfully")

	// Run migrations
	if err := db.RunMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
