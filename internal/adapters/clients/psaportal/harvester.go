package psaportal

import (
	"context"
	"errors"
	"fmt"
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

// ErrPersistence marks a Run failure that came from persisting the token or
// snapshot to the store, as opposed to a browser/Lightdash/network failure.
// The caller propagates these (non-zero exit) because a DB write fault is
// retryable, unlike a Cloudflare block where a retry cannot help.
var ErrPersistence = errors.New("psaportal: persistence failure")

// Harvester persists the handshake token, then fetches the analytics
// __data.json via the browser session and immediately exchanges the freshly
// minted embed JWT for the Lightdash rows, persisting them as the snapshot
// the main app's sync reads.
type Harvester struct {
	repo      TokenRepository
	snapshots SnapshotWriter
	ld        *lightdashClient
	logger    observability.Logger
}

// HarvesterOption configures optional Harvester dependencies.
type HarvesterOption func(*Harvester)

// WithLightdashBaseURL overrides the Lightdash endpoint (tests).
func WithLightdashBaseURL(url string) HarvesterOption {
	return func(h *Harvester) { h.ld = newLightdashClient(url) }
}

// NewHarvester builds a Harvester.
func NewHarvester(repo TokenRepository, snapshots SnapshotWriter, logger observability.Logger, opts ...HarvesterOption) *Harvester {
	h := &Harvester{
		repo:      repo,
		snapshots: snapshots,
		ld:        newLightdashClient(defaultLightdashBaseURL),
		logger:    logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Run persists the handshake token, then fetches the analytics __data.json via
// the browser session and exchanges its embed JWT for the Lightdash rows. The
// token is saved before the Lightdash exchange so a Lightdash failure still
// leaves a fresh token behind. Failures persisting the token or snapshot are
// wrapped in ErrPersistence so the caller can propagate them (retryable) while
// treating browser/Lightdash failures as best-effort.
func (h *Harvester) Run(ctx context.Context, session Fetcher, token string, expiresAt time.Time) error {
	if token == "" {
		return fmt.Errorf("psaportal: harvester received empty token")
	}
	if err := h.repo.SaveToken(ctx, token, expiresAt); err != nil {
		return fmt.Errorf("%w: save token: %w", ErrPersistence, err)
	}
	h.logger.Info(ctx, "harvested PSA portal access token",
		observability.String("expires_at", expiresAt.Format(time.RFC3339)))

	resp, err := session.Do(ctx, FetchRequest{
		URL:    "/buyercampaignmanager/analytics/__data.json?x-sveltekit-invalidated=001",
		Method: "GET",
	})
	if err != nil {
		return fmt.Errorf("psaportal: analytics fetch: %w", err)
	}
	if resp.Status != 200 || !strings.Contains(resp.Body, "embedUrl") {
		return fmt.Errorf("psaportal: analytics __data.json fetch failed (status %d)", resp.Status)
	}

	rows, err := h.fetchRowsFromAnalytics(ctx, []byte(resp.Body))
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
		return fmt.Errorf("%w: save snapshot: %w", ErrPersistence, err)
	}
	h.logger.Info(ctx, "saved PSA portal rows snapshot", observability.Int("rows", len(rows)))
	return nil
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
