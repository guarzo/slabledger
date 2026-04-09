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

// newResponsesTestClient creates a Responses API client pointing at a test server.
func newResponsesTestClient(t *testing.T, serverURL string, opts ...Option) *Client {
	t.Helper()
	client, err := NewClient(Config{
		Endpoint:       serverURL + "/test.services.ai.azure.com",
		APIKey:         "test-key",
		DeploymentName: "test-model",
	}, opts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// --- StreamCompletion: Responses API ---

func TestStreamCompletion_ResponsesAPI(t *testing.T) {
	tests := []struct {
		name             string
		handler          func(attempts *atomic.Int32) http.HandlerFunc
		expectedAttempts int32
		expectDone       bool
	}{
		{
			name: "retries on mid-stream failure",
			handler: func(attempts *atomic.Int32) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					attempt := attempts.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					if attempt == 1 {
						fmt.Fprint(w, "event: response.output_text.delta\n")
						fmt.Fprint(w, "data: {\"delta\":\"partial\"}\n\n")
						return
					}
					fmt.Fprint(w, "event: response.output_text.delta\n")
					fmt.Fprint(w, "data: {\"delta\":\"full result\"}\n\n")
					fmt.Fprint(w, "event: response.completed\n")
					fmt.Fprint(w, "data: {\"response\":{\"id\":\"resp_123\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n")
				}
			},
			expectedAttempts: 2,
			expectDone:       true,
		},
		{
			name: "no retry after response completed",
			handler: func(attempts *atomic.Int32) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					attempts.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, "event: response.output_text.delta\n")
					fmt.Fprint(w, "data: {\"delta\":\"result\"}\n\n")
					fmt.Fprint(w, "event: response.completed\n")
					fmt.Fprint(w, "data: {\"response\":{\"id\":\"resp_456\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n")
				}
			},
			expectedAttempts: 1,
			expectDone:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(tt.handler(&attempts))
			defer server.Close()

			client := newResponsesTestClient(t, server.URL)

			var chunks []ai.CompletionChunk
			err := client.StreamCompletion(context.Background(), ai.CompletionRequest{
				Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
			}, func(chunk ai.CompletionChunk) {
				chunks = append(chunks, chunk)
			})

			if err != nil {
				t.Fatalf("StreamCompletion error: %v", err)
			}
			if attempts.Load() != tt.expectedAttempts {
				t.Errorf("attempts = %d, want %d", attempts.Load(), tt.expectedAttempts)
			}
			if len(chunks) == 0 {
				t.Fatal("expected at least one chunk")
			}
			if lastChunk := chunks[len(chunks)-1]; lastChunk.Done != tt.expectDone {
				t.Errorf("last chunk Done = %v, want %v", lastChunk.Done, tt.expectDone)
			}
		})
	}
}

func TestStreamCompletion_RateLimitRetryWithBackoff(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, "rate limited")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, "data: {\"delta\":\"ok\"}\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"response\":{\"id\":\"resp_789\"}}\n\n")
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.StreamCompletion(ctx, ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {})

	if err == nil {
		t.Fatal("expected error due to timeout during backoff")
	}
	if attempts.Load() < 1 {
		t.Errorf("expected at least 1 attempt, got %d", attempts.Load())
	}
}

func TestStreamCompletion_CapacityErrorBackoff(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if attempt == 1 {
			// Emit a capacity error via SSE.
			fmt.Fprint(w, "event: error\n")
			fmt.Fprint(w, `data: {"error":{"type":"server_error","code":"no_capacity"}}`+"\n\n")
			return
		}
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, "data: {\"delta\":\"ok\"}\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"response\":{\"id\":\"resp_cap\"}}\n\n")
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.StreamCompletion(ctx, ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {})

	if err == nil {
		t.Fatal("expected error due to timeout during capacity backoff")
	}
}

func TestStreamCompletion_PermanentErrorNoRetry(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "bad request")
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	err := client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {})

	if err == nil {
		t.Fatal("expected permanent error")
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry for 400), got %d", attempts.Load())
	}
}

// --- StreamCompletion: Chat Completions API ---

