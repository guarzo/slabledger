// Command psa-harvest logs into the PSA Buyer Campaign Manager portal with a
// headless browser and writes a fresh, AES-encrypted access token plus the
// current portal rows snapshot to Postgres for the main app to consume. Run it
// hourly in an image that has a browser (see Dockerfile.harvest); the lean
// alpine app image cannot run one and only reads the rows snapshot back out of
// the database.
//
// Required env: PSA_PORTAL_EMAIL, PSA_PORTAL_PASSWORD, ENCRYPTION_KEY, DATABASE_URL.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/psaportal"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
)

// Compile-time guards: the Postgres stores must satisfy the client's
// TokenRepository (read+write) and SnapshotWriter contracts.
var _ psaportal.TokenRepository = (*postgres.PSAPortalTokenStore)(nil)
var _ psaportal.SnapshotWriter = (*postgres.PSAPortalSnapshotStore)(nil)

func main() {
	if err := run(); err != nil {
		log.Fatalf("psa-harvest: %v", err)
	}
}

func run() error {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "json")
	// Bound the whole harvest: the Playwright browser run and the DB writes
	// inherit this deadline, so a hung login or navigation kills the process
	// instead of leaving the scheduled machine blocked (and auto-restarting)
	// forever. The in-script Playwright steps time out well inside this.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	switch {
	case cfg.PSAPortal.Email == "" || cfg.PSAPortal.Password == "":
		return errors.New("PSA_PORTAL_EMAIL and PSA_PORTAL_PASSWORD are required")
	case cfg.Auth.EncryptionKey == "":
		return errors.New("ENCRYPTION_KEY is required (token is encrypted at rest)")
	case cfg.Database.URL == "":
		return errors.New("DATABASE_URL is required")
	}

	dbCtx, dbCancel := context.WithTimeout(ctx, 90*time.Second)
	db, err := postgres.Open(dbCtx, cfg.Database.URL, logger)
	dbCancel()
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
	defer func() { _ = db.Close() }()

	enc, err := crypto.NewAESEncryptor(cfg.Auth.EncryptionKey)
	if err != nil {
		return fmt.Errorf("encryptor: %w", err)
	}
	store := postgres.NewPSAPortalTokenStore(db.DB, enc)
	snapshots := postgres.NewPSAPortalSnapshotStore(db.DB)

	// workDir "." — the image's WORKDIR is where web/scripts/ lives.
	h := psaportal.NewHarvester(store, snapshots, ".", cfg.PSAPortal.Email, cfg.PSAPortal.Password, logger)
	// Every run does the full cycle: the embed JWT captured by the browser lives
	// ~1h, so it must be exchanged for rows immediately, every time. The script
	// itself skips the SSO login while the stored token is still valid.
	// Harvest is best-effort: a browser/Lightdash failure (e.g. a transient
	// Cloudflare 403) must not block the drain, which only needs the stored
	// token. Log and continue; the token, if any, was already saved before the
	// Lightdash exchange inside Run.
	if err := h.Run(ctx); err != nil {
		logger.Warn(ctx, "psa-harvest: token/snapshot harvest failed, continuing to drain",
			observability.Err(err))
	} else {
		logger.Info(ctx, "psa-harvest: token and rows snapshot refreshed")
	}

	if cfg.PSASync.CampaignSyncEnabled {
		tp := psaportal.NewStoredTokenProvider(store)
		portal := psaportal.New(tp, psaportal.Config{}, psaportal.WithLogger(logger))
		snap := postgres.NewPSACampaignSnapshotStore(db.DB)
		queue := postgres.NewPSACampaignPushQueueStore(db.DB)
		linker := postgres.NewPSACampaignLinker(db.DB)

		campaigns, err := portal.FetchCampaigns(ctx)
		switch {
		case err != nil:
			logger.Error(ctx, "psa-harvest: fetch campaigns failed", observability.Err(err))
		case len(campaigns) == 0:
			logger.Warn(ctx, "psa-harvest: fetch campaigns returned no rows, skipping snapshot save")
		default:
			if err := snap.SaveSnapshot(ctx, campaigns); err != nil {
				logger.Error(ctx, "psa-harvest: save snapshot failed", observability.Err(err))
			}
		}

		pushed, failed, skipped := psaportal.DrainApprovedPushes(ctx, tp, portal, queue, linker, logger)
		if !skipped {
			logger.Info(ctx, "psa-harvest: push queue drained",
				observability.Int("pushed", pushed), observability.Int("failed", failed))
		}
	}

	return nil
}
