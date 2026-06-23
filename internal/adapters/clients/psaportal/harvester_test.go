package psaportal

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// fakeTokenRepo implements TokenRepository for tests.
type fakeTokenRepo struct {
	currentToken     string
	currentExpiresAt time.Time
	currentErr       error
	savedToken       string
	savedExpiresAt   time.Time
	saveErr          error
}

func (r *fakeTokenRepo) CurrentToken(_ context.Context) (string, time.Time, error) {
	return r.currentToken, r.currentExpiresAt, r.currentErr
}

func (r *fakeTokenRepo) SaveToken(_ context.Context, token string, expiresAt time.Time) error {
	r.savedToken = token
	r.savedExpiresAt = expiresAt
	return r.saveErr
}

func TestHarvester_EnsureFreshToken_SkipsWhenFresh(t *testing.T) {
	repo := &fakeTokenRepo{
		currentToken:     "still-valid",
		currentExpiresAt: time.Now().Add(2 * time.Hour), // far in the future
	}
	h := NewHarvester(repo, ".", "email@test.com", "pw", observability.NewNoopLogger())
	// Override executable to one that would fail if called
	h.name = "false"
	h.args = nil

	if err := h.EnsureFreshToken(context.Background()); err != nil {
		t.Fatalf("expected nil (skipped), got %v", err)
	}
	// SaveToken should not have been called
	if repo.savedToken != "" {
		t.Errorf("expected SaveToken not called, got token %q", repo.savedToken)
	}
}

func TestHarvester_EnsureFreshToken_HarvestsWhenStale(t *testing.T) {
	repo := &fakeTokenRepo{
		// No token stored → will harvest
	}
	h := NewHarvester(repo, ".", "email@test.com", "pw", observability.NewNoopLogger())
	// Use a shell command that echoes the expected JSON
	h.name = "sh"
	h.args = []string{"-c", `printf '{"accessToken":"abc","expiresAt":"2099-01-01T00:00:00Z"}'`}

	if err := h.EnsureFreshToken(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.savedToken != "abc" {
		t.Errorf("expected savedToken=%q, got %q", "abc", repo.savedToken)
	}
	if repo.savedExpiresAt.IsZero() {
		t.Error("expected non-zero savedExpiresAt")
	}
}

func TestHarvester_EnsureFreshToken_HarvestsWhenExpired(t *testing.T) {
	repo := &fakeTokenRepo{
		currentToken:     "expired",
		currentExpiresAt: time.Now().Add(-1 * time.Hour), // expired
	}
	h := NewHarvester(repo, ".", "u", "p", observability.NewNoopLogger())
	h.name = "sh"
	h.args = []string{"-c", `printf '{"accessToken":"fresh","expiresAt":"2099-06-01T00:00:00Z"}'`}

	if err := h.EnsureFreshToken(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.savedToken != "fresh" {
		t.Errorf("expected savedToken=%q, got %q", "fresh", repo.savedToken)
	}
}

func TestHarvester_harvest_ExecError(t *testing.T) {
	repo := &fakeTokenRepo{}
	h := NewHarvester(repo, ".", "u", "p", observability.NewNoopLogger())
	h.name = "false" // exits with code 1
	h.args = nil

	if err := h.EnsureFreshToken(context.Background()); err == nil {
		t.Fatal("expected error from failing executable")
	}
}

func TestHarvester_harvest_EmptyToken(t *testing.T) {
	repo := &fakeTokenRepo{}
	h := NewHarvester(repo, ".", "u", "p", observability.NewNoopLogger())
	h.name = "sh"
	h.args = []string{"-c", `printf '{"accessToken":"","expiresAt":"2099-01-01T00:00:00Z"}'`}

	if err := h.EnsureFreshToken(context.Background()); err == nil {
		t.Fatal("expected error for empty token")
	}
}