func TestStreamCompletion_ChatCompletions_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Chat completions format
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	// Not .services.ai.azure.com → uses Chat Completions API
	client, err := NewClient(Config{
		Endpoint:       server.URL + "/test.openai.azure.com",
		APIKey:         "test-key",
		DeploymentName: "gpt-4o",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	var deltas []string
	var gotDone bool
	err = client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {
		if chunk.Delta != "" {
			deltas = append(deltas, chunk.Delta)
		}
		if chunk.Done {
			gotDone = true
		}
	})

	if err != nil {
		t.Fatalf("StreamCompletion error: %v", err)
	}
	if !gotDone {
		t.Error("expected Done chunk")
	}
	if len(deltas) != 2 || deltas[0] != "Hello " || deltas[1] != "world" {
		t.Errorf("deltas = %v, want [Hello , world]", deltas)
	}
}

func TestStreamCompletion_ChatCompletions_NoRetryAfterEmit(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		// Emit one chunk then disconnect without [DONE]
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint:       server.URL + "/test.openai.azure.com",
		APIKey:         "test-key",
		DeploymentName: "gpt-4o",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {})

	// Chat completions: emitted chunk prevents retry, so no error is returned
	// (the stream parser emits Done: true on EOF).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry for chat completions after emit), got %d", attempts.Load())
	}
}

// --- parseSSEStream (Chat Completions) ---

func TestParseSSEStream_ValidEvents(t *testing.T) {
	input := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":" world"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	client, _ := NewClient(Config{
		Endpoint:       "https://test.openai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	var chunks []ai.CompletionChunk
	err := client.parseSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3", len(chunks))
	}
	if chunks[0].Delta != "Hello" {
		t.Errorf("chunk[0].Delta = %q, want %q", chunks[0].Delta, "Hello")
	}
	if chunks[1].Delta != " world" {
		t.Errorf("chunk[1].Delta = %q, want %q", chunks[1].Delta, " world")
	}
	if !chunks[2].Done {
		t.Error("last chunk should be Done")
	}
}

func TestParseSSEStream_MalformedJSON(t *testing.T) {
	input := "data: {invalid json}\n\ndata: [DONE]\n\n"

	client, _ := NewClient(Config{
		Endpoint:       "https://test.openai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	}, WithLogger(observability.NewNoopLogger()))

	var chunks []ai.CompletionChunk
	err := client.parseSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}
	// Malformed chunk is skipped, [DONE] emits final chunk.
	if len(chunks) != 1 || !chunks[0].Done {
		t.Errorf("expected 1 Done chunk, got %d chunks", len(chunks))
	}
}

func TestParseSSEStream_ToolCalls(t *testing.T) {
	// Simulate incremental tool call accumulation.
	input := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_price","arguments":""}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"card\":"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"pikachu\"}"}}]}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	client, _ := NewClient(Config{
		Endpoint:       "https://test.openai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	var chunks []ai.CompletionChunk
	err := client.parseSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}

	// Final chunk should contain the accumulated tool call.
	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.Done {
		t.Error("last chunk should be Done")
	}
	if len(lastChunk.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(lastChunk.ToolCalls))
	}
	tc := lastChunk.ToolCalls[0]
	if tc.Name != "get_price" {
		t.Errorf("tool call name = %q, want %q", tc.Name, "get_price")
	}
	if tc.Arguments != `{"card":"pikachu"}` {
		t.Errorf("tool call arguments = %q, want %q", tc.Arguments, `{"card":"pikachu"}`)
	}
}

