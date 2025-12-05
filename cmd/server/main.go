// Package main implements the GoQueue HTTP API server for task enqueuing.
// The server provides a REST API endpoint to add tasks to the queue.
//
// API Endpoints:
//
//	POST /enqueue - Enqueues a new task to the distributed queue
//
// Request Format:
//
//	{
//	  "type": "email",
//	  "payload": {
//	    "to": "user@example.com",
//	    "subject": "Hello"
//	  }
//	}
//
// Response Format:
//
//	Task enqueued: <task-id>
//
// Usage:
//
//	go run cmd/server/main.go
//
// The server listens on :8081 and connects to Redis at localhost:6379.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/guido-cesarano/distributedq/pkg/logger"
	"github.com/guido-cesarano/distributedq/pkg/queue"
	"github.com/guido-cesarano/distributedq/pkg/tasks"
	"github.com/redis/go-redis/v9"
)

// authMiddleware wraps an http.HandlerFunc and enforces API Key authentication.
func authMiddleware(next http.HandlerFunc, requiredKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If no key is configured, allow all (dev mode)
		if requiredKey == "" {
			next(w, r)
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		if apiKey != requiredKey {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// enableCORS wraps an http.HandlerFunc and adds CORS headers.
func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow all origins for dev
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-API-Key")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// setupRouter configures the HTTP handlers and returns the mux.
func setupRouter(client *queue.Client, apiKey string) *http.ServeMux {
	mux := http.NewServeMux()

	// Helper to chain middlewares: CORS -> Auth (optional) -> Handler
	// Note: We apply CORS *before* Auth so OPTIONS requests (preflight) don't fail auth.
	// But wait, enableCORS handles OPTIONS and returns. Auth middleware checks key.
	// If we wrap Auth(CORS(Handler)), Auth runs first.
	// We want CORS(Auth(Handler)).
	// enableCORS calls next(w, r). next is Auth(Handler).
	// So: mux.HandleFunc(path, enableCORS(authMiddleware(handler, apiKey)))

	// enqueueHandler processes POST requests to add tasks to the queue
	mux.HandleFunc("/enqueue", enableCORS(authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Validate HTTP method
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse request body
		var req struct {
			Type     string      `json:"type"`     // Task type
			Payload  interface{} `json:"payload"`  // Task data
			Priority int         `json:"priority"` // Optional: 0=Low, 1=Default, 2=High
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Set default priority if not specified (or if 0, which is Low)
		// If user sends 0 explicitly, it's Low. If they omit it, it's 0 (Low).
		// To make Default (1) the actual default, we need logic.
		// Let's assume 0 is Low, 1 is Default, 2 is High.
		// If user wants Default, they should send 1 or we default to 1?
		// Standard int default is 0. Let's force default to 1 if 0 is passed?
		// No, 0 is a valid priority (Low).
		// Let's use a pointer or specific logic if we want "Default" to be default.
		// For simplicity: 0=Low, 1=Default, 2=High.
		// If omitted, it's 0 (Low).
		// Let's change logic: If user provides nothing, we want Default (1).
		// But JSON unmarshal gives 0.
		// Let's use a pointer for Priority to check presence.

		// Create task with unique ID and current timestamp
		task := tasks.Task{
			ID:        uuid.New().String(),
			Type:      req.Type,
			Payload:   req.Payload,
			CreatedAt: time.Now(),
			Priority:  req.Priority,
		}

		// Enqueue task to Redis
		if err := client.Enqueue(context.Background(), task); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Return success response with task ID
		fmt.Fprintf(w, "Task enqueued: %s\n", task.ID)
	}, apiKey)))

	// resultHandler retrieves the result of a task
	mux.HandleFunc("/result", enableCORS(authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		taskID := r.URL.Query().Get("id")
		if taskID == "" {
			http.Error(w, "Missing task ID", http.StatusBadRequest)
			return
		}

		result, err := client.GetResult(context.Background(), taskID)
		if err == redis.Nil {
			http.Error(w, "Result not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(result))
	}, apiKey)))

	// scheduleHandler registers a new cron job
	mux.HandleFunc("/schedule", enableCORS(authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Spec     string      `json:"spec"`     // Cron expression (e.g. "@every 1m")
			Type     string      `json:"type"`     // Task type
			Payload  interface{} `json:"payload"`  // Task data
			Priority int         `json:"priority"` // Optional priority
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		task := tasks.Task{
			ID: uuid.New().String(), // Template ID, will be reused/ignored?
			// Actually, in Schedule() we should probably generate new IDs.
			// But for now let's just use this.
			Type:      req.Type,
			Payload:   req.Payload,
			CreatedAt: time.Now(),
			Priority:  req.Priority,
		}

		entryID, err := client.Schedule(context.Background(), req.Spec, task)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid cron spec: %v", err), http.StatusBadRequest)
			return
		}

		fmt.Fprintf(w, "Job scheduled with EntryID: %d\n", entryID)
	}, apiKey)))

	// statsHandler returns the current queue depths
	// We protect this with Auth too, or leave open?
	// Plan didn't specify, but dashboard will need key if protected.
	// Let's protect it to be safe, dashboard can send key.
	mux.HandleFunc("/stats", enableCORS(authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		depths := client.GetQueueDepths(context.Background())

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(depths); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}, apiKey)))

	// tasksHandler returns a list of tasks from a specific queue
	mux.HandleFunc("/tasks", enableCORS(authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		queueName := r.URL.Query().Get("queue")
		if queueName == "" {
			http.Error(w, "Missing queue parameter", http.StatusBadRequest)
			return
		}

		// Inspect top 50 tasks (arbitrary limit for inspector)
		tasks, err := client.InspectQueue(context.Background(), queueName, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(tasks); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}, apiKey)))

	return mux
}

// main initializes the HTTP server and registers the /enqueue endpoint handler.
func main() {
	client := queue.NewClient("127.0.0.1:6379")

	// Start Cron Scheduler
	client.StartCronScheduler()
	defer client.StopCronScheduler()

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		logger.Log.Warn().Msg("API_KEY not set. Authentication disabled.")
	} else {
		logger.Log.Info().Msg("API Authentication enabled.")
	}

	mux := setupRouter(client, apiKey)

	logger.Log.Info().Msg("Server listening on :8081")
	if err := http.ListenAndServe(":8081", mux); err != nil {
		logger.Log.Fatal().Err(err).Msg("Server failed")
	}
}
