//go:build integration

package queue

import (
	"btc-giftcard/pkg/cache"
	"btc-giftcard/pkg/logger"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Initialize logger for tests
	_ = logger.Init("development")
}

// setupTestRedis initializes Redis client for queue testing
func setupTestRedis(t *testing.T) *StreamQueue {
	t.Helper()

	cfg := cache.Config{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       2, // Use DB 2 for queue tests to avoid conflicts
	}

	err := cache.Init(cfg)
	require.NoError(t, err, "Failed to connect to test Redis")

	return NewStreamQueue(cache.Client)
}

// cleanupTestRedis flushes the test database
func cleanupTestRedis(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	err := cache.Client.FlushDB(ctx).Err()
	require.NoError(t, err, "Failed to flush test Redis DB")
}

func TestStreamQueue_DeclareStream(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	stream := "test:stream"
	group := "test-group"

	// First declaration should succeed
	err := q.DeclareStream(ctx, stream, group)
	require.NoError(t, err)

	// Second declaration should also succeed (idempotent)
	err = q.DeclareStream(ctx, stream, group)
	require.NoError(t, err)
}

func TestStreamQueue_Publish(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	stream := "test:publish"
	data := []byte("hello world")

	// Publish a message
	msgID, err := q.Publish(ctx, stream, data)
	require.NoError(t, err)
	assert.NotEmpty(t, msgID)

	// Verify message exists in stream
	// Read directly from Redis to verify
	result, err := cache.Client.XRange(ctx, stream, "-", "+").Result()
	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, msgID, result[0].ID)
	assert.Equal(t, data, []byte(result[0].Values["data"].(string)))
}

func TestStreamQueue_PublishMultiple(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	stream := "test:publish:multiple"

	// Publish multiple messages
	messageCount := 5
	msgIDs := make([]string, messageCount)
	for i := 0; i < messageCount; i++ {
		data := []byte(fmt.Sprintf("message-%d", i))
		msgID, err := q.Publish(ctx, stream, data)
		require.NoError(t, err)
		msgIDs[i] = msgID
	}

	// Verify all messages exist
	result, err := cache.Client.XLen(ctx, stream).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(messageCount), result)
}

func TestStreamQueue_Consume_SingleMessage(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := "test:consume:single"
	group := "test-group"
	consumer := "test-consumer-1"

	// Declare stream and group
	err := q.DeclareStream(ctx, stream, group)
	require.NoError(t, err)

	// Publish a message
	expectedData := []byte("test message")
	msgID, err := q.Publish(ctx, stream, expectedData)
	require.NoError(t, err)

	// Track received message
	var receivedData []byte
	var receivedMsgID string
	var wg sync.WaitGroup
	wg.Add(1)

	// Handler function
	handler := func(messageID string, data []byte) error {
		receivedMsgID = messageID
		receivedData = data
		wg.Done()
		cancel() // Stop consumer after receiving message
		return nil
	}

	// Start consumer in goroutine
	go func() {
		_ = q.Consume(ctx, stream, group, consumer, handler)
	}()

	// Wait for message to be processed
	wg.Wait()

	// Verify message was received correctly
	assert.Equal(t, msgID, receivedMsgID)
	assert.Equal(t, expectedData, receivedData)
}

func TestStreamQueue_Consume_MultipleMessages(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := "test:consume:multiple"
	group := "test-group"
	consumer := "test-consumer-1"

	// Declare stream and group
	err := q.DeclareStream(ctx, stream, group)
	require.NoError(t, err)

	// Publish multiple messages
	messageCount := 5
	for i := 0; i < messageCount; i++ {
		data := []byte(fmt.Sprintf("message-%d", i))
		_, err := q.Publish(ctx, stream, data)
		require.NoError(t, err)
	}

	// Track received messages
	receivedCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(messageCount)

	// Handler function
	handler := func(messageID string, data []byte) error {
		mu.Lock()
		receivedCount++
		count := receivedCount
		mu.Unlock()
		wg.Done()
		if count == messageCount {
			cancel() // Stop after all messages
		}
		return nil
	}

	// Start consumer
	go func() {
		_ = q.Consume(ctx, stream, group, consumer, handler)
	}()

	// Wait for all messages
	wg.Wait()

	assert.Equal(t, messageCount, receivedCount)
}

