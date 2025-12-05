package integration_tests

import (
	"context"
	"testing"
	"time"

	"github.com/guido-cesarano/distributedq/pkg/queue"
	"github.com/guido-cesarano/distributedq/pkg/tasks"
	"github.com/redis/go-redis/v9"
)

// setupIntegrationRedis connects to the local Redis instance.
// Requires docker-compose up -d to be running.
func setupIntegrationRedis(t *testing.T) *queue.Client {
	// Check if Redis is reachable
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping integration test: Redis not reachable at localhost:6379 (%v)", err)
	}

	// Clear queues for clean state
	rdb.Del(context.Background(), "main_queue", "processing_queue", "delayed_queue", "dead_letter_queue")

	return queue.NewClient("localhost:6379")
}

func TestIntegrationFlow(t *testing.T) {
	client := setupIntegrationRedis(t)
	ctx := context.Background()

	// 1. Enqueue Task
	task := tasks.Task{
		ID:        "integration-test-1",
		Type:      "integration",
		Payload:   map[string]string{"msg": "hello"},
		CreatedAt: time.Now(),
	}

	if err := client.Enqueue(ctx, task); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// 2. Dequeue Task
	dequeuedTask, raw, err := client.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if dequeuedTask.ID != task.ID {
		t.Errorf("Expected ID %s, got %s", task.ID, dequeuedTask.ID)
	}

	// 3. Ack Task
	if err := client.Ack(ctx, raw); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}

	// Verify queues are empty
	depths := client.GetQueueDepths(ctx)
	if depths["main_queue"] != 0 {
		t.Errorf("Expected main_queue empty, got %d", depths["main_queue"])
	}
	if depths["processing_queue"] != 0 {
		t.Errorf("Expected processing_queue empty, got %d", depths["processing_queue"])
	}
}
