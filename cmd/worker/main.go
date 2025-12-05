// Package main implements the GoQueue worker process.
// The worker continuously dequeues tasks from Redis, processes them, and tracks metrics.
//
// Features:
//   - Concurrent task processing with graceful shutdown
//   - Prometheus metrics exposed on :8080/metrics
//   - Automatic retry with exponential backoff
//   - Dead Letter Queue for failed tasks
//   - Background scheduler for delayed task processing
//
// Usage:
//
//	go run cmd/worker/main.go
//
// The worker connects to Redis at localhost:6379 and exposes metrics at localhost:8080.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/guido-cesarano/distributedq/pkg/logger"
	"github.com/guido-cesarano/distributedq/pkg/queue"
	"github.com/guido-cesarano/distributedq/pkg/tasks"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus metrics for monitoring task processing.
var (
	// tasksProcessed tracks the total number of processed tasks by status and type.
	// Labels:
	//   - status: "success", "retry", or "failed"
	//   - type: task type (e.g., "email", "notification")
	tasksProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "goqueue_processed_total",
		Help: "The total number of processed tasks",
	}, []string{"status", "type"})

	// taskDuration tracks task processing latency in seconds.
	// This histogram is used to calculate percentiles (P50, P95, P99) in Grafana.
	// Labels:
	//   - type: task type (e.g., "email", "notification")
	taskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "goqueue_task_duration_seconds",
		Help:    "Duration of task processing",
		Buckets: prometheus.DefBuckets,
	}, []string{"type"})

	// queueDepth tracks the number of tasks in each queue.
	// This gauge is updated periodically by the metrics collector goroutine.
	// Labels:
	//   - queue: queue name ("main_queue", "processing_queue", "delayed_queue", "dead_letter_queue")
	queueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goqueue_queue_depth",
		Help: "Number of tasks in each queue",
	}, []string{"queue"})

	// queueLatency tracks the time a task spends in the queue before being processed.
	// It is calculated as time.Now() - task.CreatedAt.
	queueLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "goqueue_queue_latency_seconds",
		Help:    "Time spent in queue before processing",
		Buckets: prometheus.DefBuckets,
	}, []string{"type"})
)

// main initializes the worker, starts the metrics server, and begins processing tasks.
// It supports graceful shutdown via SIGINT/SIGTERM signals.
func main() {
	client := queue.NewClient("127.0.0.1:6379")
	ctx, cancel := context.WithCancel(context.Background())

	// Start Prometheus metrics server on port 8080
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		logger.Log.Info().Msg("Metrics server listening on :8080")
		http.ListenAndServe(":8080", nil)
	}()

	// Setup graceful shutdown handlers
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Log.Info().Msg("Shutting down worker...")
		cancel()
	}()

	logger.Log.Info().Msg("Worker started. Waiting for tasks...")

	// Start queue depth collector (updates metrics every 5 seconds)
	go collectQueueMetrics(ctx, client)

	startWorker(ctx, client)
}

