package psaportal

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestHarvester_EnsureFreshToken(t *testing.T) {
	tests := []struct {
		name          string
		currentToken  string
		currentExpiry time.Time
		cmdName       string
		cmdArgs       []string
		wantSaved     string
		wantSavedTime bool // expect a non-zero savedExpiresAt
		wantErr       bool
	}{
		{
			name:          "skips when fresh",
			currentToken:  "still-valid",
			currentExpiry: time.Now().Add(2 * time.Hour),
			cmdName:       "false", // would fail if called
			wantSaved:     "",      // SaveToken must not be called
		},
		{
			name:          "harvests when stale",
			cmdName:       "sh",
			cmdArgs:       []string{"-c", `printf '{"accessToken":"abc","expiresAt":"2099-01-01T00:00:00Z"}'`},
			wantSaved:     "abc",
			wantSavedTime: true,
		},
		{
			name:          "harvests when expired",
			currentToken:  "expired",
			currentExpiry: time.Now().Add(-1 * time.Hour),
			cmdName:       "sh",
			cmdArgs:       []string{"-c", `printf '{"accessToken":"fresh","expiresAt":"2099-06-01T00:00:00Z"}'`},
			wantSaved:     "fresh",
			wantSavedTime: true,
		},
		{
			name:    "exec error",
			cmdName: "false", // exits with code 1
			wantErr: true,
		},
		{
			name:    "empty token",
			cmdName: "sh",
			cmdArgs: []string{"-c", `printf '{"accessToken":"","expiresAt":"2099-01-01T00:00:00Z"}'`},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mocks.PSATokenRepositoryMock{
				CurrentTokenFn: func(_ context.Context) (string, time.Time, error) {
					return tt.currentToken, tt.currentExpiry, nil
				},
			}
			h := NewHarvester(repo, ".", "email@test.com", "pw", observability.NewNoopLogger())
			h.name = tt.cmdName
			h.args = tt.cmdArgs

			err := h.EnsureFreshToken(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.SavedToken != tt.wantSaved {
				t.Errorf("savedToken: expected %q, got %q", tt.wantSaved, repo.SavedToken)
			}
			if tt.wantSavedTime && repo.SavedExpiresAt.IsZero() {
				t.Error("expected non-zero savedExpiresAt")
			}
		})
	}
}
