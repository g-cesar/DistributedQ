// Package main provides a benchmark tool for GoQueue to measure task processing throughput.
// It enqueues a large number of dummy tasks and measures completion time.
//
// Usage:
//
//	go run benchmark/main.go -tasks 100000
package main

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/guido-cesarano/distributedq/pkg/queue"
	"github.com/guido-cesarano/distributedq/pkg/tasks"
)

func main() {
	numTasks := flag.Int("tasks", 100000, "Number of tasks to enqueue")
	numWorkers := flag.Int("workers", 10, "Number of concurrent enqueuers")
	flag.Parse()

	client := queue.NewClient("localhost:6379")
	ctx := context.Background()

	fmt.Printf("GoQueue Benchmark\n")
	fmt.Printf("=================\n")
	fmt.Printf("Tasks to enqueue: %d\n", *numTasks)
	fmt.Printf("Concurrent workers: %d\n\n", *numWorkers)

	// Enqueue phase
	fmt.Printf("Starting enqueue phase...\n")
	startEnqueue := time.Now()

	var wg sync.WaitGroup
	var enqueued atomic.Int64
	tasksPerWorker := *numTasks / *numWorkers

	for i := 0; i < *numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < tasksPerWorker; j++ {
				task := tasks.Task{
					ID:        uuid.New().String(),
					Type:      "benchmark",
					Payload:   map[string]interface{}{"worker": workerID, "task": j},
					CreatedAt: time.Now(),
				}
				if err := client.Enqueue(ctx, task); err != nil {
					fmt.Printf("Error enqueuing: %v\n", err)
					return
				}
				enqueued.Add(1)
			}
		}(i)
	}

	wg.Wait()
	enqueueTime := time.Since(startEnqueue)

	fmt.Printf("✓ Enqueued %d tasks in %s\n", enqueued.Load(), enqueueTime)
	fmt.Printf("  Throughput: %.2f tasks/sec\n\n", float64(enqueued.Load())/enqueueTime.Seconds())

	// Wait for processing
	fmt.Printf("Waiting for all tasks to be processed...\n")
	startProcess := time.Now()

	// Poll Redis until main_queue and processing_queue are empty
	for {
		depths := client.GetQueueDepths(ctx)
		remaining := depths["main_queue"] + depths["processing_queue"]

		if remaining == 0 {
			break
		}

		// Print progress every 2 seconds
		time.Sleep(2 * time.Second)
		fmt.Printf("  Remaining: %d tasks\n", remaining)
	}

	processTime := time.Since(startProcess)

	fmt.Printf("\n✓ All tasks processed in %s\n", processTime)
	fmt.Printf("  Throughput: %.2f tasks/sec\n", float64(*numTasks)/processTime.Seconds())

	totalTime := enqueueTime + processTime
	fmt.Printf("\nTotal time: %s\n", totalTime)
	fmt.Printf("Overall throughput: %.2f tasks/sec\n", float64(*numTasks)/totalTime.Seconds())
}
