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
	"log"
	"log/slog"
	"os"

	"github.com/guarzo/slabledger/internal/adapters/clients/psaportal"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
)

func main() {
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		log.Fatalf("psa-harvest: config: %v", err)
	}
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "json")
	ctx := context.Background()

	switch {
	case cfg.PSAPortal.Email == "" || cfg.PSAPortal.Password == "":
		log.Fatal("psa-harvest: PSA_PORTAL_EMAIL and PSA_PORTAL_PASSWORD are required")
	case cfg.Auth.EncryptionKey == "":
		log.Fatal("psa-harvest: ENCRYPTION_KEY is required (token is encrypted at rest)")
	case cfg.Database.URL == "":
		log.Fatal("psa-harvest: DATABASE_URL is required")
	}

	db, err := postgres.Open(cfg.Database.URL, logger)
	if err != nil {
		log.Fatalf("psa-harvest: db open: %v", err)
	}
	defer func() { _ = db.Close() }()

	enc, err := crypto.NewAESEncryptor(cfg.Auth.EncryptionKey)
	if err != nil {
		log.Fatalf("psa-harvest: encryptor: %v", err)
	}
	store := postgres.NewPSAPortalTokenStore(db.DB, enc)

	// workDir "." — the image's WORKDIR is where web/scripts/ lives.
	h := psaportal.NewHarvester(store, ".", cfg.PSAPortal.Email, cfg.PSAPortal.Password, logger)
	if err := h.Harvest(ctx); err != nil {
		log.Fatalf("psa-harvest: %v", err)
	}
	logger.Info(ctx, "psa-harvest: access token refreshed")
}