func TestParseSSEStream_EmptyChoices(t *testing.T) {
	input := "data: {\"choices\":[]}\n\ndata: [DONE]\n\n"

	client, _ := NewClient(Config{
		Endpoint:       "https://test.openai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	var chunks []ai.CompletionChunk
	err := client.parseSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}
	// Only the [DONE] chunk.
	if len(chunks) != 1 || !chunks[0].Done {
		t.Errorf("expected 1 Done chunk, got %d chunks", len(chunks))
	}
}

func TestParseSSEStream_NoLinePrefix(t *testing.T) {
	// Lines without "data: " prefix are ignored.
	input := "event: ping\n\n: comment\n\ndata: [DONE]\n\n"

	client, _ := NewClient(Config{
		Endpoint:       "https://test.openai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	var chunks []ai.CompletionChunk
	err := client.parseSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}
	if len(chunks) != 1 || !chunks[0].Done {
		t.Errorf("expected 1 Done chunk, got %v", chunks)
	}
}

func TestParseSSEStream_StreamEndWithoutDone(t *testing.T) {
	// Stream ends without [DONE] — should emit Done: true anyway.
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"

	client, _ := NewClient(Config{
		Endpoint:       "https://test.openai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	}, WithLogger(observability.NewNoopLogger()))

	var chunks []ai.CompletionChunk
	err := client.parseSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("parseSSEStream error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}
	if !chunks[len(chunks)-1].Done {
		t.Error("last chunk should be Done even without [DONE]")
	}
}

// --- parseResponsesSSEStream ---

func TestParseResponsesSSEStream_TextDelta(t *testing.T) {
	input := strings.Join([]string{
		"event: response.created",
		`data: {"response":{"id":"resp_abc"}}`,
		"",
		"event: response.output_text.delta",
		`data: {"delta":"Hello "}`,
		"",
		"event: response.output_text.delta",
		`data: {"delta":"world"}`,
		"",
		"event: response.completed",
		`data: {"response":{"id":"resp_abc","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`,
		"",
	}, "\n")

	client, _ := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	var chunks []ai.CompletionChunk
	result, err := client.parseResponsesSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result.responseID != "resp_abc" {
		t.Errorf("responseID = %q, want %q", result.responseID, "resp_abc")
	}
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3", len(chunks))
	}
	if chunks[0].Delta != "Hello " {
		t.Errorf("chunk[0].Delta = %q", chunks[0].Delta)
	}
	if chunks[1].Delta != "world" {
		t.Errorf("chunk[1].Delta = %q", chunks[1].Delta)
	}
	if !chunks[2].Done {
		t.Error("last chunk should be Done")
	}
	if chunks[2].ConversationState != "resp_abc" {
		t.Errorf("ConversationState = %v, want resp_abc", chunks[2].ConversationState)
	}
	if chunks[2].Usage == nil || chunks[2].Usage.TotalTokens != 15 {
		t.Errorf("Usage = %v, want TotalTokens=15", chunks[2].Usage)
	}
}

func TestParseResponsesSSEStream_FunctionCall(t *testing.T) {
	input := strings.Join([]string{
		"event: response.created",
		`data: {"response":{"id":"resp_fc"}}`,
		"",
		"event: response.output_item.added",
		`data: {"output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"get_price"}}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"fc_1","delta":"{\"card\":"}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"item_id":"fc_1","delta":"\"pikachu\"}"}`,
		"",
		"event: response.completed",
		`data: {"response":{"id":"resp_fc"}}`,
		"",
	}, "\n")

	client, _ := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	var chunks []ai.CompletionChunk
	_, err := client.parseResponsesSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.Done {
		t.Error("last chunk should be Done")
	}
	if len(lastChunk.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(lastChunk.ToolCalls))
	}
	tc := lastChunk.ToolCalls[0]
	if tc.Name != "get_price" {
		t.Errorf("tool call name = %q", tc.Name)
	}
	if tc.ID != "call_abc" {
		t.Errorf("tool call ID = %q, want %q", tc.ID, "call_abc")
	}
	if tc.Arguments != `{"card":"pikachu"}` {
		t.Errorf("tool call arguments = %q", tc.Arguments)
	}
}

func TestParseResponsesSSEStream_ErrorEvent(t *testing.T) {
	input := strings.Join([]string{
		"event: error",
		`data: {"error":{"type":"server_error","code":"rate_limit"}}`,
		"",
	}, "\n")

	client, _ := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	_, err := client.parseResponsesSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {
		t.Error("should not emit chunks on error")
	})
	if err == nil {
		t.Fatal("expected error from SSE error event")
	}
	var rlErr *rateLimitError
	if !errors.As(err, &rlErr) {
		t.Errorf("expected rateLimitError, got %T: %v", err, err)
	}
}

func TestParseResponsesSSEStream_StreamEndWithoutCompleted(t *testing.T) {
	input := strings.Join([]string{
		"event: response.created",
		`data: {"response":{"id":"resp_x"}}`,
		"",
		"event: response.output_text.delta",
		`data: {"delta":"partial"}`,
		"",
	}, "\n")

	client, _ := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	}, WithLogger(observability.NewNoopLogger()))

	_, err := client.parseResponsesSSEStream(context.Background(), strings.NewReader(input), func(chunk ai.CompletionChunk) {})
	if err == nil {
		t.Fatal("expected error for stream ending without response.completed")
	}
	if !errors.Is(err, ErrResponseIncomplete) {
		t.Errorf("expected ErrResponseIncomplete, got: %v", err)
	}
}

