package mocks

import (
	"fmt"
)

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

// NewMockHTTPClientWithTCGdexResponses creates a mock HTTP client
// pre-configured with realistic TCGdex API responses.
//
// Pre-configured endpoints:
//   - GET /v2/en/series (all series)
//   - GET /v2/en/sets (all sets)
//
// Example:
//
//	mock := NewMockHTTPClientWithTCGdexResponses()
//	mock.AddResponse("https://api.tcgdex.net/v2/en/sets", ...)
func NewMockHTTPClientWithTCGdexResponses() *MockHTTPClient {
	mock := NewMockHTTPClient()

	// Configure TCGdex sets endpoint
	setsResponse := MockHTTPResponse{
		StatusCode: 200,
		Body: `[
			{
				"id": "base1",
				"name": "Base Set",
				"logo": "https://assets.tcgdex.net/en/base/base1/logo",
				"cardCount": {"total": 102, "official": 102}
			},
			{
				"id": "base2",
				"name": "Jungle",
				"logo": "https://assets.tcgdex.net/en/base/base2/logo",
				"cardCount": {"total": 64, "official": 64}
			},
			{
				"id": "base3",
				"name": "Fossil",
				"logo": "https://assets.tcgdex.net/en/base/base3/logo",
				"cardCount": {"total": 62, "official": 62}
			}
		]`,
	}

	mock.AddResponse(fmt.Sprintf("%s/v2/en/sets", "https://api.tcgdex.net"), setsResponse)
	mock.AddResponse("/v2/en/sets", setsResponse)

	return mock
}
