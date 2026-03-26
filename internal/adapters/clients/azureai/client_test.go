package azureai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/ai"
)

func TestStreamCompletion_RetriesResponsesAPI_MidStreamFailure(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		if attempt == 1 {
			// Stream a text delta then drop (no response.completed).
			fmt.Fprint(w, "event: response.output_text.delta\n")
			fmt.Fprint(w, "data: {\"delta\":\"partial\"}\n\n")
			return
		}

		// Second attempt: complete successfully.
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, "data: {\"delta\":\"full result\"}\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"response\":{\"id\":\"resp_123\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n")
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint:       server.URL + "/test.services.ai.azure.com",
		APIKey:         "test-key",
		DeploymentName: "test-model",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	var chunks []ai.CompletionChunk
	err = client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {
		chunks = append(chunks, chunk)
	})

	if err != nil {
		t.Fatalf("StreamCompletion returned error: %v", err)
	}
	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts.Load())
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.Done {
		t.Error("last chunk should be Done")
	}
}

func TestStreamCompletion_NoRetryAfterResponseCompleted(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, "data: {\"delta\":\"result\"}\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"response\":{\"id\":\"resp_456\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n")
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint:       server.URL + "/test.services.ai.azure.com",
		APIKey:         "test-key",
		DeploymentName: "test-model",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = client.StreamCompletion(context.Background(), ai.CompletionRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hello"}},
	}, func(chunk ai.CompletionChunk) {})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts.Load())
	}
}
