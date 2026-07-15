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

// TokenStore returns the most recently harvested portal access token.
// A "" token (no row yet) is not an error — the caller treats it as "needs harvest".
type TokenStore interface {
	CurrentToken(ctx context.Context) (token string, expiresAt time.Time, err error)
}

// TokenRepository reads and writes the harvested portal token. It extends the
// read-only TokenStore with a write path (embedding keeps CurrentToken declared
// in exactly one place, so the two interfaces can't drift apart).
type TokenRepository interface {
	TokenStore
	SaveToken(ctx context.Context, token string, expiresAt time.Time) error
}

// Browser short-circuit windows. When the stored token still has at least
// tokenFreshWindow of life AND the rows snapshot is no older than
// snapshotFreshWindow, Run skips launching Chromium entirely — avoiding the
// back-to-back headless launches that trigger Cloudflare 403s on rapid re-runs.
const (
	tokenFreshWindow    = 30 * time.Minute
	snapshotFreshWindow = 55 * time.Minute
)

// Harvester runs the Playwright login script, persists the access token, and
// immediately exchanges the freshly minted embed JWT for the Lightdash rows,
// persisting them as the snapshot the main app's sync reads.
type Harvester struct {
	repo      TokenRepository
	snapshots SnapshotWriter
	ld        *lightdashClient
	name      string   // executable, e.g. "node"
	args      []string // e.g. ["web/scripts/harvest-psa-token.mjs"]
	dir       string   // working dir (repo root)
	env       []string // extra env (PSA_PORTAL_EMAIL/PASSWORD=...)
	logger    observability.Logger
}

// HarvesterOption configures optional Harvester dependencies.
type HarvesterOption func(*Harvester)

// WithLightdashBaseURL overrides the Lightdash endpoint (tests).
func WithLightdashBaseURL(url string) HarvesterOption {
	return func(h *Harvester) { h.ld = newLightdashClient(url) }
}

// NewHarvester builds a Harvester that runs `node web/scripts/harvest-psa-token.mjs`.
func NewHarvester(repo TokenRepository, snapshots SnapshotWriter, workDir, email, password string, logger observability.Logger, opts ...HarvesterOption) *Harvester {
	h := &Harvester{
		repo:      repo,
		snapshots: snapshots,
		ld:        newLightdashClient(defaultLightdashBaseURL),
		name:      "node",
		args:      []string{"web/scripts/harvest-psa-token.mjs"},
		dir:       workDir,
		env:       []string{"PSA_PORTAL_EMAIL=" + email, "PSA_PORTAL_PASSWORD=" + password},
		logger:    logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Run performs one harvest cycle. It first short-circuits (see browserNeeded):
// when the stored token and rows snapshot are both fresh it skips the browser
// entirely and returns nil. Otherwise it runs the browser script (passing the
// stored token so a still-valid session skips the SSO login), persists the
// fresh token, then exchanges the just-minted embed JWT (~1h TTL, so it must be
// used immediately) for the Lightdash rows and persists the snapshot. The token
// is saved before the Lightdash exchange so a Lightdash failure still leaves a
// fresh token behind.
func (h *Harvester) Run(ctx context.Context) error {
	if !h.browserNeeded(ctx) {
		h.logger.Info(ctx, "harvest skipped: token fresh + snapshot recent")
		return nil
	}
	res, err := h.execScript(ctx)
	if err != nil {
		return err
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

	rows, err := h.fetchRowsFromAnalytics(ctx, []byte(res.AnalyticsData))
	if err != nil {
		return err
	}
	// Refuse to overwrite a good snapshot with an empty one: a transient 0-row
	// Lightdash result (error envelope, filter glitch) would otherwise stamp a
	// fresh fetched_at and defeat the reader's staleness guard, turning a loud
	// stale-failure into a silent "0 rows imported". Leave the previous snapshot
	// in place so it ages out loudly instead.
	if len(rows) == 0 {
		return fmt.Errorf("psaportal: harvest returned 0 rows; keeping previous snapshot")
	}
	if err := h.snapshots.SaveSnapshot(ctx, rows, time.Now()); err != nil {
		return err
	}
	h.logger.Info(ctx, "saved PSA portal rows snapshot", observability.Int("rows", len(rows)))
	return nil
}

// browserNeeded reports whether this run must launch the browser. It returns
// true (browser required) when the stored token is missing/stale or the rows
// snapshot is due; false only when both are fresh. Any read error is treated as
// "needed" so a lookup failure never suppresses a harvest.
func (h *Harvester) browserNeeded(ctx context.Context) bool {
	tok, exp, err := h.repo.CurrentToken(ctx)
	if err != nil || tok == "" || time.Until(exp) < tokenFreshWindow {
		return true
	}
	fetchedAt, err := h.snapshots.SnapshotFetchedAt(ctx)
	if err != nil || time.Since(fetchedAt) > snapshotFreshWindow {
		return true
	}
	return false
}

type scriptResult struct {
	AccessToken   string `json:"accessToken"`
	ExpiresAt     string `json:"expiresAt"`
	AnalyticsData string `json:"analyticsData"`
}

// execScript runs the browser script. A stored, still-valid token is passed via
// PSA_PORTAL_ACCESS_TOKEN so the script can skip the SSO login (cookie injection).
func (h *Harvester) execScript(ctx context.Context) (*scriptResult, error) {
	env := h.env
	if tok, exp, err := h.repo.CurrentToken(ctx); err == nil && tok != "" && time.Until(exp) > 5*time.Minute {
		env = append(append([]string{}, env...), "PSA_PORTAL_ACCESS_TOKEN="+tok)
	}
	cmd := exec.CommandContext(ctx, h.name, h.args...)
	cmd.Dir = h.dir
	cmd.Env = append(cmd.Environ(), env...)
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
		return nil, fmt.Errorf("psaportal: harvester exec: %w", err)
	}
	var res scriptResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &res); err != nil {
		return nil, fmt.Errorf("psaportal: harvester output: %w", err)
	}
	return &res, nil
}

// fetchRowsFromAnalytics extracts the embedUrl from the captured __data.json and
// exchanges its embed JWT for the itemized-purchases rows.
func (h *Harvester) fetchRowsFromAnalytics(ctx context.Context, analyticsData []byte) ([]map[string]string, error) {
	v, err := DecodeSvelteKitValue(analyticsData, "embedUrl")
	if err != nil {
		return nil, err
	}
	embedURL := strings.Trim(string(v), `"`)
	projectUUID, embedJWT, err := parseEmbedURL(embedURL)
	if err != nil {
		return nil, err
	}
	tileUUID, err := h.ld.findTileUUIDBySlug(ctx, projectUUID, embedJWT, itemizedPurchasesSlug)
	if err != nil {
		return nil, err
	}
	return h.ld.tileRows(ctx, projectUUID, embedJWT, tileUUID)
}
