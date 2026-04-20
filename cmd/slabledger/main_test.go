package main

import (
	"context"
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

