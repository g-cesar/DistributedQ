// Package tasks defines the core data structures for task representation in the GoQueue system.
// Tasks are units of work that can be enqueued, processed by workers, and retried on failure.
package tasks

import (
	"time"
)

// Task represents a unit of work to be processed by the distributed task queue.
// Each task contains metadata for tracking, routing, and retry logic.
//
// The Type field is used to route tasks to appropriate handlers, while the Payload
// contains the actual job-specific data. The RetryCount is automatically incremented
// by the queue system when a task fails and needs to be retried.
type Task struct {
	// ID is a unique identifier for the task (typically UUID).
	ID string `json:"id"`

	// Type categorizes the task for routing and metrics (e.g., "email", "notification").
	Type string `json:"type"`

	// Payload contains the job-specific data as a generic interface.
	// Workers are responsible for type assertion based on the Type field.
	Payload interface{} `json:"payload"`

	// CreatedAt is the timestamp when the task was first enqueued.
	CreatedAt time.Time `json:"created_at"`

	// RetryCount tracks how many times this task has been retried after failures.
	// The queue system uses this to implement exponential backoff and max retry limits.
	RetryCount int `json:"retry_count"`

	// Priority determines the processing order of the task.
	// Higher priority tasks are processed before lower priority ones.
	// 0 = Low, 1 = Default, 2 = High
	Priority int `json:"priority"`
}

const (
	PriorityLow     = 0
	PriorityDefault = 1
	PriorityHigh    = 2
)
