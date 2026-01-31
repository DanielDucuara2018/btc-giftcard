//go:build integration

package cache

import (
	"btc-giftcard/pkg/logger"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Initialize logger for tests
	_ = logger.Init("development")
}

// setupTestRedis initializes Redis client for testing
func setupTestRedis(t *testing.T) {
	t.Helper()

	cfg := Config{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       1, // Use DB 1 for tests to avoid conflicts
	}

	err := Init(cfg)
	require.NoError(t, err, "Failed to connect to test Redis")
}

// cleanupTestRedis flushes the test database
func cleanupTestRedis(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	err := Client.FlushDB(ctx).Err()
	require.NoError(t, err, "Failed to flush test Redis DB")
}

func TestRedis_Init(t *testing.T) {
	cfg := Config{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       1,
	}

	err := Init(cfg)
	require.NoError(t, err)
	assert.NotNil(t, Client)

	// Test connection with Ping
	err = Ping(context.Background())
	assert.NoError(t, err)

	// Cleanup
	cleanupTestRedis(t)
}

func TestRedis_SetAndGet(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	key := "test:key"
	value := "test-value"

	// Set a key
	err := Set(ctx, key, value, 0)
	require.NoError(t, err)

	// Get the key
	result, err := Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, result)
}

func TestRedis_Get_NonExistentKey(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()

	// Get non-existent key should return empty string, not error
	result, err := Get(ctx, "non:existent:key")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestRedis_SetWithExpiration(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	key := "test:expiring:key"
	value := "will-expire"

	// Set with 1 second expiration
	err := Set(ctx, key, value, 1*time.Second)
	require.NoError(t, err)

	// Key should exist immediately
	result, err := Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, result)

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	// Key should be gone
	result, err = Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestRedis_Delete(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	key1 := "test:delete:1"
	key2 := "test:delete:2"

	// Set two keys
	err := Set(ctx, key1, "value1", 0)
	require.NoError(t, err)
	err = Set(ctx, key2, "value2", 0)
	require.NoError(t, err)

	// Delete both keys
	count, err := Delete(ctx, key1, key2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Verify they're gone
	exists, err := Exists(ctx, key1)
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = Exists(ctx, key2)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRedis_Exists(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	key := "test:exists"

	// Key should not exist initially
	exists, err := Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	// Set the key
	err = Set(ctx, key, "value", 0)
	require.NoError(t, err)

	// Key should now exist
	exists, err = Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRedis_SetNX(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	key := "test:setnx"

	// First SetNX should succeed
	set, err := SetNX(ctx, key, "value1", 0)
	require.NoError(t, err)
	assert.True(t, set, "First SetNX should succeed")

	// Second SetNX should fail (key exists)
	set, err = SetNX(ctx, key, "value2", 0)
	require.NoError(t, err)
	assert.False(t, set, "Second SetNX should fail")

	// Value should still be the first one
	result, err := Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, "value1", result)
}

func TestRedis_SetNX_WithExpiration(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	key := "test:setnx:expire"

	// Set with expiration
	set, err := SetNX(ctx, key, "value", 1*time.Second)
	require.NoError(t, err)
	assert.True(t, set)

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	// SetNX should succeed again after expiration
	set, err = SetNX(ctx, key, "new-value", 0)
	require.NoError(t, err)
	assert.True(t, set, "SetNX should succeed after key expired")
}

func TestRedis_Incr(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	key := "test:counter"

	// Increment non-existent key (should start at 0, then increment to 1)
	count, err := Incr(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Increment again
	count, err = Incr(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Increment again
	count, err = Incr(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestRedis_Expire(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	key := "test:expire"

	// Set a key without expiration
	err := Set(ctx, key, "value", 0)
	require.NoError(t, err)

	// Add expiration
	err = Expire(ctx, key, 1*time.Second)
	require.NoError(t, err)

	// Key should exist immediately
	exists, err := Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	// Key should be gone
	exists, err = Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRedis_Close(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	// Close should not error
	err := Close()
	assert.NoError(t, err)

	// Reinitialize for cleanup
	setupTestRedis(t)
}

func TestRedis_Ping(t *testing.T) {
	setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()

	// Ping should succeed
	err := Ping(ctx)
	assert.NoError(t, err)
}