func TestParseResponsesSSEStream_ContextCancelled(t *testing.T) {
	// A long-running stream that gets cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	input := strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"delta":"x"}`,
		"",
	}, "\n")

	client, _ := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	_, err := client.parseResponsesSSEStream(ctx, strings.NewReader(input), func(chunk ai.CompletionChunk) {})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// --- parseSSEError ---

func TestParseSSEError_NoCapacity(t *testing.T) {
	client, _ := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	}, WithLogger(observability.NewNoopLogger()))

	data := `{"error":{"type":"server_error","code":"no_capacity"}}`
	err := client.parseSSEError(context.Background(), data)

	var capErr *capacityError
	if !errors.As(err, &capErr) {
		t.Errorf("expected capacityError, got %T: %v", err, err)
	}
}

func TestParseSSEError_RateLimit(t *testing.T) {
	client, _ := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	data := `{"error":{"type":"rate_limit","code":"rate_limit_exceeded"}}`
	err := client.parseSSEError(context.Background(), data)

	var rlErr *rateLimitError
	if !errors.As(err, &rlErr) {
		t.Errorf("expected rateLimitError, got %T: %v", err, err)
	}
}

func TestParseSSEError_MalformedJSON(t *testing.T) {
	client, _ := NewClient(Config{
		Endpoint:       "https://test.services.ai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})

	// Malformed JSON defaults to rateLimitError.
	err := client.parseSSEError(context.Background(), "not json")
	var rlErr *rateLimitError
	if !errors.As(err, &rlErr) {
		t.Errorf("expected rateLimitError for malformed data, got %T: %v", err, err)
	}
}

// --- isPermanentError ---

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		permanent bool
	}{
		{"nil error", nil, false},
		{"rate limit error", &rateLimitError{raw: "x"}, false},
		{"capacity error", &capacityError{raw: "x"}, false},
		{"400 bad request", fmt.Errorf("azure ai returned 400: bad request"), true},
		{"401 unauthorized", fmt.Errorf("azure ai returned 401: unauthorized"), true},
		{"403 forbidden", fmt.Errorf("azure ai returned 403: forbidden"), true},
		{"404 not found", fmt.Errorf("azure ai returned 404: not found"), true},
		{"429 not permanent", fmt.Errorf("azure ai returned 429: too many requests"), false},
		{"500 not permanent", fmt.Errorf("azure ai returned 500: server error"), false},
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

// --- Error type messages ---

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

// --- pollResponseFallback ---

func TestPollResponseFallback_CompletedResponse(t *testing.T) {
	pollResp := map[string]any{
		"id":     "resp_poll",
		"status": "completed",
		"output": []map[string]any{
			{
				"type": "message",
				"content": []map[string]any{
					{"type": "output_text", "text": "Final answer"},
				},
			},
		},
		"usage": map[string]any{
			"input_tokens":  100,
			"output_tokens": 50,
			"total_tokens":  150,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pollResp)
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	var chunks []ai.CompletionChunk
	// Use a generous timeout since poll has a 20s initial wait.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := client.pollResponseFallback(ctx, "resp_poll", func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("pollResponseFallback error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if !chunks[0].Done {
		t.Error("chunk should be Done")
	}
	if chunks[0].Delta != "Final answer" {
		t.Errorf("Delta = %q, want %q", chunks[0].Delta, "Final answer")
	}
	if chunks[0].Usage == nil || chunks[0].Usage.TotalTokens != 150 {
		t.Errorf("Usage = %v, want TotalTokens=150", chunks[0].Usage)
	}
}

func TestPollResponseFallback_FailedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_fail",
			"status": "failed",
		})
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := client.pollResponseFallback(ctx, "resp_fail", func(chunk ai.CompletionChunk) {
		t.Error("should not emit chunks for failed response")
	})
	if err == nil {
		t.Fatal("expected error for failed status")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPollResponseFallback_IncompleteWithOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_inc",
			"status": "incomplete",
			"incomplete_details": map[string]any{
				"reason": "max_output_tokens",
			},
			"output": []map[string]any{
				{
					"type": "message",
					"content": []map[string]any{
						{"type": "output_text", "text": "Partial output"},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks []ai.CompletionChunk
	err := client.pollResponseFallback(ctx, "resp_inc", func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Delta != "Partial output" {
		t.Errorf("Delta = %q, want %q", chunks[0].Delta, "Partial output")
	}
}

func TestPollResponseFallback_IncompleteWithNoOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_inc",
			"status": "incomplete",
			"incomplete_details": map[string]any{
				"reason": "max_output_tokens",
			},
			"output": []map[string]any{},
		})
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := client.pollResponseFallback(ctx, "resp_inc", func(chunk ai.CompletionChunk) {
		t.Error("should not emit chunks for incomplete with no output")
	})
	if err == nil {
		t.Fatal("expected error for incomplete with no output")
	}
}

func TestPollResponseFallback_ToolCallParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_tc",
			"status": "completed",
			"output": []map[string]any{
				{
					"type":      "function_call",
					"name":      "get_price",
					"call_id":   "call_123",
					"arguments": `{"card":"charizard"}`,
				},
			},
		})
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var chunks []ai.CompletionChunk
	err := client.pollResponseFallback(ctx, "resp_tc", func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(chunks[0].ToolCalls))
	}
	tc := chunks[0].ToolCalls[0]
	if tc.Name != "get_price" {
		t.Errorf("tool call name = %q", tc.Name)
	}
	if tc.Arguments != `{"card":"charizard"}` {
		t.Errorf("tool call arguments = %q", tc.Arguments)
	}
}

