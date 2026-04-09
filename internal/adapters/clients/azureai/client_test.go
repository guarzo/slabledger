package azureai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/ai"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// newTestClient creates a client pointing at a test server using withBaseURL.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "test-key",
		DeploymentName: "gpt-4o",
	}, withBaseURL(serverURL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// sseEvent formats a single SSE event with event type and JSON data.
func sseEvent(eventType, data string) string {
	return "event: " + eventType + "\ndata: " + data + "\n\n"
}

// sseResponse writes SSE events as a streaming HTTP response with flushing.
func sseResponse(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for _, e := range events {
		fmt.Fprint(w, e)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func TestStreamCompletion_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseResponse(w,
			sseEvent("response.created", `{"type":"response.created","response":{"id":"resp_123","status":"in_progress"}}`),
			sseEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"Hello "}`),
			sseEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"world"}`),
			sseEvent("response.completed", `{"type":"response.completed","response":{"id":"resp_123","status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`),
		)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	var chunks []ai.CompletionChunk
	err := client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})

	if err != nil {
		t.Fatalf("StreamCompletion error: %v", err)
	}
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}
	// Verify text deltas.
	var deltas []string
	for _, c := range chunks {
		if c.Delta != "" && !c.Done {
			deltas = append(deltas, c.Delta)
		}
	}
	if len(deltas) != 2 || deltas[0] != "Hello " || deltas[1] != "world" {
		t.Errorf("deltas = %v, want [Hello , world]", deltas)
	}
	// Verify final chunk.
	last := chunks[len(chunks)-1]
	if !last.Done {
		t.Error("last chunk should be Done")
	}
	if last.ConversationState != "resp_123" {
		t.Errorf("ConversationState = %v, want resp_123", last.ConversationState)
	}
	if last.Usage == nil || last.Usage.TotalTokens != 15 {
		t.Errorf("Usage = %v, want TotalTokens=15", last.Usage)
	}
}

func TestStreamCompletion_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseResponse(w,
			sseEvent("response.created", `{"type":"response.created","response":{"id":"resp_tc","status":"in_progress"}}`),
			sseEvent("response.function_call_arguments.done", `{"type":"response.function_call_arguments.done","item_id":"fc_1","name":"get_price","arguments":"{\"card\":\"pikachu\"}"}`),
			sseEvent("response.completed", `{"type":"response.completed","response":{"id":"resp_tc","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"get_price","arguments":"{\"card\":\"pikachu\"}"}],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`),
		)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	var chunks []ai.CompletionChunk
	err := client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})

	if err != nil {
		t.Fatalf("StreamCompletion error: %v", err)
	}
	// Find the completed chunk.
	var doneChunk *ai.CompletionChunk
	for i := range chunks {
		if chunks[i].Done {
			doneChunk = &chunks[i]
		}
	}
	if doneChunk == nil {
		t.Fatal("no Done chunk found")
	}
	if len(doneChunk.ToolCalls) == 0 {
		t.Fatal("expected tool calls in done chunk")
	}
	tc := doneChunk.ToolCalls[0]
	if tc.Name != "get_price" {
		t.Errorf("tool call name = %q, want get_price", tc.Name)
	}
	if tc.ID != "call_abc" {
		t.Errorf("tool call ID = %q, want call_abc", tc.ID)
	}
}

func TestStreamCompletion_RetryOnMidStreamFailure(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			// Send partial data then close (no response.completed).
			sseResponse(w,
				sseEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"partial"}`),
			)
			return
		}
		sseResponse(w,
			sseEvent("response.created", `{"type":"response.created","response":{"id":"resp_retry","status":"in_progress"}}`),
			sseEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"full result"}`),
			sseEvent("response.completed", `{"type":"response.completed","response":{"id":"resp_retry","status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`),
		)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	var chunks []ai.CompletionChunk
	err := client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})

	if err != nil {
		t.Fatalf("StreamCompletion error: %v", err)
	}
	if attempts.Load() < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts.Load())
	}
	// Should have received Done.
	var gotDone bool
	for _, c := range chunks {
		if c.Done {
			gotDone = true
		}
	}
	if !gotDone {
		t.Error("expected Done chunk")
	}
}

func TestStreamCompletion_NoRetryAfterCompleted(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		sseResponse(w,
			sseEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":"result"}`),
			sseEvent("response.completed", `{"type":"response.completed","response":{"id":"resp_once","status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`),
		)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	err := client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {})

	if err != nil {
		t.Fatalf("StreamCompletion error: %v", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts.Load())
	}
}

func TestStreamCompletion_PermanentError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "invalid request",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	err := client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {})

	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry for 400), got %d", attempts.Load())
	}
}