func TestStreamQueue_Consume_HandlerError(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream := "test:consume:error"
	group := "test-group"
	consumer := "test-consumer-1"

	// Declare stream and group
	err := q.DeclareStream(ctx, stream, group)
	require.NoError(t, err)

	// Publish a message
	data := []byte("test message")
	_, err = q.Publish(ctx, stream, data)
	require.NoError(t, err)

	// Track handler calls
	callCount := 0
	var mu sync.Mutex

	// Handler that returns error
	handler := func(messageID string, data []byte) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return errors.New("handler error")
	}

	// Start consumer
	go func() {
		_ = q.Consume(ctx, stream, group, consumer, handler)
	}()

	// Wait a bit for processing
	time.Sleep(500 * time.Millisecond)

	// Handler should have been called at least once
	mu.Lock()
	assert.GreaterOrEqual(t, callCount, 1)
	mu.Unlock()

	// Create new context for pending check (old one may be cancelled)
	checkCtx := context.Background()

	// Message should NOT be ACKed (still in pending list)
	pending, err := cache.Client.XPending(checkCtx, stream, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pending.Count, "Message should remain pending when handler fails")
}

func TestStreamQueue_Consume_MultipleConsumers(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := "test:consume:multi-consumer"
	group := "test-group"

	// Declare stream and group
	err := q.DeclareStream(ctx, stream, group)
	require.NoError(t, err)

	// Publish multiple messages
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		data := []byte(fmt.Sprintf("message-%d", i))
		_, err := q.Publish(ctx, stream, data)
		require.NoError(t, err)
	}

	// Track received messages per consumer
	var mu sync.Mutex
	consumer1Count := 0
	consumer2Count := 0
	var wg sync.WaitGroup
	wg.Add(messageCount)

	// Handler for consumer 1
	handler1 := func(messageID string, data []byte) error {
		mu.Lock()
		consumer1Count++
		total := consumer1Count + consumer2Count
		mu.Unlock()
		wg.Done()
		if total == messageCount {
			cancel()
		}
		return nil
	}

	// Handler for consumer 2
	handler2 := func(messageID string, data []byte) error {
		mu.Lock()
		consumer2Count++
		total := consumer1Count + consumer2Count
		mu.Unlock()
		wg.Done()
		if total == messageCount {
			cancel()
		}
		return nil
	}

	// Start two consumers
	go func() {
		_ = q.Consume(ctx, stream, group, "consumer-1", handler1)
	}()
	go func() {
		_ = q.Consume(ctx, stream, group, "consumer-2", handler2)
	}()

	// Wait for all messages
	wg.Wait()

	// At least one consumer should have processed messages
	// Note: Due to timing, one consumer might process all messages before the other starts
	// This is acceptable behavior for load balancing
	assert.Equal(t, messageCount, consumer1Count+consumer2Count, "Total messages should match")
}

func TestStreamQueue_Consume_JSONMessages(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := "test:consume:json"
	group := "test-group"
	consumer := "test-consumer-1"

	// Declare stream and group
	err := q.DeclareStream(ctx, stream, group)
	require.NoError(t, err)

	// Define test message struct
	type TestMessage struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	expectedMsg := TestMessage{
		ID:   "123",
		Name: "John Doe",
		Age:  30,
	}

	// Publish JSON message
	jsonData, err := json.Marshal(expectedMsg)
	require.NoError(t, err)

	_, err = q.Publish(ctx, stream, jsonData)
	require.NoError(t, err)

	// Track received message
	var receivedMsg TestMessage
	var wg sync.WaitGroup
	wg.Add(1)

	// Handler function
	handler := func(messageID string, data []byte) error {
		err := json.Unmarshal(data, &receivedMsg)
		if err != nil {
			return err
		}
		wg.Done()
		cancel()
		return nil
	}

	// Start consumer
	go func() {
		_ = q.Consume(ctx, stream, group, consumer, handler)
	}()

	// Wait for message
	wg.Wait()

	// Verify message was deserialized correctly
	assert.Equal(t, expectedMsg.ID, receivedMsg.ID)
	assert.Equal(t, expectedMsg.Name, receivedMsg.Name)
	assert.Equal(t, expectedMsg.Age, receivedMsg.Age)
}

