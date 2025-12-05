package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/guido-cesarano/distributedq/pkg/tasks"
	"github.com/redis/go-redis/v9"
)

func setupTestRedis() (*miniredis.Miniredis, *Client) {
	s, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	client := NewClient(s.Addr())
	return s, client
}

func TestEnqueue(t *testing.T) {
	s, client := setupTestRedis()
	defer s.Close()

	ctx := context.Background()
	task := tasks.Task{
		ID:        "test-id",
		Type:      "email",
		Payload:   map[string]string{"to": "test@example.com"},
		CreatedAt: time.Now(),
		Priority:  tasks.PriorityDefault,
	}

	err := client.Enqueue(ctx, task)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Verify task is in Redis using direct redis client connection to miniredis
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	len, _ := rdb.LLen(ctx, "queue:default").Result()
	if len != 1 {
		t.Errorf("Expected queue:default length 1, got %d", len)
	}
}

func TestPriorityDequeue(t *testing.T) {
	s, client := setupTestRedis()
	defer s.Close()
	ctx := context.Background()

	// Enqueue tasks with different priorities
	highTask := tasks.Task{ID: "high", Type: "test", Priority: tasks.PriorityHigh}
	defaultTask := tasks.Task{ID: "default", Type: "test", Priority: tasks.PriorityDefault}
	lowTask := tasks.Task{ID: "low", Type: "test", Priority: tasks.PriorityLow}

	client.Enqueue(ctx, lowTask)
	client.Enqueue(ctx, highTask)
	client.Enqueue(ctx, defaultTask)

	// Dequeue 1: Should be High
	task1, _, err := client.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue 1 failed: %v", err)
	}
	if task1.ID != "high" {
		t.Errorf("Expected high task, got %s", task1.ID)
	}

	// Dequeue 2: Should be Default
	task2, _, err := client.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue 2 failed: %v", err)
	}
	if task2.ID != "default" {
		t.Errorf("Expected default task, got %s", task2.ID)
	}

	// Dequeue 3: Should be Low
	task3, _, err := client.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue 3 failed: %v", err)
	}
	if task3.ID != "low" {
		t.Errorf("Expected low task, got %s", task3.ID)
	}
}

func TestRetry(t *testing.T) {
	s, client := setupTestRedis()
	defer s.Close()

	ctx := context.Background()
	task := tasks.Task{
		ID:         "test-id",
		RetryCount: 0,
	}
	rawTask := "{}"

	err := client.Retry(ctx, task, rawTask)
	if err != nil {
		t.Fatalf("Retry failed: %v", err)
	}

	// Verify task added to delayed_queue
	exists := s.Exists("delayed_queue")
	if !exists {
		t.Error("Expected delayed_queue to exist")
	}

	// Verify score is in future
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	tasks, _ := rdb.ZRangeWithScores(ctx, "delayed_queue", 0, -1).Result()
	if len(tasks) != 1 {
		t.Fatalf("Expected 1 task in delayed_queue, got %d", len(tasks))
	}

	if tasks[0].Score <= float64(time.Now().UnixNano()) {
		t.Error("Expected task score to be in the future")
	}
}

func TestTaskResult(t *testing.T) {
	s, client := setupTestRedis()
	defer s.Close()
	ctx := context.Background()

	taskID := "result-test-id"
	expectedResult := map[string]string{"status": "success"}

	// Set Result
	err := client.SetResult(ctx, taskID, expectedResult)
	if err != nil {
		t.Fatalf("SetResult failed: %v", err)
	}

	// Get Result
	resultJSON, err := client.GetResult(ctx, taskID)
	if err != nil {
		t.Fatalf("GetResult failed: %v", err)
	}

	var result map[string]string
	err = json.Unmarshal([]byte(resultJSON), &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["status"] != "success" {
		t.Errorf("Expected status success, got %s", result["status"])
	}

	// Verify TTL (miniredis supports TTL)
	ttl := s.TTL("result:" + taskID)
	if ttl == 0 {
		t.Error("Expected TTL to be set")
	}
}

func TestSchedule(t *testing.T) {
	s, client := setupTestRedis()
	defer s.Close()

	client.StartCronScheduler()
	defer client.StopCronScheduler()

	ctx := context.Background()
	task := tasks.Task{
		ID:   "scheduled-task",
		Type: "cron",
	}

	// Schedule to run every second
	_, err := client.Schedule(ctx, "@every 1s", task)
	if err != nil {
		t.Fatalf("Schedule failed: %v", err)
	}

	// Wait for 2 seconds to allow job to run
	time.Sleep(2 * time.Second)

	// Verify task is in Redis
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	len, _ := rdb.LLen(ctx, "queue:default").Result()
	if len < 1 {
		t.Errorf("Expected at least 1 scheduled task, got %d", len)
	}
}

func TestRateLimit(t *testing.T) {
	s, client := setupTestRedis()
	defer s.Close()
	ctx := context.Background()

	key := "ratelimit:test"
	limit := 1 // 1 token per second
	burst := 1 // Capacity 1

	// First call should succeed
	allowed, err := client.Allow(ctx, key, limit, burst)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !allowed {
		t.Error("Expected first call to be allowed")
	}

	// Second call immediately after should fail (burst consumed)
	allowed, err = client.Allow(ctx, key, limit, burst)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if allowed {
		t.Error("Expected second call to be denied")
	}

	// Wait for refill (1.1s)
	time.Sleep(1100 * time.Millisecond)

	// Third call should succeed
	allowed, err = client.Allow(ctx, key, limit, burst)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if !allowed {
		t.Error("Expected third call to be allowed after refill")
	}
}
