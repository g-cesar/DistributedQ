# API Documentation

## Overview

DistributedQ provides a simple HTTP REST API for enqueuing tasks to the distributed queue system.

**Base URL:** `http://localhost:8081`

---

## Endpoints

### POST /enqueue

Enqueues a new task to the distributed queue for asynchronous processing.

#### Request

**Headers:**
```
Content-Type: application/json
X-API-Key: <your-api-key>
```

**Body:**
```json
{
  "type": "string",      // Required. Task type identifier (e.g., "email", "notification")
  "priority": 1,         // Optional. Priority level: 2 (High), 1 (Default), 0 (Low)
  "payload": object      // Required. Task-specific data as JSON object
}
```

#### Response

**Success (200 OK):**
```
Task enqueued: <task-uuid>
```

**Error Responses:**

| Status Code | Description |
|-------------|-------------|
| 400 Bad Request | Invalid JSON or missing required fields |
| 401 Unauthorized | Missing or invalid API Key |
| 405 Method Not Allowed | HTTP method is not POST |
| 500 Internal Server Error | Redis connection failure or internal error |

| 405 Method Not Allowed | HTTP method is not POST |
| 500 Internal Server Error | Redis connection failure or internal error |

### POST /schedule

Schedules a recurring task using a cron expression.

#### Request

**Body:**
```json
{
  "spec": "@every 1m",   // Required. Cron expression (e.g., "* * * * *", "@daily")
  "type": "string",      // Required. Task type
  "payload": object      // Required. Task payload
}
```

#### Response

**Success (200 OK):**
```
Job scheduled with EntryID: <id>
```

### GET /result

Retrieves the result of a completed task.

#### Request

**Query Parameters:**
- `id`: The UUID of the task to retrieve the result for.

**Example:** `GET /result?id=8651ba0e-8b8a-4119-9a91-abb036b7f7e0`

#### Response

**Success (200 OK):**
```json
{
  "status": "completed",
  "data": { ... }
}
```

**Error Responses:**
- `404 Not Found`: Result not found or expired (TTL 24h)
- `400 Bad Request`: Missing task ID

- `400 Bad Request`: Missing task ID
- `405 Method Not Allowed`: Invalid HTTP method
- `500 Internal Server Error`: Redis error

### GET /stats

Retrieves current depth (number of tasks) for all queues.

#### Request

**Headers:**
```
X-API-Key: <your-api-key>
```

#### Response

**Success (200 OK):**
```json
{
  "queue:default": 5,
  "queue:high": 0,
  "queue:low": 0,
  "processing_queue": 1,
  "delayed_queue": 0,
  "dead_letter_queue": 0,
  "completed_queue": 10
}
```

### GET /tasks

Inspects the content of a specific queue. Returns the top 50 tasks.

#### Request

**Query Parameters:**
- `queue`: Name of the queue (e.g., `queue:default`, `processing_queue`, `completed_queue`)

**Headers:**
```
X-API-Key: <your-api-key>
```

**Example:** `GET /tasks?queue=completed_queue`

#### Response

**Success (200 OK):**
```json
[
  {
    "id": "uuid...",
    "type": "email",
    "priority": 1,
    "created_at": "2023-10-27T...",
    "retry_count": 0,
    "payload": {...}
  },
  ...
]
```

---

## Task Types

The `type` field is used to route tasks to appropriate handlers in the worker. You can define any custom task types based on your application needs.

### Common Task Types

#### Email
```json
{
  "type": "email",
  "payload": {
    "to": "user@example.com",
    "from": "noreply@example.com",
    "subject": "Welcome!",
    "body": "Thanks for signing up",
    "html": true
  }
}
```

#### Notification
```json
{
  "type": "notification",
  "payload": {
    "user_id": 12345,
    "message": "Your order has shipped",
    "type": "push"
  }
}
```

#### Image Resize
```json
{
  "type": "image_resize",
  "payload": {
    "source_url": "https://example.com/image.jpg",
    "target_width": 800,
    "target_height": 600,
    "format": "webp"
  }
}
```

