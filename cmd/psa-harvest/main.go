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

	"github.com/google/uuid"

	"github.com/guarzo/slabledger/internal/adapters/clients/psaportal"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/csvimport"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/platform/config"
	"github.com/guarzo/slabledger/internal/platform/crypto"
	"github.com/guarzo/slabledger/internal/platform/telemetry"
)

// Compile-time guards: the Postgres stores must satisfy the client's
// TokenRepository (read+write) and SnapshotWriter contracts.
var _ psaportal.TokenRepository = (*postgres.PSAPortalTokenStore)(nil)
var _ psaportal.SnapshotWriter = (*postgres.PSAPortalSnapshotStore)(nil)
var _ psacampaign.CatalogStore = (*postgres.PSAPortalCatalogStore)(nil)

func main() {
	baseline, rest, err := parseBaselineFlag(os.Args[1:])
	if err != nil {
		log.Fatalf("psa-harvest: %v", err)
	}
	if err := run(baseline, rest); err != nil {
		log.Fatalf("psa-harvest: %v", err)
	}
}

func run(baseline bool, args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	logger := telemetry.NewSlogLogger(slog.LevelInfo, "json")
	// Bound the whole harvest: the Playwright browser run and the DB writes
	// inherit this deadline, so a hung login or navigation kills the process
	// instead of leaving the scheduled machine blocked (and auto-restarting)
	// forever. The in-script Playwright steps time out well inside this.
	// Baseline mode fetches the edit form for every campaign in the fleet
	// rather than relying on the incremental snapshot, so it gets a longer
	// budget; it is still bounded, and it is a one-time, operator-invoked run
	// rather than the hourly schedule.
	timeout := 5 * time.Minute
	if baseline {
		timeout = 20 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch {
	case cfg.PSAPortal.Email == "" || cfg.PSAPortal.Password == "":
		return errors.New("PSA_PORTAL_EMAIL and PSA_PORTAL_PASSWORD are required")
	case cfg.Auth.EncryptionKey == "":
		return errors.New("ENCRYPTION_KEY is required (token is encrypted at rest)")
	case cfg.Database.URL == "":
		return errors.New("DATABASE_URL is required")
	case cfg.PSAPortal.PushSigningKey == "":
		// Fail closed. Without the key the drain cannot authenticate any
		// approval, so every queued row would be marked failed — better to
		// refuse the run than to burn the queue.
		return errors.New("PSA_PUSH_SIGNING_KEY is required (approvals are signed; it must match the web app's key)")
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
	pushSigner, err := crypto.NewHMACSigner(cfg.PSAPortal.PushSigningKey, cfg.PSAPortal.PushSigningKeyID)
	if err != nil {
		return fmt.Errorf("push signer: %w", err)
	}
	store := postgres.NewPSAPortalTokenStore(db.DB, enc)
	snapshots := postgres.NewPSAPortalSnapshotStore(db.DB)

	// One browser login per run, shared by the token/analytics harvest and the
	// campaign sync/drain, so every psacard.com call clears Cloudflare. The
	// writes cannot reach the portal any other way, so a failed session open is
	// fatal for the run.
	storedToken, _, _ := store.CurrentToken(ctx) // best-effort; "" just means full SSO
	session, token, expiresAt, err := psaportal.OpenBrowserSession(ctx, ".", cfg.PSAPortal.Email, cfg.PSAPortal.Password, storedToken, cfg.PSAPortal.ProxyURL, logger)
	if err != nil {
		return fmt.Errorf("open portal session: %w", err)
	}
	defer func() { _ = session.Close() }()

	h := psaportal.NewHarvester(store, snapshots, logger)
	// Best-effort: a failed analytics read must not skip queued writes (the
	// session is already authenticated). A persistence failure (token/snapshot
	// DB write) is retryable, so propagate it for a non-zero exit; a
	// browser/Lightdash failure is not helped by a retry — log and continue to
	// the drain, which rides the same authenticated session.
	if err := h.Run(ctx, session, token, expiresAt); err != nil {
		if errors.Is(err, psaportal.ErrPersistence) {
			return err
		}
		logger.Warn(ctx, "psa-harvest: token/analytics harvest failed, continuing to drain",
			observability.Err(err))
	} else {
		logger.Info(ctx, "psa-harvest: token and rows snapshot refreshed")
	}

	// Baseline mode does all its work inside the campaign-sync block below. If
	// sync is disabled, that block never runs and `run` would return nil — an
	// exit 0 the operator reads as "baseline complete" when nothing was pulled.
	// Refuse instead.
	if baseline && !cfg.PSASync.CampaignSyncEnabled {
		return fmt.Errorf("baseline: PSA campaign sync is disabled (PSA_CAMPAIGN_SYNC_ENABLED); " +
			"the baseline pull has nothing to read from — enable it and re-run")
	}

	if cfg.PSASync.CampaignSyncEnabled {
		portal := psaportal.New(session, psaportal.Config{}, psaportal.WithLogger(logger))
		snap := postgres.NewPSACampaignSnapshotStore(db.DB)
		queue := postgres.NewPSACampaignPushQueueStore(db.DB)
		linker := postgres.NewPSACampaignLinker(db.DB)
		catalog := postgres.NewPSAPortalCatalogStore(db.DB)
		campaignStore := postgres.NewCampaignStore(db.DB, logger)

		campaignsFresh := false
		campaigns, specLists, err := portal.FetchCampaigns(ctx)
		switch {
		case err != nil:
			// Normal runs tolerate this: the drain below still has value, and
			// the next hourly run retries. Baseline runs must not — a failed
			// fetch means `campaigns` is nil, and continuing would hand
			// runBaselinePull an empty fleet it would report as a clean sweep.
			if baseline {
				return fmt.Errorf("baseline: fetch campaigns: %w", err)
			}
			logger.Error(ctx, "psa-harvest: fetch campaigns failed", observability.Err(err))
		case len(campaigns) == 0:
			// Same reasoning. Zero portal campaigns is a plausible transient
			// (an auth redirect that still returns 200), and it is
			// indistinguishable from "the baseline had nothing to do" unless
			// this path is fatal.
			if baseline {
				return fmt.Errorf("baseline: portal returned zero campaigns; refusing to record an empty baseline")
			}
			logger.Warn(ctx, "psa-harvest: fetch campaigns returned no rows, skipping snapshot save")
		default:
			if err := snap.SaveSnapshot(ctx, campaigns); err != nil {
				logger.Error(ctx, "psa-harvest: save snapshot failed", observability.Err(err))
			} else {
				campaignsFresh = true
			}
		}

		// Only fetch the itemized rows snapshot when the campaign snapshot is
		// actually fresh — decideReconcileGate would skip on campaignsFresh
		// alone anyway, so there is nothing to diagnose from rowsErr in that case.
		var rows []csvimport.PSAExportRow
		var rowsErr error
		if campaignsFresh {
			rowProvider := psaportal.NewSnapshotRowProvider(snapshots, logger)
			rows, rowsErr = rowProvider.FetchRows(ctx)
		}

		gate := decideReconcileGate(campaignsFresh, rowsErr)
		if !gate.proceed {
			if gate.skipErr != nil {
				logger.Warn(ctx, gate.skipMsg, observability.Err(gate.skipErr))
			} else {
				logger.Warn(ctx, gate.skipMsg)
			}
		} else {
			resolver := psaportal.NewCampaignResolver(snap, campaignStore, nil)
			imp := buildImportService(db, logger, campaignStore, resolver)
			res, recErr := imp.ReconcilePSAAttribution(ctx, rows)
			if recErr != nil {
				logger.Error(ctx, "psa-harvest: attribution reconcile failed", observability.Err(recErr))
			} else {
				logger.Info(ctx, "psa-harvest: attribution reconciled",
					observability.Int("moved", res.Moved),
					observability.Int("unresolved", res.Unresolved),
					observability.Int("soldSkipped", res.SoldSkipped))
			}
		}

		// Persist the portal reference catalog on every run, baseline or not:
		// this is what keeps the main server's translation Resolver inside
		// psacampaign.CatalogMaxAge without needing its own portal session.
		if len(specLists) > 0 {
			if err := catalog.SaveSpecLists(ctx, specLists); err != nil {
				logger.Error(ctx, "psa-harvest: save spec-list catalog failed", observability.Err(err))
			}
		}
		if subjects, err := portal.FetchSubjects(ctx, psacampaign.PokemonCategoryID); err != nil {
			logger.Error(ctx, "psa-harvest: fetch subjects failed", observability.Err(err))
		} else if err := catalog.SaveSubjects(ctx, psacampaign.PokemonCategoryID, subjects); err != nil {
			logger.Error(ctx, "psa-harvest: save subject catalog failed", observability.Err(err))
		}

		if baseline {
			// Zero portal writes: return here, before DrainPushQueue is ever
			// reached. This is the whole safety property of -baseline-pull,
			// enforced structurally rather than by hoping the queue is empty.
			if err := runBaselinePull(ctx, campaigns, campaignStore, logger); err != nil {
				return fmt.Errorf("baseline: %w", err)
			}
			logger.Info(ctx, "psa-harvest: baseline pull complete, all linked campaigns had complete targeting")
			return nil
		}

		pushed, failed := psaportal.DrainPushQueue(ctx, portal, queue, linker, pushSigner, logger)
		logger.Info(ctx, "psa-harvest: push queue drained",
			observability.Int("pushed", pushed), observability.Int("failed", failed))
	}

	return nil
}

// reconcileGate is the pure freshness decision for whether PSA attribution
// reconciliation should run this harvest. Separated from decideReconcileGate's
// callers' I/O (SaveSnapshot, FetchRows) so the gate itself is unit-testable
// without a live DB/browser session.
type reconcileGate struct {
	proceed bool
	// skipMsg and skipErr are logged at Warn when proceed is false. skipErr is
	// nil for the "campaign snapshot not refreshed" case, since there is no
	// error value for that — the campaign fetch/save outcome is a bool by the
	// time it reaches this gate.
	skipMsg string
	skipErr error
}

// decideReconcileGate decides whether reconciliation may run, given whether
// the campaign snapshot was actually refreshed this run and the outcome of
// fetching the itemized rows snapshot (nil rowsErr, or unevaluated, when
// campaignsFresh is false — the campaign gate alone already skips).
//
// Resolving fresh purchases against either a stale campaign list or a stale
// itemized snapshot risks attributing to a renamed or superseded campaign;
// reconciliation is idempotent, so skipping only costs one cycle.
func decideReconcileGate(campaignsFresh bool, rowsErr error) reconcileGate {
	switch {
	case !campaignsFresh:
		return reconcileGate{skipMsg: "psa-harvest: skipping attribution reconcile, campaign snapshot not refreshed"}
	case rowsErr != nil:
		return reconcileGate{skipMsg: "psa-harvest: skipping attribution reconcile", skipErr: rowsErr}
	default:
		return reconcileGate{proceed: true}
	}
}

// buildImportService assembles the CSV-intake service for this binary's one use
// of it: PSA attribution reconciliation. It wires the repositories that path
// touches plus the pending-items repository, so an unresolved PSA campaign name
// still lands in the operator work queue.
func buildImportService(db *postgres.DB, logger observability.Logger, campaignStore *postgres.CampaignStore, resolver inventory.PSACampaignResolver) csvimport.Service {
	return csvimport.NewService(csvimport.Deps{
		Campaigns:    campaignStore,
		Purchases:    postgres.NewPurchaseStore(db.DB, logger),
		Sales:        postgres.NewSaleStore(db.DB, logger),
		Finance:      postgres.NewFinanceStore(db.DB, logger),
		PendingItems: postgres.NewPendingItemsRepository(db.DB),
		PSAResolver:  resolver,
		IDGen:        uuid.NewString,
		Logger:       logger,
	})
}
