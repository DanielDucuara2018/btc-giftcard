package cache

import (
	"btc-giftcard/pkg/logger"
	"context"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"time"
)

type Config struct {
	Host     string
	Port     string
	Password string
	DB       int
}

var Client *redis.Client

func Init(cfg Config) error {
	// redis options
	opts := redis.Options{
		Addr:     cfg.Host + ":" + cfg.Port,
		Password: cfg.Password, // no password set
		DB:       cfg.DB,       // use default DB
	}

	// Create Redis client
	rdb := redis.NewClient(&opts)

	// Test connection with Ping
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		logger.Error("Failed to connect to Redis", zap.Error(err))
		return err
	}

	// Set global Client variable
	Client = rdb
	logger.Info("Connected to Redis successfully", zap.String("host", cfg.Host))
	return nil
}

func Get(ctx context.Context, key string) (string, error) {
	val, err := Client.Get(ctx, key).Result()
	if err == redis.Nil { // Key does not exist
		return "", nil
	} else if err != nil {
		logger.Error("Failed to get key from Redis", zap.String("key", key), zap.Error(err))
		return "", err
	}
	return val, nil
}

func Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	err := Client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		logger.Error("Failed to set key in Redis", zap.String("key", key), zap.Error(err))
		return err
	}
	return nil
}

func Delete(ctx context.Context, keys ...string) (int64, error) {
	res, err := Client.Del(ctx, keys...).Result()
	if err != nil {
		logger.Error("Failed to delete keys from Redis", zap.Strings("keys", keys), zap.Error(err))
		return 0, err
	}
	return res, nil
}

func Exists(ctx context.Context, key string) (bool, error) {
	res, err := Client.Exists(ctx, key).Result()
	if err != nil {
		logger.Error("Failed to check existence of key in Redis", zap.String("key", key), zap.Error(err))
		return false, err
	}
	return res > 0, nil
}

func SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	// Set if Not eXists - returns true if set, false if key exists (prevents race conditions)
	set, err := Client.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		logger.Error("Failed to set NX key in Redis", zap.String("key", key), zap.Error(err))
		return false, err
	}
	return set, nil
}

func Incr(ctx context.Context, key string) (int64, error) {
	res, err := Client.Incr(ctx, key).Result()
	if err != nil {
		logger.Error("Failed to increment key in Redis", zap.String("key", key), zap.Error(err))
		return 0, err
	}
	return res, nil
}

func Expire(ctx context.Context, key string, expiration time.Duration) error {
	// Set expiration on existing key
	err := Client.Expire(ctx, key, expiration).Err()
	if err != nil {
		logger.Error("Failed to set expiration on key in Redis", zap.String("key", key), zap.Error(err))
		return err
	}
	return nil
}

// Ping tests the Redis connection
func Ping(ctx context.Context) error {
	return Client.Ping(ctx).Err()
}

// Close closes the Redis connection
func Close() error {
	if Client != nil {
		return Client.Close()
	}
	return nil
}
