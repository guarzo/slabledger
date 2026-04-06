package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestShowHelp(t *testing.T) {
	// showHelp prints to stdout; verify it does not panic.
	showHelp()
}

func TestValidateEnvironmentVariables(t *testing.T) {
	logger := mocks.NewMockLogger()
	ctx := context.Background()

	tests := []struct {
		name                 string
		cfg                  config.Config
		wantMissingRequired  bool
		wantWarningsContains string
	}{
		{
			name:                "empty config has no required failures",
			cfg:                 config.Config{},
			wantMissingRequired: false,
		},
		{
			name: "encryption key set but no Google OAuth triggers warning",
			cfg: config.Config{
				Auth: config.AuthConfig{
					EncryptionKey: "a]32-char-encryption-key-for-test",
				},
			},
			wantMissingRequired:  false,
			wantWarningsContains: "ENCRYPTION_KEY is set but GOOGLE_CLIENT_ID is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear Google OAuth env vars so LoadGoogleOAuthConfig returns empty values.
			t.Setenv("GOOGLE_CLIENT_ID", "")
			t.Setenv("GOOGLE_CLIENT_SECRET", "")

			result := validateEnvironmentVariables(ctx, logger, &tt.cfg)

			if tt.wantMissingRequired && len(result.MissingRequired) == 0 {
				t.Error("expected MissingRequired to have entries, got none")
			}
			if !tt.wantMissingRequired && len(result.MissingRequired) > 0 {
				t.Errorf("expected no MissingRequired, got %v", result.MissingRequired)
			}

			if tt.wantWarningsContains != "" {
				found := false
				for _, w := range result.Warnings {
					if strings.Contains(w, tt.wantWarningsContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected Warnings to contain %q, got %v", tt.wantWarningsContains, result.Warnings)
				}
			}
		})
	}
}

func TestResolveDatabasePath(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name:    "empty path returns error",
			input:   "",
			wantErr: true,
		},
		{
			name:    "absolute path returned cleaned",
			input:   "/tmp/data/../data/test.db",
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				expected := "/tmp/data/test.db"
				if result != expected {
					t.Errorf("expected %q, got %q", expected, result)
				}
			},
		},
		{
			name:    "relative path returns absolute path",
			input:   "data/test.db",
			wantErr: false,
			checkFunc: func(t *testing.T, result string) {
				if !filepath.IsAbs(result) {
					t.Errorf("expected absolute path, got %q", result)
				}
				// Should end with data/test.db
				if filepath.Base(result) != "test.db" {
					t.Errorf("expected path to end with test.db, got %q", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveDatabasePath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

func TestInitializeCache(t *testing.T) {
	t.Run("empty path returns nil", func(t *testing.T) {
		result := initializeCache("")
		if result != nil {
			t.Errorf("expected nil cache for empty path, got %v", result)
		}
	})
}
