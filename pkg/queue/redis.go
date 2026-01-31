package queue

import (
	"btc-giftcard/pkg/logger"
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// StreamQueue wraps Redis client for stream-based message queue operations
type StreamQueue struct {
	client *redis.Client
}

// NewStreamQueue creates a new StreamQueue instance with the provided Redis client
func NewStreamQueue(client *redis.Client) *StreamQueue {
	return &StreamQueue{client: client}
}

// DeclareStream ensures a consumer group exists for the given stream
// Creates the consumer group if it doesn't exist
// Handles BUSYGROUP error gracefully (group already exists)
func (q *StreamQueue) DeclareStream(ctx context.Context, stream string, group string) error {
	err := q.client.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil {
		// BUSYGROUP means the group already exists - that's fine
		if strings.Contains(err.Error(), "BUSYGROUP") {
			logger.Info("Consumer group already exists", zap.String("stream", stream), zap.String("group", group))
			return nil
		}
		logger.Error("Failed to create consumer group", zap.String("stream", stream), zap.String("group", group), zap.Error(err))
		return err
	}
	logger.Info("Consumer group created successfully", zap.String("stream", stream), zap.String("group", group))
	return nil
}

// Publish adds a message to the specified stream
// Returns the generated message ID
func (q *StreamQueue) Publish(ctx context.Context, stream string, data []byte) (string, error) {
	args := &redis.XAddArgs{
		Stream: stream,
		MaxLen: 10000,
		Approx: true,
		ID:     "*",
		Values: map[string]interface{}{
			"data": data,
		},
	}
	id, err := q.client.XAdd(ctx, args).Result()
	if err != nil {
		logger.Error("Failed to publish message to stream", zap.String("stream", stream), zap.Error(err))
		return "", err
	}

	logger.Info("Published message to stream", zap.String("stream", stream), zap.String("messageID", id))
	return id, nil
}

// Consume starts consuming messages from the stream as part of a consumer group
// Runs in a blocking loop until context is cancelled
// Handler is called for each message; if it returns nil, message is ACKed
func (q *StreamQueue) Consume(ctx context.Context, stream string, group string, consumer string, handler func(messageID string, data []byte) error) error {
	args := &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    10,
		Block:    time.Second * 5,
	}

	doWork := func() error {
		res, err := q.client.XReadGroup(ctx, args).Result()
		if err != nil {
			if err == redis.Nil {
				return nil
			}
			logger.Error("Failed to read from stream", zap.String("stream", stream), zap.Error(err))
			return err
		}

		for _, xstream := range res {
			for _, msg := range xstream.Messages {
				q.handleMessage(ctx, stream, group, msg, handler)
			}
		}
		return nil
	}

	counter := 0
	for {
		select {
		case <-ctx.Done():
			logger.Info("Context cancelled, stopping consumer", zap.String("stream", stream), zap.String("consumer", consumer))
			return nil
		default:
			counter++
			if counter%10 == 0 {
				q.reclaimPendingMessages(ctx, stream, group, consumer, handler)
			}
			if err := doWork(); err != nil {
				logger.Error("Error in consume loop", zap.Error(err))
			}
		}
	}
}

// reclaimPendingMessages recovers messages that were delivered but not acknowledged
// (e.g., worker crashed before ACKing)
func (q *StreamQueue) reclaimPendingMessages(ctx context.Context, stream string, group string, consumer string, handler func(messageID string, data []byte) error) error {
	args := &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    group,
		MinIdle:  time.Minute * 5,
		Start:    "0-0",
		Consumer: consumer,
		Count:    100,
	}

	res, _, err := q.client.XAutoClaim(ctx, args).Result()
	if err != nil {
		if err == redis.Nil {
			return nil
		}
		logger.Error("Failed to read idle messages", zap.String("stream", stream), zap.Error(err))
		return err
	}
	for _, msg := range res {
		q.handleMessage(ctx, stream, group, msg, handler)
	}
	return nil
}

func (q *StreamQueue) handleMessage(ctx context.Context, stream string, group string, msg redis.XMessage, handler func(messageID string, data []byte) error) {
	dataValue, ok := msg.Values["data"]
	if !ok {
		logger.Error("Message missing 'data' field", zap.String("messageID", msg.ID))
		q.client.XAck(ctx, stream, group, msg.ID)
		return
	}

	dataBytes, ok := dataValue.(string)
	if !ok {
		logger.Error("Message 'data' field is not a string", zap.String("messageID", msg.ID))
		q.client.XAck(ctx, stream, group, msg.ID)
		return
	}

	logger.Info("Processing message", zap.String("messageID", msg.ID), zap.String("stream", stream))
	err := handler(msg.ID, []byte(dataBytes))
	if err == nil {
		q.client.XAck(ctx, stream, group, msg.ID)
		logger.Info("Message processed successfully", zap.String("messageID", msg.ID))
	} else {
		logger.Error("Handler failed to process message", zap.String("messageID", msg.ID), zap.Error(err))
	}
}
