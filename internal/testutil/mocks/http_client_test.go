package mocks_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestMockHTTPClient_GetJSON_Success(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.AddResponse("https://api.test.com/data", mocks.MockHTTPResponse{
		StatusCode: 200,
		Body:       `{"message": "success"}`,
	})

	var result struct {
		Message string `json:"message"`
	}

	err := mock.GetJSON(context.Background(), "https://api.test.com/data", nil, 0, &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result.Message != "success" {
		t.Errorf("expected message 'success', got: %s", result.Message)
	}
}

func TestMockHTTPClient_GetJSON_Error(t *testing.T) {
	expectedErr := fmt.Errorf("network error")
	mock := mocks.NewMockHTTPClient(mocks.WithError(expectedErr))

	var result map[string]interface{}
	err := mock.GetJSON(context.Background(), "https://api.test.com/data", nil, 0, &result)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != expectedErr.Error() {
		t.Errorf("expected error %v, got: %v", expectedErr, err)
	}
}

func TestMockHTTPClient_HTTPError(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.AddResponse("https://api.test.com/notfound", mocks.MockHTTPResponse{
		StatusCode: 404,
		Body:       "not found",
	})

	var result map[string]interface{}
	err := mock.GetJSON(context.Background(), "https://api.test.com/notfound", nil, 0, &result)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "HTTP 404: not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMockHTTPClient_Delay(t *testing.T) {
	mock := mocks.NewMockHTTPClient(mocks.WithDelay(100 * time.Millisecond))
	mock.SetDefaultResponse(mocks.MockHTTPResponse{
		StatusCode: 200,
		Body:       `{"ok": true}`,
	})

	start := time.Now()
	var result map[string]interface{}
	_ = mock.GetJSON(context.Background(), "https://api.test.com/slow", nil, 0, &result)
	duration := time.Since(start)

	if duration < 100*time.Millisecond {
		t.Errorf("expected delay of at least 100ms, got %v", duration)
	}
}

func TestMockHTTPClient_Timeout(t *testing.T) {
	mock := mocks.NewMockHTTPClient(mocks.WithDelay(5 * time.Second))
	mock.SetDefaultResponse(mocks.MockHTTPResponse{
		StatusCode: 200,
		Body:       `{"ok": true}`,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var result map[string]interface{}
	err := mock.GetJSON(ctx, "https://api.test.com/slow", nil, 0, &result)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestMockHTTPClient_CallLog(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.SetDefaultResponse(mocks.MockHTTPResponse{
		StatusCode: 200,
		Body:       `{}`,
	})

	headers := map[string]string{"X-Api-Key": "test-key"}
	_ = mock.GetJSON(context.Background(), "https://api.test.com/endpoint1", headers, 0, nil)
	_ = mock.GetJSON(context.Background(), "https://api.test.com/endpoint2", nil, 0, nil)

	callLog := mock.GetCallLog()
	if len(callLog) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(callLog))
	}

	if callLog[0].URL != "https://api.test.com/endpoint1" {
		t.Errorf("expected first URL to be endpoint1, got: %s", callLog[0].URL)
	}
	if callLog[0].Headers["X-Api-Key"] != "test-key" {
		t.Error("expected X-Api-Key header to be recorded")
	}
}

func TestMockHTTPClient_Stats(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.SetDefaultResponse(mocks.MockHTTPResponse{
		StatusCode: 200,
		Body:       `{}`,
	})

	_ = mock.GetJSON(context.Background(), "https://api.test.com/1", nil, 0, nil)
	_ = mock.GetJSON(context.Background(), "https://api.test.com/2", nil, 0, nil)

	stats := mock.GetStats()
	if stats.GetJSONCount != 2 {
		t.Errorf("expected 2 GetJSON calls, got %d", stats.GetJSONCount)
	}
	if stats.TotalCalls != 2 {
		t.Errorf("expected 2 total calls, got %d", stats.TotalCalls)
	}
}

func TestMockHTTPClient_TCGdexHelper(t *testing.T) {
	mock := mocks.NewMockHTTPClientWithTCGdexResponses()

	var result []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	err := mock.GetJSON(context.Background(), "https://api.tcgdex.net/v2/en/sets", nil, 0, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("expected 3 sets, got %d", len(result))
	}

	if result[0].ID != "base1" {
		t.Errorf("expected first set to be 'base1', got: %s", result[0].ID)
	}
}

func TestMockHTTPClient_PatternMatching(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.AddResponse("/v2/cards", mocks.MockHTTPResponse{
		StatusCode: 200,
		Body:       `{"data": [{"id": "card1"}]}`,
	})

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	// Should match any URL containing "/v2/cards"
	err := mock.GetJSON(context.Background(), "https://api.tcgdex.net/v2/cards?q=set.id:base", nil, 0, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Data) != 1 {
		t.Errorf("expected 1 card, got %d", len(result.Data))
	}

	if result.Data[0].ID != "card1" {
		t.Errorf("expected card ID 'card1', got: %s", result.Data[0].ID)
	}
}

func TestMockHTTPClient_Reset(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.SetDefaultResponse(mocks.MockHTTPResponse{
		StatusCode: 200,
		Body:       `{}`,
	})

	_ = mock.GetJSON(context.Background(), "https://api.test.com/1", nil, 0, nil)
	_ = mock.GetJSON(context.Background(), "https://api.test.com/2", nil, 0, nil)

	stats := mock.GetStats()
	if stats.TotalCalls != 2 {
		t.Errorf("expected 2 total calls before reset, got %d", stats.TotalCalls)
	}

	mock.Reset()

	stats = mock.GetStats()
	if stats.TotalCalls != 0 {
		t.Errorf("expected 0 total calls after reset, got %d", stats.TotalCalls)
	}

	callLog := mock.GetCallLog()
	if len(callLog) != 0 {
		t.Errorf("expected empty call log after reset, got %d calls", len(callLog))
	}
}

func TestMockHTTPClient_Get(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.AddResponse("https://api.test.com/resource", mocks.MockHTTPResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       `{"result": "ok"}`,
	})

	resp, err := mock.Get(context.Background(), "https://api.test.com/resource", nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Headers.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got: %s", contentType)
	}

	if string(resp.Body) != `{"result": "ok"}` {
		t.Errorf("unexpected body: %s", string(resp.Body))
	}
}

func TestMockHTTPClient_Post(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.AddResponse("https://api.test.com/create", mocks.MockHTTPResponse{
		StatusCode: 201,
		Body:       `{"id": "123"}`,
	})

	body := []byte(`{"name": "test"}`)
	resp, err := mock.Post(context.Background(), "https://api.test.com/create", nil, body, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 201 {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}

	// Verify request was recorded with body
	callLog := mock.GetCallLog()
	if len(callLog) != 1 {
		t.Fatalf("expected 1 call, got %d", len(callLog))
	}

	if callLog[0].Method != "POST" {
		t.Errorf("expected POST method, got: %s", callLog[0].Method)
	}

	if string(callLog[0].Body) != `{"name": "test"}` {
		t.Errorf("unexpected body in call log: %s", string(callLog[0].Body))
	}
}

func TestMockHTTPClient_PostJSON(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.AddResponse("https://api.test.com/create", mocks.MockHTTPResponse{
		StatusCode: 200,
		Body:       `{"id": "456", "status": "created"}`,
	})

	requestBody := map[string]string{"name": "test"}
	var responseBody struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}

	err := mock.PostJSON(context.Background(), "https://api.test.com/create", nil, requestBody, 0, &responseBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if responseBody.ID != "456" {
		t.Errorf("expected ID '456', got: %s", responseBody.ID)
	}

	if responseBody.Status != "created" {
		t.Errorf("expected status 'created', got: %s", responseBody.Status)
	}

	// Verify stats
	stats := mock.GetStats()
	if stats.PostJSONCount != 1 {
		t.Errorf("expected 1 PostJSON call, got %d", stats.PostJSONCount)
	}
}

func TestMockHTTPClient_FailAfterN(t *testing.T) {
	// FailAfterN should succeed for first N calls, then fail
	mock := mocks.NewMockHTTPClient(
		mocks.WithFailAfterN(2),
	)
	mock.SetDefaultResponse(mocks.MockHTTPResponse{
		StatusCode: 200,
		Body:       `{"ok": true}`,
	})

	// First 2 calls should succeed
	for i := 0; i < 2; i++ {
		err := mock.GetJSON(context.Background(), "https://api.test.com/data", nil, 0, nil)
		if err != nil {
			t.Errorf("call %d should succeed, got error: %v", i+1, err)
		}
	}

	// Third call should fail
	err := mock.GetJSON(context.Background(), "https://api.test.com/data", nil, 0, nil)
	if err == nil {
		t.Error("call 3 should fail, got nil error")
	}

	// Should be the default error from checkBehavior
	if !strings.Contains(err.Error(), "failed after") {
		t.Errorf("expected 'failed after' error, got: %v", err)
	}
}

func TestMockHTTPClient_DefaultResponse(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	// Don't add any specific responses, use default

	var result struct {
		Data []interface{} `json:"data"`
	}

	err := mock.GetJSON(context.Background(), "https://api.test.com/anything", nil, 0, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default response is {"data": []}
	if len(result.Data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(result.Data))
	}
}

func TestMockHTTPClient_ErrorResponse(t *testing.T) {
	mock := mocks.NewMockHTTPClient()
	mock.AddResponse("https://api.test.com/fail", mocks.MockHTTPResponse{
		Error: fmt.Errorf("connection refused"),
	})

	var result map[string]interface{}
	err := mock.GetJSON(context.Background(), "https://api.test.com/fail", nil, 0, &result)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "connection refused" {
		t.Errorf("expected 'connection refused', got: %v", err)
	}

	// Verify error was counted
	stats := mock.GetStats()
	if stats.TotalErrors != 1 {
		t.Errorf("expected 1 error, got %d", stats.TotalErrors)
	}
}

func TestMockHTTPClientWithStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{"404 Not Found", 404, "not found"},
		{"429 Rate Limited", 429, "rate limited"},
		{"503 Service Unavailable", 503, "unavailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := mocks.NewMockHTTPClientWithStatusCode(tt.statusCode, tt.body)

			var result map[string]interface{}
			err := mock.GetJSON(context.Background(), "https://api.test.com/resource", nil, 0, &result)

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			expectedErr := fmt.Sprintf("HTTP %d: %s", tt.statusCode, tt.body)
			if err.Error() != expectedErr {
				t.Errorf("expected error '%s', got: %v", expectedErr, err)
			}
		})
	}
}

func TestMockHTTPClientWithError(t *testing.T) {
	expectedErr := fmt.Errorf("network timeout")
	mock := mocks.NewMockHTTPClientWithError(expectedErr)

	var result map[string]interface{}
	err := mock.GetJSON(context.Background(), "https://api.test.com/resource", nil, 0, &result)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != expectedErr.Error() {
		t.Errorf("expected error '%v', got: %v", expectedErr, err)
	}
}

// Performance test to ensure mock is fast
func TestMockHTTPClient_Performance(t *testing.T) {
	mock := mocks.NewMockHTTPClientWithTCGdexResponses()

	start := time.Now()
	iterations := 1000

	for i := 0; i < iterations; i++ {
		var result []interface{}
		_ = mock.GetJSON(context.Background(), "https://api.tcgdex.net/v2/en/sets", nil, 0, &result)
	}

	duration := time.Since(start)
	avgPerCall := duration / time.Duration(iterations)

	// Each call should complete in < 1ms on average
	if avgPerCall > time.Millisecond {
		t.Errorf("mock too slow: average %v per call (expected < 1ms)", avgPerCall)
	}

	t.Logf("Performance: %d calls in %v (avg %v per call)", iterations, duration, avgPerCall)
}
