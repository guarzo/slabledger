package azureai

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponsesRoutingMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{
			name:     "rewrites /openai/responses",
			path:     "/openai/responses",
			wantPath: "/openai/deployments/gpt-4o/responses",
		},
		{
			name:     "rewrites /openai/responses with ID suffix",
			path:     "/openai/responses/resp_abc123",
			wantPath: "/openai/deployments/gpt-4o/responses/resp_abc123",
		},
		{
			name:     "does not rewrite non-responses path",
			path:     "/openai/chat/completions",
			wantPath: "/openai/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := responsesRoutingMiddleware("gpt-4o")

			req := httptest.NewRequest(http.MethodPost, "https://example.com"+tt.path, nil)

			var capturedPath string
			next := func(r *http.Request) (*http.Response, error) {
				capturedPath = r.URL.Path
				return &http.Response{StatusCode: 200}, nil
			}

			_, err := middleware(req, next)
			if err != nil {
				t.Fatalf("middleware returned error: %v", err)
			}
			if capturedPath != tt.wantPath {
				t.Errorf("path = %q, want %q", capturedPath, tt.wantPath)
			}
		})
	}
}
