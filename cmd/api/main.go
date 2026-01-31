package main

import (
	"btc-giftcard/internal/database"
	"btc-giftcard/pkg/cache"
	"btc-giftcard/pkg/logger"
	"context"
	"time"

	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	if err := logger.Init(logger.GetEnv()); err != nil {
		panic(err)
	}
	defer logger.Sync() // Flush logs before exit

	logger.Info("Server starting", zap.Int("port", 8080))
	logger.Debug("Debug mode enabled")
	logger.Warn("This is a warning", zap.String("reason", "testing"))

	cacheCfg := cache.Config{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	if err := cache.Init(cacheCfg); err != nil {
		logger.Fatal("Failed to initialize cache", zap.Error(err))
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

	dbCfg := database.Config{
		Host:            "localhost",
		Port:            "5432",
		User:            "postgres",
		Password:        "postgres",
		DB:              "btcgifter",
		SslMode:         "disable",
		MaxConns:        25,
		MinConns:        5,
		MaxConnLifetime: 5,
		MaxConnIdleTime: 1,
	}

	// Initialize database
	db, err := database.NewDB(dbCfg)
	if err != nil {
		logger.Fatal("Failed to initialize database connection", zap.Error(err))
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(ctx); err != nil {
		logger.Fatal("Database ping failed", zap.Error(err))
	}
	logger.Info("Database connected and verified successfully")

	// Run migrations
	if err := db.RunMigrations(); err != nil {
		logger.Fatal("Failed to run migrations", zap.Error(err))
	}
}
