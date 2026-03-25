package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/platform/resilience"
)

func BenchmarkClient_Get(b *testing.B) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("benchmark")
	client := NewClient(config)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Get(ctx, server.URL, nil, 5*time.Second)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_GetJSON(b *testing.B) {
	type Response struct {
		Status string `json:"status"`
		Value  int    `json:"value"`
	}

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok", "value": 42}`))
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("benchmark")
	client := NewClient(config)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var result Response
		err := client.GetJSON(ctx, server.URL, nil, 5*time.Second, &result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_Post(b *testing.B) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created": true}`))
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("benchmark")
	client := NewClient(config)

	ctx := context.Background()
	body := []byte(`{"name": "test"}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Post(ctx, server.URL, nil, body, 5*time.Second)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_WithRetry(b *testing.B) {
	attempts := int32(0)

	// Create test server that fails once then succeeds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count%2 == 1 {
			// First attempt of each request fails
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Second attempt succeeds
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Create client with fast retry
	config := DefaultConfig("benchmark")
	config.RetryPolicy = resilience.RetryPolicy{
		MaxRetries:     2,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		BackoffFactor:  2.0,
	}
	client := NewClient(config)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Get(ctx, server.URL, nil, 5*time.Second)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_WithObserver(b *testing.B) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Create client with observer
	observer := &TestObserver{}
	config := DefaultConfig("benchmark")
	config.Observer = observer
	client := NewClient(config)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.Get(ctx, server.URL, nil, 5*time.Second)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_ConcurrentRequests(b *testing.B) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Create client
	config := DefaultConfig("benchmark")
	client := NewClient(config)

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.Get(ctx, server.URL, nil, 5*time.Second)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// Benchmark comparison: httpx vs raw http.Client
func BenchmarkRawHTTPClient_Get(b *testing.B) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Create raw HTTP client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
		req.Header.Set("User-Agent", "slabledger/1.0")
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Body.Close()
	}
}
