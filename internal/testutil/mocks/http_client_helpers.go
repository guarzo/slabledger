package mocks

// NewMockHTTPClientWithError creates a mock that always returns an error.
// Useful for testing error handling.
//
// Example:
//
//	mock := NewMockHTTPClientWithError(fmt.Errorf("API unavailable"))
//
//	_, err := mock.GetJSON(ctx, "https://api.test.com/resource", nil, 0, nil)
//	// err will be "API unavailable"
func NewMockHTTPClientWithError(err error) *MockHTTPClient {
	return NewMockHTTPClient(WithError(err))
}

// NewMockHTTPClientWithStatusCode creates a mock that returns a specific HTTP status.
// Useful for testing HTTP error handling (404, 429, 503, etc.)
//
// Example:
//
//	// Test 404 handling
//	mock := NewMockHTTPClientWithStatusCode(404, "not found")
//
//	// Test rate limiting
//	mock := NewMockHTTPClientWithStatusCode(429, "rate limited")
//
//	// Test service unavailable
//	mock := NewMockHTTPClientWithStatusCode(503, "service unavailable")
func NewMockHTTPClientWithStatusCode(statusCode int, body string) *MockHTTPClient {
	mock := NewMockHTTPClient()
	mock.SetDefaultResponse(MockHTTPResponse{
		StatusCode: statusCode,
		Body:       body,
	})
	return mock
}