#### Webhook
```json
{
  "type": "webhook",
  "payload": {
    "url": "https://api.example.com/callback",
    "method": "POST",
    "body": {
      "event": "user.created",
      "user_id": 123
    }
  }
}
```

---

## Examples

### Using curl

```bash
curl -X POST http://localhost:8081/enqueue \
  -H "Content-Type: application/json" \
  -d '{
    "type": "email",
    "payload": {
      "to": "user@example.com",
      "subject": "Test Email"
    }
    "type": "email",
    "priority": 2,
    "payload": {
      "to": "user@example.com",
      "subject": "Test Email"
    }
  }'
```

### Scheduling a Task
```bash
curl -X POST http://localhost:8081/schedule \
  -H "Content-Type: application/json" \
  -d '{
    "spec": "@every 5m",
    "type": "cleanup",
    "payload": {}
  }'
```

### Using Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

type EnqueueRequest struct {
    Type    string      `json:"type"`
    Payload interface{} `json:"payload"`
}

func enqueueTask(taskType string, payload interface{}) error {
    req := EnqueueRequest{
        Type:    taskType,
        Payload: payload,
    }
    
    body, err := json.Marshal(req)
    if err != nil {
        return err
    }
    
    resp, err := http.Post(
        "http://localhost:8081/enqueue",
        "application/json",
        bytes.NewBuffer(body),
    )
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("Failed to enqueue: %s", resp.Status)
    }
    
    return nil
}

func main() {
    payload := map[string]interface{}{
        "to":      "user@example.com",
        "subject": "Hello",
    }
    
    err := enqueueTask("email", payload)
    if err != nil {
        panic(err)
    }
    
    fmt.Println("Task enqueued successfully")
}
```

### Using Python

```python
import requests
import json

def enqueue_task(task_type, payload):
    url = "http://localhost:8081/enqueue"
    
    data = {
        "type": task_type,
        "payload": payload
    }
    
    response = requests.post(url, json=data)
    
    if response.status_code == 200:
        print(f"Task enqueued: {response.text}")
    else:
        print(f"Error: {response.status_code} - {response.text}")

# Example usage
payload = {
    "to": "user@example.com",
    "subject": "Hello from Python"
}

enqueue_task("email", payload)
```

### Using JavaScript (Node.js)

```javascript
const axios = require('axios');

async function enqueueTask(taskType, payload) {
    try {
        const response = await axios.post('http://localhost:8081/enqueue', {
            type: taskType,
            payload: payload
        });
        
        console.log(`Task enqueued: ${response.data}`);
    } catch (error) {
        console.error(`Error: ${error.response?.status} - ${error.message}`);
    }
}

// Example usage
const payload = {
    to: 'user@example.com',
    subject: 'Hello from Node.js'
};

enqueueTask('email', payload);
```

---

## Rate Limiting

Currently, DistributedQ does not implement rate limiting at the API level. Consider implementing rate limiting at the reverse proxy level (e.g., nginx, HAProxy) for production deployments.

---

## Authentication

DistributedQ uses API Key authentication. You must provide the `X-API-Key` header with every request.

**Configuration:**
Set the `API_KEY` environment variable on the server.

**Header:**
```
X-API-Key: your-secret-key
```

**Example:**
```bash
curl -H "X-API-Key: your-secret-key" ...
```

---

## Best Practices

1. **Include Task IDs**: Store the returned task UUID for tracking
2. **Idempotency**: Design tasks to be idempotent when possible
3. **Payload Size**: Keep payloads small (< 1MB recommended)
4. **Error Handling**: Always check HTTP status codes
5. **Timeouts**: Set reasonable HTTP client timeouts (e.g., 5 seconds)

---

## Monitoring API Health

Check if the API server is running:

```bash
curl http://localhost:8081/enqueue -X GET
```

Expected response: `405 Method Not Allowed` (confirms server is up)

For a proper health check endpoint, add:

```go
http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, "OK")
})
```
