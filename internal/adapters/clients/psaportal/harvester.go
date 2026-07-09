package psaportal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// TokenRepository reads and writes the harvested portal token.
type TokenRepository interface {
	CurrentToken(ctx context.Context) (token string, expiresAt time.Time, err error)
	SaveToken(ctx context.Context, token string, expiresAt time.Time) error
}

// Harvester runs the Playwright login script to refresh the stored access token.
type Harvester struct {
	repo     TokenRepository
	name     string   // executable, e.g. "node"
	args     []string // e.g. ["web/scripts/harvest-psa-token.mjs"]
	dir      string   // working dir (repo root)
	env      []string // extra env (PSA_PORTAL_EMAIL/PASSWORD=...)
	freshFor time.Duration
	logger   observability.Logger
}

// NewHarvester builds a Harvester that runs `node web/scripts/harvest-psa-token.mjs`.
func NewHarvester(repo TokenRepository, workDir, email, password string, logger observability.Logger) *Harvester {
	return &Harvester{
		repo:     repo,
		name:     "node",
		args:     []string{"web/scripts/harvest-psa-token.mjs"},
		dir:      workDir,
		env:      []string{"PSA_PORTAL_EMAIL=" + email, "PSA_PORTAL_PASSWORD=" + password},
		freshFor: time.Hour,
		logger:   logger,
	}
}

// EnsureFreshToken harvests a new token unless the stored one is still valid
// for at least freshFor.
func (h *Harvester) EnsureFreshToken(ctx context.Context) error {
	tok, exp, err := h.repo.CurrentToken(ctx)
	if err == nil && tok != "" && time.Until(exp) > h.freshFor {
		return nil // still fresh
	}
	h.logger.Info(ctx, "harvesting PSA portal access token")
	return h.harvest(ctx)
}

func (h *Harvester) harvest(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, h.name, h.args...)
	cmd.Dir = h.dir
	cmd.Env = append(cmd.Environ(), h.env...)
	out, err := cmd.Output()
	if err != nil {
		// exec.Output captures the child's stderr on ExitError — surface it so
		// harvester failures (login/selector errors) are diagnosable from logs.
		var stderr string
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		h.logger.Error(ctx, "PSA portal token harvest failed",
			observability.Err(err), observability.String("stderr", stderr))
		return fmt.Errorf("psaportal: harvester exec: %w", err)
	}
	var res struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   string `json:"expiresAt"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &res); err != nil {
		return fmt.Errorf("psaportal: harvester output: %w", err)
	}
	exp, err := time.Parse(time.RFC3339, res.ExpiresAt)
	if err != nil {
		return fmt.Errorf("psaportal: harvester expiresAt: %w", err)
	}
	if res.AccessToken == "" {
		return fmt.Errorf("psaportal: harvester returned empty token")
	}
	if err := h.repo.SaveToken(ctx, res.AccessToken, exp); err != nil {
		return err
	}
	h.logger.Info(ctx, "harvested PSA portal access token",
		observability.String("expires_at", exp.Format(time.RFC3339)))
	return nil
}