func TestStreamQueue_ReclaimPendingMessages(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx := context.Background()
	stream := "test:reclaim"
	group := "test-group"

	// Declare stream and group
	err := q.DeclareStream(ctx, stream, group)
	require.NoError(t, err)

	// Publish a message
	expectedData := []byte("test message for reclaim")
	msgID, err := q.Publish(ctx, stream, expectedData)
	require.NoError(t, err)

	// Read message without ACKing (simulate crashed consumer)
	messages, err := cache.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "crashed-consumer",
		Streams:  []string{stream, ">"},
		Count:    1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, messages, 1)

	// Message should be in pending state
	pending, err := cache.Client.XPending(ctx, stream, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pending.Count, "Message should be pending after read without ACK")

	// Track if message was reclaimed and processed
	var reclaimedData []byte
	var reclaimedMsgID string
	var mu sync.Mutex
	processed := false

	// Handler that will process the reclaimed message
	handler := func(messageID string, data []byte) error {
		mu.Lock()
		reclaimedData = data
		reclaimedMsgID = messageID
		processed = true
		mu.Unlock()
		return nil
	}

	// Call reclaimPendingMessages directly
	// Note: This won't reclaim because MinIdle is 5 minutes and message is fresh
	// This tests that the method executes without error
	err = q.reclaimPendingMessages(ctx, stream, group, "recovery-consumer", handler)
	require.NoError(t, err, "reclaimPendingMessages should execute without error")

	// Message should still be pending (MinIdle not exceeded)
	assert.False(t, processed, "Message should not be reclaimed yet (MinIdle = 5 min)")

	// For actual reclaim testing, manually claim with 0 MinIdle
	// This verifies the reclaim logic would work if MinIdle was exceeded
	claimed, _, err := cache.Client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    group,
		Consumer: "recovery-consumer",
		MinIdle:  0, // Claim immediately for test
		Start:    "0-0",
		Count:    100,
	}).Result()
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, msgID, claimed[0].ID)

	// Process the claimed message through handleMessage (simulating what reclaimPendingMessages does)
	q.handleMessage(ctx, stream, group, claimed[0], handler)

	// Verify message was processed
	assert.True(t, processed, "Message should be processed after manual claim")
	assert.Equal(t, msgID, reclaimedMsgID)
	assert.Equal(t, expectedData, reclaimedData)

	// Message should now be ACKed (pending count = 0)
	pending, err = cache.Client.XPending(ctx, stream, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending.Count, "Message should be ACKed after processing")
}

func TestStreamQueue_MessageOrdering(t *testing.T) {
	q := setupTestRedis(t)
	defer cleanupTestRedis(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream := "test:ordering"
	group := "test-group"
	consumer := "test-consumer-1"

	// Declare stream and group
	err := q.DeclareStream(ctx, stream, group)
	require.NoError(t, err)

	// Publish messages in order
	messageCount := 10
	for i := 0; i < messageCount; i++ {
		data := []byte(fmt.Sprintf("%d", i))
		_, err := q.Publish(ctx, stream, data)
		require.NoError(t, err)
	}

	// Track received messages order
	var receivedOrder []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(messageCount)

	// Handler function
	handler := func(messageID string, data []byte) error {
		mu.Lock()
		receivedOrder = append(receivedOrder, string(data))
		count := len(receivedOrder)
		mu.Unlock()
		wg.Done()
		if count == messageCount {
			cancel()
		}
		return nil
	}

	// Start consumer
	go func() {
		_ = q.Consume(ctx, stream, group, consumer, handler)
	}()

	// Wait for all messages
	wg.Wait()

	// Verify order
	assert.Len(t, receivedOrder, messageCount)
	for i := 0; i < messageCount; i++ {
		assert.Equal(t, fmt.Sprintf("%d", i), receivedOrder[i], "Messages should be received in order")
	}
}
