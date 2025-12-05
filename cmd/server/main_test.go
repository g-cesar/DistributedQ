package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/guido-cesarano/distributedq/pkg/queue"
)

func TestAuthMiddleware(t *testing.T) {
	// Setup mock Redis
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer s.Close()

	client := queue.NewClient(s.Addr())
	apiKey := "secret-key"
	mux := setupRouter(client, apiKey)

	tests := []struct {
		name           string
		headerKey      string
		headerValue    string
		expectedStatus int
	}{
		{
			name:           "No API Key",
			headerKey:      "",
			headerValue:    "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Wrong API Key",
			headerKey:      "X-API-Key",
			headerValue:    "wrong-key",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Correct API Key",
			headerKey:      "X-API-Key",
			headerValue:    "secret-key",
			expectedStatus: http.StatusBadRequest, // 400 because body is empty, but auth passed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/enqueue", nil)
			if tt.headerKey != "" {
				req.Header.Set(tt.headerKey, tt.headerValue)
			}

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestAuthDisabled(t *testing.T) {
	// Setup mock Redis
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	defer s.Close()

	client := queue.NewClient(s.Addr())
	apiKey := "" // Disabled
	mux := setupRouter(client, apiKey)

	req := httptest.NewRequest("POST", "/enqueue", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should pass auth and hit handler (which returns 400 Bad Request for empty body)
	// or 405 if method check fails first?
	// Handler checks method first. We sent POST.
	// Then checks body. Empty body -> 400.
	// If auth failed, we'd get 401.
	if w.Code == http.StatusUnauthorized {
		t.Errorf("Expected auth to be disabled, got 401")
	}
}