func TestPollResponseFallback_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 404 to keep polling.
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	// Very short timeout — should cancel during the initial 20s wait.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.pollResponseFallback(ctx, "resp_timeout", func(chunk ai.CompletionChunk) {})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestPollResponseFallback_QueuedThenCompleted(t *testing.T) {
	var pollCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := pollCount.Add(1)
		if count <= 1 {
			json.NewEncoder(w).Encode(map[string]any{
				"id":     "resp_q",
				"status": "in_progress",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id":     "resp_q",
			"status": "completed",
			"output": []map[string]any{
				{
					"type": "message",
					"content": []map[string]any{
						{"type": "output_text", "text": "Done after queued"},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var chunks []ai.CompletionChunk
	err := client.pollResponseFallback(ctx, "resp_q", func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Delta != "Done after queued" {
		t.Errorf("unexpected chunks: %v", chunks)
	}
}

func TestPollResponseFallback_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "internal server error")
	}))
	defer server.Close()

	client := newResponsesTestClient(t, server.URL, WithLogger(observability.NewNoopLogger()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := client.pollResponseFallback(ctx, "resp_500", func(chunk ai.CompletionChunk) {})
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- NewClient validation ---

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

func TestNewClient_DefaultAPIVersion(t *testing.T) {
	client, err := NewClient(Config{
		Endpoint:       "https://test.openai.azure.com",
		APIKey:         "key",
		DeploymentName: "model",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.config.APIVersion != "2024-12-01-preview" {
		t.Errorf("APIVersion = %q, want default", client.config.APIVersion)
	}
}

// --- URL building ---

func TestBuildURL_AzureOpenAI(t *testing.T) {
	client, _ := NewClient(Config{
		Endpoint:       "https://myresource.openai.azure.com",
		APIKey:         "key",
		DeploymentName: "gpt-4o",
		APIVersion:     "2024-12-01-preview",
	})
	url := client.buildURL("https://myresource.openai.azure.com")
	expected := "https://myresource.openai.azure.com/openai/deployments/gpt-4o/chat/completions?api-version=2024-12-01-preview"
	if url != expected {
		t.Errorf("buildURL = %q, want %q", url, expected)
	}
}

func TestBuildURL_NonAzure(t *testing.T) {
	client, _ := NewClient(Config{
		Endpoint:       "https://custom-llm.example.com",
		APIKey:         "key",
		DeploymentName: "model",
	})
	url := client.buildURL("https://custom-llm.example.com")
	if url != "https://custom-llm.example.com/chat/completions" {
		t.Errorf("buildURL = %q", url)
	}
}

func TestBuildResponsesURL_AIFoundry(t *testing.T) {
	client, _ := NewClient(Config{
		Endpoint:       "https://myproject.services.ai.azure.com",
		APIKey:         "key",
		DeploymentName: "gpt-5.4-pro",
	})
	url := client.buildResponsesURL("https://myproject.services.ai.azure.com")
	if url != "https://myproject.services.ai.azure.com/openai/v1/responses" {
		t.Errorf("buildResponsesURL = %q", url)
	}
}

func TestBuildResponsesURL_AzureOpenAI(t *testing.T) {
	client, _ := NewClient(Config{
		Endpoint:       "https://myresource.openai.azure.com",
		APIKey:         "key",
		DeploymentName: "gpt-4o",
	})
	url := client.buildResponsesURL("https://myresource.openai.azure.com")
	expected := fmt.Sprintf("https://myresource.openai.azure.com/openai/deployments/gpt-4o/responses?api-version=%s", responsesAPIVersion)
	if url != expected {
		t.Errorf("buildResponsesURL = %q, want %q", url, expected)
	}
}

// --- isAzureOpenAI ---

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

// --- flattenToolCalls ---

func TestFlattenToolCalls(t *testing.T) {
	t.Run("empty map", func(t *testing.T) {
		result := flattenToolCalls(map[int]*ai.ToolCall{})
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("sorted by index", func(t *testing.T) {
		m := map[int]*ai.ToolCall{
			2: {Name: "b"},
			0: {Name: "a"},
			1: {Name: "c"},
		}
		result := flattenToolCalls(m)
		if len(result) != 3 {
			t.Fatalf("expected 3 results, got %d", len(result))
		}
		if result[0].Name != "a" || result[1].Name != "c" || result[2].Name != "b" {
			t.Errorf("unexpected order: %v", result)
		}
	})
}

// --- Auth header tests ---

func TestAuthHeader_AzureOpenAI(t *testing.T) {
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("api-key")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client, _ := NewClient(Config{
		Endpoint:       server.URL + "/test.openai.azure.com",
		APIKey:         "my-api-key",
		DeploymentName: "model",
	})

	_ = client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}},
	}, func(chunk ai.CompletionChunk) {})

	if gotHeader != "my-api-key" {
		t.Errorf("expected api-key header = %q, got %q", "my-api-key", gotHeader)
	}
}

func TestAuthHeader_AIFoundry(t *testing.T) {
	var gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("api-key")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"response\":{\"id\":\"resp_x\"}}\n\n")
	}))
	defer server.Close()

	// AI Foundry endpoint (.services.ai.azure.com uses api-key header).
	client, _ := NewClient(Config{
		Endpoint:       server.URL + "/test.services.ai.azure.com",
		APIKey:         "my-api-key",
		DeploymentName: "model",
	})

	_ = client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}},
	}, func(chunk ai.CompletionChunk) {})

	if gotAPIKey != "my-api-key" {
		t.Errorf("expected api-key header = %q, got %q", "my-api-key", gotAPIKey)
	}
}
