package pokemonprice

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		wantAvail bool
	}{
		{
			name:      "valid API key",
			apiKey:    "test_api_key",
			wantAvail: true,
		},
		{
			name:      "empty API key",
			apiKey:    "",
			wantAvail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.apiKey)
			if client == nil {
				t.Fatal("NewClient returned nil")
			}

			if got := client.Available(); got != tt.wantAvail {
				t.Errorf("Available() = %v, want %v", got, tt.wantAvail)
			}

			if client.Name() != "pokemonprice" {
				t.Errorf("Name() = %v, want pokemonprice", client.Name())
			}
		})
	}
}

func TestClient_Close(t *testing.T) {
	client := NewClient("test_key")
	if err := client.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestClient_GetPrice_NotAvailable(t *testing.T) {
	client := NewClient("")
	_, statusCode, headers, err := client.GetPrice(context.Background(), "test_set", "test_card", "001")
	if err == nil {
		t.Error("GetPrice() should return error when API key not configured")
	}

	// Use error type assertion for robust testing
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("GetPrice() error type = %T, want *apperrors.AppError", err)
	}

	// Verify error code indicates missing configuration (appErr is non-nil here)
	if appErr.Code != apperrors.ErrCodeConfigMissing {
		t.Errorf("GetPrice() error code = %v, want %v", appErr.Code, apperrors.ErrCodeConfigMissing)
	}

	// When not available, status code should be 0 and headers nil
	if statusCode != 0 {
		t.Errorf("GetPrice() statusCode = %d, want 0 when not available", statusCode)
	}
	if headers != nil {
		t.Errorf("GetPrice() headers = %v, want nil when not available", headers)
	}
}
