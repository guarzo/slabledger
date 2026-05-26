package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestUpstreamError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  UpstreamError
		want string
	}{
		{
			name: "with message",
			err:  UpstreamError{Provider: "dh", Op: "POST /v1/foo", StatusCode: 422, Message: "No active channel"},
			want: `dh POST /v1/foo: status 422: No active channel`,
		},
		{
			name: "without message uses body",
			err:  UpstreamError{Provider: "dh", Op: "PATCH /v1/bar", StatusCode: 500, Body: "internal error"},
			want: `dh PATCH /v1/bar: status 500: internal error`,
		},
		{
			name: "no message or body",
			err:  UpstreamError{Provider: "dh", Op: "GET /v1/baz", StatusCode: 404},
			want: `dh GET /v1/baz: status 404`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpstreamError_IsClientError(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{200, false}, {399, false}, {400, true}, {422, true}, {499, true}, {500, false}, {503, false},
	}
	for _, tt := range tests {
		ue := UpstreamError{StatusCode: tt.status}
		if got := ue.IsClientError(); got != tt.want {
			t.Errorf("IsClientError(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestUpstreamError_ExtractMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		ct   string
		want string
	}{
		{
			name: "json error field",
			body: `{"error":"No active channel configured for: shopify"}`,
			ct:   "application/json",
			want: "No active channel configured for: shopify",
		},
		{
			name: "json message field",
			body: `{"message":"bad request"}`,
			ct:   "application/json",
			want: "bad request",
		},
		{
			name: "json without error or message",
			body: `{"foo":"bar"}`,
			ct:   "application/json",
			want: `{"foo":"bar"}`,
		},
		{
			name: "non-json plain",
			body: "internal error",
			ct:   "text/plain",
			want: "internal error",
		},
		{
			name: "empty body",
			body: "",
			ct:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractUpstreamMessage([]byte(tt.body), tt.ct)
			if got != tt.want {
				t.Errorf("extractUpstreamMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpstreamError_ErrorsAs(t *testing.T) {
	original := &UpstreamError{Provider: "dh", Op: "POST /v1/sync", StatusCode: 422, Message: "x"}
	wrapped := fmt.Errorf("operation failed: %w", original)
	var got *UpstreamError
	if !errors.As(wrapped, &got) {
		t.Fatal("errors.As did not find UpstreamError through wrapping")
	}
	if got.StatusCode != 422 {
		t.Errorf("StatusCode = %d, want 422", got.StatusCode)
	}
}

// Sanity check: extractUpstreamMessage trims and ignores nulls.
func TestUpstreamError_ExtractMessage_IgnoresEmptyJSONField(t *testing.T) {
	if got := extractUpstreamMessage([]byte(`{"error":""}`), "application/json"); got != `{"error":""}` {
		t.Errorf("empty error field should fall through to raw body, got %q", got)
	}
}

// Ensure JSON detection works even when content-type has charset suffix.
func TestUpstreamError_ExtractMessage_JSONWithCharset(t *testing.T) {
	body := `{"error":"foo"}`
	if got := extractUpstreamMessage([]byte(body), "application/json; charset=utf-8"); got != "foo" {
		t.Errorf("got %q, want %q", got, "foo")
	}
}

var _ = json.Marshal // keep json import if unused above