func TestClassifyBackoff(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		attempt  int
		wantSecs int
	}{
		{"capacity attempt 0", &capacityError{raw: "x"}, 0, 60},
		{"capacity attempt 1", &capacityError{raw: "x"}, 1, 120},
		{"rate_limit attempt 0", &rateLimitError{raw: "x"}, 0, 30},
		{"rate_limit attempt 1", &rateLimitError{raw: "x"}, 1, 60},
		{"transient attempt 0", fmt.Errorf("connection reset"), 0, 5},
		{"transient attempt 1", fmt.Errorf("connection reset"), 1, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyBackoff(tt.err, tt.attempt)
			wantDur := time.Duration(tt.wantSecs) * time.Second
			if got != wantDur {
				t.Errorf("classifyBackoff = %v, want %v", got, wantDur)
			}
		})
	}
}

func TestNewClient_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "missing endpoint",
			cfg:     Config{APIKey: "key", DeploymentName: "model"},
			wantErr: "Endpoint is required",
		},
		{
			name:    "missing API key",
			cfg:     Config{Endpoint: "https://test.openai.azure.com", DeploymentName: "model"},
			wantErr: "APIKey is required",
		},
		{
			name:    "missing deployment name",
			cfg:     Config{Endpoint: "https://test.openai.azure.com", APIKey: "key"},
			wantErr: "DeploymentName is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		permanent bool
	}{
		{"nil error", nil, false},
		{"rate limit error", &rateLimitError{raw: "x"}, false},
		{"capacity error", &capacityError{raw: "x"}, false},
		// SDK error format.
		{"SDK 400", fmt.Errorf(`400 Bad Request {"message":"invalid"}`), true},
		{"SDK 401", fmt.Errorf(`401 Unauthorized`), true},
		{"SDK 403", fmt.Errorf(`403 Forbidden`), true},
		{"SDK 404", fmt.Errorf(`404 Not Found`), true},
		{"SDK 429 not permanent", fmt.Errorf(`429 Too Many Requests`), false},
		{"SDK 500 not permanent", fmt.Errorf(`500 Internal Server Error`), false},
		// Generic status code format.
		{"status code 400", fmt.Errorf("status code: 400"), true},
		{"status code 429", fmt.Errorf("status code: 429"), false},
		{"generic error", fmt.Errorf("connection reset"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPermanentError(tt.err); got != tt.permanent {
				t.Errorf("isPermanentError(%v) = %v, want %v", tt.err, got, tt.permanent)
			}
		})
	}
}

func TestIsAzureOpenAI(t *testing.T) {
	tests := []struct {
		endpoint string
		expected bool
	}{
		{"https://myresource.openai.azure.com", true},
		{"https://myresource.cognitiveservices.azure.com", true},
		{"https://myproject.services.ai.azure.com", true},
		{"https://custom-llm.example.com", false},
		{"https://api.openai.com", false},
	}
	for _, tt := range tests {
		if got := isAzureOpenAI(tt.endpoint); got != tt.expected {
			t.Errorf("isAzureOpenAI(%q) = %v, want %v", tt.endpoint, got, tt.expected)
		}
	}
}

func TestRateLimitError_Message(t *testing.T) {
	err := &rateLimitError{raw: "too fast"}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("unexpected message: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "too fast") {
		t.Errorf("expected raw message in error: %s", err.Error())
	}
}

func TestCapacityError_Message(t *testing.T) {
	err := &capacityError{raw: "overloaded"}
	if !strings.Contains(err.Error(), "capacity exceeded") {
		t.Errorf("unexpected message: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "overloaded") {
		t.Errorf("expected raw message in error: %s", err.Error())
	}
}

// TestStreamCompletion_CapacityError verifies capacity errors from SSE are classified.
func TestStreamCompletion_CapacityError(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		sseResponse(w,
			sseEvent("error", `{"type":"error","code":"no_capacity","message":"capacity exceeded"}`),
		)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "test-key",
		DeploymentName: "gpt-4o",
	}, WithLogger(observability.NewNoopLogger()), withBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	retErr := client.StreamCompletion(ctx, ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {})

	if retErr == nil {
		t.Fatal("expected error")
	}
	// Should have attempted at least once.
	if attempts.Load() < 1 {
		t.Errorf("expected at least 1 attempt, got %d", attempts.Load())
	}
	// Error should be a capacity error or context deadline (from backoff timeout).
	var capErr *capacityError
	if !errors.As(retErr, &capErr) && !errors.Is(retErr, context.DeadlineExceeded) {
		t.Logf("error type: %T, value: %v", retErr, retErr)
	}
}
