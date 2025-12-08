package main

import (
	"btc-giftcard/pkg/cache"
	"btc-giftcard/pkg/logger"
	"context"
	"go.uber.org/zap"
	"time"
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

	cfg := cache.Config{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	if err := cache.Init(cfg); err != nil {
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
}
