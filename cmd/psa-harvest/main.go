// Command psa-harvest logs into the PSA Buyer Campaign Manager portal with a
// headless browser and writes a fresh, AES-encrypted access token to Postgres for
// the main app to consume. Run it on a schedule (~every 12h) in an image that has a
// browser (see Dockerfile.harvest); the lean alpine app image cannot run one and
// only reads the token back out of the database.
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

	"github.com/guarzo/slabledger/internal/adapters/clients/psaportal"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
)

// Compile-time guard: the Postgres token store must satisfy the client's
// TokenRepository (read+write) contract the harvester below depends on.
var _ psaportal.TokenRepository = (*postgres.PSAPortalTokenStore)(nil)

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
	ctx := context.Background()

	switch {
	case cfg.PSAPortal.Email == "" || cfg.PSAPortal.Password == "":
		return errors.New("PSA_PORTAL_EMAIL and PSA_PORTAL_PASSWORD are required")
	case cfg.Auth.EncryptionKey == "":
		return errors.New("ENCRYPTION_KEY is required (token is encrypted at rest)")
	case cfg.Database.URL == "":
		return errors.New("DATABASE_URL is required")
	}

	db, err := postgres.Open(cfg.Database.URL, logger)
	if err != nil {
		return fmt.Errorf("db open: %w", err)
	}
	defer func() { _ = db.Close() }()

	enc, err := crypto.NewAESEncryptor(cfg.Auth.EncryptionKey)
	if err != nil {
		return fmt.Errorf("encryptor: %w", err)
	}
	store := postgres.NewPSAPortalTokenStore(db.DB, enc)

	// workDir "." — the image's WORKDIR is where web/scripts/ lives.
	h := psaportal.NewHarvester(store, ".", cfg.PSAPortal.Email, cfg.PSAPortal.Password, logger)
	if err := h.Harvest(ctx); err != nil {
		return err
	}
	logger.Info(ctx, "psa-harvest: access token refreshed")
	return nil
}