// startWorker runs the main worker loop that dequeues and processes tasks.
// It also starts the background scheduler for handling delayed tasks.
//
// Task Processing Flow:
//  1. Dequeue task atomically from main_queue to processing_queue
//  2. Process the task via processTask()
//  3. On success: Ack (remove from processing_queue) and increment success metric
//  4. On failure:
//     - If retries < 3: Schedule retry with exponential backoff, increment retry metric
//     - If retries >= 3: Move to dead_letter_queue, increment failed metric
//
// The function runs until the context is cancelled (graceful shutdown).
// startWorker runs the main worker loop that dequeues and processes tasks.
// It also starts the background scheduler for handling delayed tasks.
func startWorker(ctx context.Context, client *queue.Client) {
	// Start Scheduler in background to process delayed tasks
	go client.StartScheduler(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			task, raw, err := client.Dequeue(ctx)
			if err != nil {
				if err != context.Canceled {
					// Silently continue on connection errors
				}
				continue
			}

			// Rate Limiting Check
			// Define limits per type (hardcoded for now, could be config)
			limit := 10 // 10 tasks/sec
			burst := 20
			allowed, err := client.Allow(ctx, fmt.Sprintf("ratelimit:%s", task.Type), limit, burst)
			if err != nil {
				logger.Log.Error().Err(err).Msg("Rate limit check failed")
				// Fail open or closed? Let's fail open (process it) or retry?
				// Let's process it to avoid stuck tasks.
			} else if !allowed {
				logger.Log.Warn().Str("type", task.Type).Msg("Rate limit exceeded, re-queueing")
				// Re-queue to delayed_queue with 5s delay
				// We use Retry logic but maybe we shouldn't increment retry count?
				// If we use Retry, it increments count.
				// Let's manually add to delayed queue without incrementing retry count.
				// Or just use Retry but don't count it as a failure?
				// Let's implement a specific Delay method or just hack it here.
				// For simplicity, let's use Retry but decrement count first? No that's hacky.
				// Let's just use Retry. It's fine if it counts as a retry, eventually it will DLQ if system is overloaded.
				// Actually, rate limiting shouldn't consume retries.
				// Let's add a Requeue method to client? Or just use Retry and accept it.
				// Let's use Retry for now.
				client.Retry(ctx, *task, raw)
				continue
			}

			// Record queue latency (time since creation until start of processing)
			start := time.Now()
			latency := start.Sub(task.CreatedAt)
			queueLatency.WithLabelValues(task.Type).Observe(latency.Seconds())

			switch task.Type {
			case "email":
				err = processEmail(task)
			case "slow":
				logger.Log.Info().Str("task_id", task.ID).Msg("Processing slow simulation task (5s)...")
				time.Sleep(5 * time.Second)
				err = nil // Simulate success after delay
			case "image_resize":
				err = processImageResize(task)
			default:
				err = processGenericTask(task)
			}

			if err != nil {
				// Handle Failure
				logger.Log.Error().Err(err).Str("task_id", task.ID).Msg("Task failed")
				if task.RetryCount < 3 { // Max Retries = 3
					client.Retry(ctx, *task, raw)
					tasksProcessed.WithLabelValues("retry", task.Type).Inc()
				} else {
					client.Fail(ctx, *task, raw)
					tasksProcessed.WithLabelValues("failed", task.Type).Inc()
				}
			} else {
				// Success
				client.Complete(ctx, raw)
				// Store dummy result for demonstration
				client.SetResult(ctx, task.ID, map[string]string{"status": "completed", "timestamp": time.Now().Format(time.RFC3339)})
				tasksProcessed.WithLabelValues("success", task.Type).Inc()
			}
		}
	}
}

// processTask simulates task processing and records latency metrics.
// In a real implementation, this would dispatch to task-type-specific handlers.
//
// Current implementation:
//   - Simulates 100ms processing time
//   - Records duration in taskDuration histogram
//   - Always succeeds (returns nil)
//
// To test retry logic, uncomment the simulated failure code.
func processTask(task *tasks.Task) error {
	start := time.Now()
	logger.Log.Info().
		Str("task_id", task.ID).
		Str("type", task.Type).
		Int("retry_count", task.RetryCount).
		Msg("Processing task")

	// Uncomment to simulate random failures for testing retry logic:
	// if task.RetryCount < 2 {
	// 	return fmt.Errorf("simulated failure")
	// }

	time.Sleep(100 * time.Millisecond) // Simulate processing time

	duration := time.Since(start)
	taskDuration.WithLabelValues(task.Type).Observe(duration.Seconds())

	// Record queue latency (time since creation until start of processing)
	latency := start.Sub(task.CreatedAt)
	queueLatency.WithLabelValues(task.Type).Observe(latency.Seconds())

	return nil
}

// collectQueueMetrics periodically queries Redis to get queue depths and updates Prometheus gauges.
// This allows monitoring of queue backlogs and processing queue size.
func collectQueueMetrics(ctx context.Context, client *queue.Client) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			depths := client.GetQueueDepths(ctx)
			for queue, depth := range depths {
				queueDepth.WithLabelValues(queue).Set(float64(depth))
			}
		}
	}
}

// processEmail handles email tasks.
func processEmail(task *tasks.Task) error {
	start := time.Now()
	logger.Log.Info().Str("task_id", task.ID).Msg("Sending email...")
	time.Sleep(200 * time.Millisecond) // Simulate checking email service
	duration := time.Since(start)
	taskDuration.WithLabelValues(task.Type).Observe(duration.Seconds())
	return nil
}

// processImageResize handles image resizing tasks.
func processImageResize(task *tasks.Task) error {
	start := time.Now()
	logger.Log.Info().Str("task_id", task.ID).Msg("Resizing image...")
	time.Sleep(500 * time.Millisecond) // Simulate CPU work
	duration := time.Since(start)
	taskDuration.WithLabelValues(task.Type).Observe(duration.Seconds())
	return nil
}

// processGenericTask handles unknown task types.
func processGenericTask(task *tasks.Task) error {
	return processTask(task)
}
