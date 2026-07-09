package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// CardLadderPurchaseLister lists unsold purchases with their image URLs.
type CardLadderPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error)
}

// CardLadderValueUpdater updates CL values on purchases.
type CardLadderValueUpdater interface {
	UpdatePurchaseCLValue(ctx context.Context, purchaseID string, clValueCents, population int) error
	// UpdatePurchaseCLError records or clears the last mapping/pricing failure reason.
	// Pass reason="" and reasonAt="" to clear on success.
	UpdatePurchaseCLError(ctx context.Context, purchaseID, reason, reasonAt string) error
}

// CardLadderGemRateUpdater persists gemRateID, psaSpecID, and CL card metadata on purchases.
type CardLadderGemRateUpdater interface {
	UpdatePurchaseGemRateID(ctx context.Context, purchaseID, gemRateID string) error
	UpdatePurchasePSASpecID(ctx context.Context, purchaseID string, psaSpecID int) error
	UpdatePurchaseCLCardMetadata(ctx context.Context, id, player, variation, category string) error
	UpdatePurchaseSetName(ctx context.Context, purchaseID, setName string) error
}

// CardLadderRefreshOption configures optional dependencies on a CardLadderRefreshScheduler.
type CardLadderRefreshOption func(*CardLadderRefreshScheduler)

// WithCLDHPushUpdater enables DH re-push when CL values change.
func WithCLDHPushUpdater(u DHPushStatusUpdater) CardLadderRefreshOption {
	return func(s *CardLadderRefreshScheduler) { s.dhPushUpdater = u }
}

// CardLadderSyncUpdater sets cl_synced_at on purchases after CL push/sync.
type CardLadderSyncUpdater interface {
	UpdatePurchaseCLSyncedAt(ctx context.Context, purchaseID string, syncedAt string) error
}

// WithCLSyncUpdater enables cl_synced_at tracking on push.
func WithCLSyncUpdater(u CardLadderSyncUpdater) CardLadderRefreshOption {
	return func(s *CardLadderRefreshScheduler) { s.syncUpdater = u }
}

// WithCLEventRecorder enables DH state-event recording for CL-refresh-driven
// re-enrollment transitions. Optional — nil means no events are written.
func WithCLEventRecorder(r dhevents.Recorder) CardLadderRefreshOption {
	return func(s *CardLadderRefreshScheduler) { s.eventRec = r }
}

// WithCLCompRefreshStore enables decoupled comp refresh (queries campaign_purchases.gem_rate_id directly).
func WithCLCompRefreshStore(s *postgres.CompRefreshStore) CardLadderRefreshOption {
	return func(sch *CardLadderRefreshScheduler) { sch.compRefreshStore = s }
}

// CLRunStats holds the counters from the most recent Card Ladder refresh run.
type CLRunStats struct {
	LastRunAt         time.Time `json:"lastRunAt"`
	DurationMs        int64     `json:"durationMs"`
	TotalPurchases    int       `json:"totalPurchases"`    // Unsold purchases considered this run.
	Updated           int       `json:"updated"`           // Purchases whose CL value was refreshed.
	Resolved          int       `json:"resolved"`          // Certs newly resolved to gemRateID via BuildCollectionCard this run.
	NoCert            int       `json:"noCert"`            // Purchases with no cert number (can't look up).
	CertResolveFailed int       `json:"certResolveFailed"` // BuildCollectionCard errored or returned no gemRateID.
	EstimateFailed    int       `json:"estimateFailed"`    // CardEstimate callable errored after cert resolved.
	NoValue           int       `json:"noValue"`           // Resolved to gemRateID but CardEstimate returned no value.
	QuotaExhausted    bool      `json:"quotaExhausted"`    // CL daily request quota was hit; remaining estimates were skipped this cycle.
	SkippedQuota      int       `json:"skippedQuota"`      // Cards skipped (not attempted) after the quota wall was hit this cycle.
	CardsPushed       int       `json:"cardsPushed"`       // Cards pushed to CL remote collection (UI hygiene).
	CardsRemoved      int       `json:"cardsRemoved"`      // Sold cards removed from CL remote collection (UI hygiene).
}

// CardLadderRefreshScheduler refreshes CL values from the Card Ladder API daily.
type CardLadderRefreshScheduler struct {
	StopHandle
	statsMu          sync.RWMutex
	clientMu         sync.RWMutex
	client           *cardladder.Client
	store            *postgres.CardLadderStore
	purchaseLister   CardLadderPurchaseLister
	valueUpdater     CardLadderValueUpdater
	gemRateUpdater   CardLadderGemRateUpdater
	syncUpdater      CardLadderSyncUpdater // optional: sets cl_synced_at on push
	salesStore       *postgres.CLSalesStore
	compRefreshStore *postgres.CompRefreshStore
	dhPushUpdater    DHPushStatusUpdater           // optional: re-enrolls changed items for DH push
	eventRec         dhevents.Recorder             // optional: records DH state-transition events
	statsStore       *postgres.SchedulerStatsStore // optional: persists lastRunStats across restarts
	logger           observability.Logger
	config           config.CardLadderConfig
	lastRunStats     *CLRunStats
}

// SetClient replaces the API client used by the scheduler. This is called when
// credentials are saved for the first time after startup (no client at boot).
func (s *CardLadderRefreshScheduler) SetClient(client *cardladder.Client) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.client = client
}

// getClient returns the current API client under a read lock.
func (s *CardLadderRefreshScheduler) getClient() *cardladder.Client {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	return s.client
}

// recordEvent is a nil-safe helper that writes a DH state event. Failures are
// logged at Warn and never propagated to the caller.
func (s *CardLadderRefreshScheduler) recordEvent(ctx context.Context, e dhevents.Event) {
	if s.eventRec == nil {
		return
	}
	if err := s.eventRec.Record(ctx, e); err != nil {
		s.logger.Warn(ctx, "CL refresh: record dh event failed",
			observability.String("type", string(e.Type)),
			observability.Err(err))
	}
}

// NewCardLadderRefreshScheduler creates a new CL refresh scheduler.
func NewCardLadderRefreshScheduler(
	client *cardladder.Client,
	store *postgres.CardLadderStore,
	purchaseLister CardLadderPurchaseLister,
	valueUpdater CardLadderValueUpdater,
	gemRateUpdater CardLadderGemRateUpdater,
	salesStore *postgres.CLSalesStore,
	logger observability.Logger,
	cfg config.CardLadderConfig,
	opts ...CardLadderRefreshOption,
) *CardLadderRefreshScheduler {
	cfg.ApplyDefaults()
	s := &CardLadderRefreshScheduler{
		StopHandle:     NewStopHandle(),
		client:         client,
		store:          store,
		purchaseLister: purchaseLister,
		valueUpdater:   valueUpdater,
		gemRateUpdater: gemRateUpdater,
		salesStore:     salesStore,
		logger:         logger,
		config:         cfg,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Start begins the scheduler loop.
func (s *CardLadderRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "Card Ladder refresh scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "card-ladder-refresh",
		Interval:     s.config.Interval,
		InitialDelay: timeUntilHour(time.Now(), s.config.RefreshHour),
		WG:           s.WG(),
		StopChan:     s.Done(),
		Logger:       s.logger,
		LogFields:    []observability.Field{observability.Int("refreshHour", s.config.RefreshHour)},
	}, func(ctx context.Context) {
		s.runOnce(ctx, true) //nolint:errcheck
	})
}

// RunOnce runs a single refresh cycle. Exported for manual trigger. The manual
// path is never gated by the already-ran-today check — an operator pressing
// "Refresh" wants a real run even if the scheduled sweep already happened today.
func (s *CardLadderRefreshScheduler) RunOnce(ctx context.Context) error {
	return s.runOnce(ctx, false)
}

// PriceSinglePurchase resolves and applies CL pricing for one purchase on
// demand. Used by the intake-time pricing enqueuer so freshly scanned certs
// don't have to wait for the daily refresh cycle.
//
// The per-purchase path mirrors runOnce's phases 1–3 for a single cert:
// resolve gemRateID+condition (cached or via BuildCollectionCard), fetch the
// catalog value, update the purchase, and re-enroll for DH push on value
// change. Phases 4–6 (sales comps, CL-UI push/remove) are daily-only.
//
// Returns nil when the integration isn't configured — on-demand pricing is
// best-effort and must not surface errors to the intake flow.
func (s *CardLadderRefreshScheduler) PriceSinglePurchase(ctx context.Context, p *inventory.Purchase) error {
	if p == nil {
		return nil
	}
	// Skip if this purchase was priced within FreshPriceWindow — kills the
	// re-import / double-scan thrash. Daily refresh still runs on its own
	// schedule and is unaffected because it calls runOnce, not this path.
	if p.CLValueCents > 0 && isPriceFresh(p.CLValueUpdatedAt) {
		s.logger.Info(ctx, "CL price: skipping, value is fresh",
			observability.String("cert", p.CertNumber),
			observability.String("updatedAt", p.CLValueUpdatedAt))
		return nil
	}
	s.logger.Info(ctx, "CL price: invoked", observability.String("cert", p.CertNumber))
	client := s.getClient()
	if client == nil {
		s.logger.Info(ctx, "CL price: no client configured, skipping",
			observability.String("cert", p.CertNumber))
		return nil
	}
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return err
	}
	if cfg == nil {
		s.logger.Info(ctx, "CL price: config not found, skipping",
			observability.String("cert", p.CertNumber))
		return nil
	}

	if p.CertNumber == "" {
		s.recordCLError(ctx, p.ID, CLReasonNoCert)
		return nil
	}

	// Seed the mapping cache from storage so resolveGemRate reuses an existing
	// (gemRateID, condition) pair when available.
	mappingByCert := make(map[string]postgres.CLCardMapping, 1)
	if m, mErr := s.store.GetMapping(ctx, p.CertNumber); mErr == nil && m != nil {
		mappingByCert[p.CertNumber] = *m
	}

	gemRateID, condition, ok, _ := s.resolveGemRate(ctx, client, p, mappingByCert)
	if !ok {
		return nil // resolveGemRate already recorded the failure reason.
	}

	value, err := s.fetchCLEstimate(ctx, client, gemRateID, condition, p.CardName)
	if err != nil {
		s.recordCLError(ctx, p.ID, CLReasonAPIError)
		return nil
	}
	if value <= 0 {
		s.recordCLError(ctx, p.ID, CLReasonNoValue)
		return nil
	}

	newCLCents := mathutil.ToCentsInt(value)
	oldCLCents := p.CLValueCents
	if err := s.valueUpdater.UpdatePurchaseCLValue(ctx, p.ID, newCLCents, p.Population); err != nil {
		return err
	}
	// Keep the in-memory purchase consistent with the DB so the next pricer
	// in PricingEnrichJob (MM, today) and any caller iterating this purchase
	// operate on the freshly persisted value instead of a stale snapshot.
	p.CLValueCents = newCLCents
	s.recordCLError(ctx, p.ID, "")

	if s.dhPushUpdater != nil && newCLCents != oldCLCents && shouldReenrollForCLChange(p) {
		if err := s.dhPushUpdater.UpdatePurchaseDHPushStatus(ctx, p.ID, inventory.DHPushStatusPending); err != nil {
			s.logger.Warn(ctx, "CL price: failed to re-enroll for DH push",
				observability.String("cert", p.CertNumber),
				observability.Err(err))
		} else {
			p.DHPushStatus = inventory.DHPushStatusPending
			s.recordEvent(ctx, dhevents.Event{
				PurchaseID:    p.ID,
				CertNumber:    p.CertNumber,
				Type:          dhevents.TypeEnrolled,
				NewPushStatus: inventory.DHPushStatusPending,
				Source:        dhevents.SourceCLRefresh,
			})
		}
	}

	s.logger.Info(ctx, "CL price: applied on-demand pricing",
		observability.String("cert", p.CertNumber),
		observability.Int("clValueCents", newCLCents))
	return nil
}

// runOnce executes the cert-first refresh cycle:
//  1. List unsold purchases.
//  2. For each purchase with a cert, ensure we have a gemRateID+condition
//     (cached in cl_card_mappings, or resolved fresh via BuildCollectionCard).
//  3. Batch-fetch catalog values for all resolved gemRateIDs.
//  4. Apply values to purchases; record per-purchase errors for the admin UI.
//  5. Phase 2 (sales comps), Phase 4 (push to CL remote for UI hygiene),
//     Phase 5 (remove sold cards from CL remote).
func (s *CardLadderRefreshScheduler) runOnce(ctx context.Context, gated bool) error {
	start := time.Now()

	// On the scheduler path, skip if a full refresh already completed today
	// (UTC). The loop runs runOnce immediately on startup whenever RefreshHour
	// has already passed today (timeUntilHour → 0), so every redeploy after
	// 04:00 UTC would otherwise replay the entire CardEstimate sweep — the
	// dominant driver of the daily-quota exhaustion. GetLastRunStats reads the
	// persisted last-run across restarts (the stats store is always wired in
	// production), so this survives process bounces. A genuine missed window
	// (machine down at 04:00, booted later with no run today) still runs. The
	// manual trigger passes gated=false to bypass this — an operator pressing
	// "Refresh" wants a real run.
	if gated {
		if last := s.GetLastRunStats(ctx); last != nil && sameUTCDate(last.LastRunAt, start) {
			s.logger.Info(ctx, "CL refresh: already ran today, skipping startup catch-up",
				observability.String("lastRunAt", last.LastRunAt.UTC().Format(time.RFC3339)))
			return nil
		}
	}

	client := s.getClient()
	if client == nil {
		s.logger.Debug(ctx, "CL refresh: no client configured, skipping")
		return nil
	}

	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to load config", observability.Err(err))
		return err
	}
	if cfg == nil {
		s.logger.Debug(ctx, "CL refresh: not configured, skipping")
		return nil
	}

	purchases, err := s.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to list purchases", observability.Err(err))
		return err
	}

	existingMappings, err := s.store.ListMappings(ctx)
	if err != nil {
		s.logger.Warn(ctx, "CL refresh: failed to load card mappings — continuing without mapping cache", observability.Err(err))
	}
	mappingByCert := make(map[string]postgres.CLCardMapping, len(existingMappings))
	for _, m := range existingMappings {
		mappingByCert[m.SlabSerial] = m
	}

	// Phase 1: resolve (cert → gemRateID+condition) for every purchase that
	// needs it. Collect the gemRateIDs we care about so Phase 2 can batch
	// catalog fetches.
	type resolved struct {
		purchase  *inventory.Purchase
		gemRateID string
		condition string
	}
	resolvedPurchases := make([]resolved, 0, len(purchases))
	stats := CLRunStats{TotalPurchases: len(purchases)}

	for i := range purchases {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p := &purchases[i]
		if p.CertNumber == "" {
			stats.NoCert++
			s.recordCLError(ctx, p.ID, CLReasonNoCert)
			continue
		}

		gemRateID, condition, ok, quotaHit := s.resolveGemRate(ctx, client, p, mappingByCert)
		if quotaHit {
			// CL quota wall hit during cert resolution. Stop resolving — every
			// further BuildCollectionCard would fail identically. Mark the run
			// so Phase 2+3 also short-circuits its estimate calls.
			stats.QuotaExhausted = true
			stats.CertResolveFailed++
			s.logger.Warn(ctx, "CL refresh: daily quota exhausted during resolve, stopping cert resolution this cycle",
				observability.Int("resolvedSoFar", len(resolvedPurchases)))
			break
		}
		if !ok {
			stats.CertResolveFailed++
			continue
		}
		if _, cached := mappingByCert[p.CertNumber]; !cached {
			stats.Resolved++
		}
		resolvedPurchases = append(resolvedPurchases, resolved{p, gemRateID, condition})
	}

	// Phase 2+3: fetch the live CardEstimate for each resolved purchase and
	// apply it. Successful and no-value estimates are cached per (gemRateID,
	// condition) so multiple physical copies of the same card cost a single
	// callable — the dominant fix for quota burn, since the index doesn't
	// reliably surface these gemRateIDs and we hit the canonical estimate
	// function directly. (A hard error is not cached, so a persistently-failing
	// card still retries once per copy — acceptable, and lets transient errors
	// recover.) When CL's daily quota is hit, stop making further estimate calls
	// for this cycle (they would all fail identically) but keep applying values
	// we already fetched this run.
	type estimateKey struct{ gemRateID, condition string }
	estimates := make(map[estimateKey]float64, len(resolvedPurchases))
	for _, r := range resolvedPurchases {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := estimateKey{r.gemRateID, r.condition}
		value, cached := estimates[key]
		if !cached {
			if stats.QuotaExhausted {
				// Quota wall already hit this cycle — don't burn more calls.
				// Tagged quota_exhausted (not api_error) so the admin /failures
				// view doesn't read these as genuine CL failures: they were
				// never attempted.
				stats.SkippedQuota++
				s.recordCLError(ctx, r.purchase.ID, CLReasonQuotaExhausted)
				continue
			}
			var err error
			value, err = s.fetchCLEstimate(ctx, client, r.gemRateID, r.condition, r.purchase.CardName)
			if err != nil {
				if apperrors.HasErrorCode(err, apperrors.ErrCodeProviderRateLimit) {
					stats.QuotaExhausted = true
					stats.SkippedQuota++
					s.recordCLError(ctx, r.purchase.ID, CLReasonQuotaExhausted)
					s.logger.Warn(ctx, "CL refresh: daily quota exhausted, skipping remaining estimates this cycle",
						observability.Int("updatedSoFar", stats.Updated))
					continue
				}
				stats.EstimateFailed++
				s.recordCLError(ctx, r.purchase.ID, CLReasonAPIError)
				continue
			}
			estimates[key] = value
		}
		if value <= 0 {
			stats.NoValue++
			s.recordCLError(ctx, r.purchase.ID, CLReasonNoValue)
			continue
		}
		newCLCents := mathutil.ToCentsInt(value)
		oldCLCents := r.purchase.CLValueCents
		if err := s.valueUpdater.UpdatePurchaseCLValue(ctx, r.purchase.ID, newCLCents, r.purchase.Population); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to update CL value",
				observability.String("cert", r.purchase.CertNumber),
				observability.Err(err))
			continue
		}
		r.purchase.CLValueCents = newCLCents
		stats.Updated++
		s.recordCLError(ctx, r.purchase.ID, "")

		// Re-enroll for DH push when market value changes. Two qualifying cases:
		//  1. Already-pushed rows (DHInventoryID != 0) — so DH picks up the new price.
		//  2. Received-but-unmatched rows — so a fresh cert resolve is attempted
		//     with the new market value, which may push it above a price floor.
		if s.dhPushUpdater != nil && newCLCents != oldCLCents && shouldReenrollForCLChange(r.purchase) {
			if err := s.dhPushUpdater.UpdatePurchaseDHPushStatus(ctx, r.purchase.ID, inventory.DHPushStatusPending); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to re-enroll for DH push",
					observability.String("cert", r.purchase.CertNumber),
					observability.Err(err))
			} else {
				r.purchase.DHPushStatus = inventory.DHPushStatusPending
				s.recordEvent(ctx, dhevents.Event{
					PurchaseID:    r.purchase.ID,
					CertNumber:    r.purchase.CertNumber,
					Type:          dhevents.TypeEnrolled,
					NewPushStatus: inventory.DHPushStatusPending,
					Source:        dhevents.SourceCLRefresh,
				})
			}
		}
	}

	// Phase 4: refresh sales comps (decoupled — queries gem_rate_id directly).
	if s.salesStore != nil {
		s.refreshSalesCompsDecoupled(ctx, client)
	}

	// Phase 5: push new certs to the CL remote collection for UI hygiene.
	// Not required for pricing (that happens via BuildCollectionCard above) —
	// only matters so the user's CL collection view reflects our inventory.
	freshMappings, _ := s.store.ListMappings(ctx)
	cardsPushed := 0
	if cfg.FirebaseUID != "" {
		cardsPushed = s.pushNewCards(ctx, client, cfg.FirebaseUID, cfg.CollectionID, purchases, freshMappings)
	}
	stats.CardsPushed = cardsPushed

	// Phase 6: remove sold cards from the CL remote collection.
	cardsRemoved := 0
	if cfg.FirebaseUID != "" {
		cardsRemoved = s.removeSoldCards(ctx, client, cfg.FirebaseUID, cfg.CollectionID, purchases, freshMappings)
	}
	stats.CardsRemoved = cardsRemoved

	stats.LastRunAt = start
	stats.DurationMs = time.Since(start).Milliseconds()

	s.logger.Info(ctx, "CL refresh: complete",
		observability.Int("totalPurchases", stats.TotalPurchases),
		observability.Int("updated", stats.Updated),
		observability.Int("resolved", stats.Resolved),
		observability.Int("noCert", stats.NoCert),
		observability.Int("certResolveFailed", stats.CertResolveFailed),
		observability.Int("estimateFailed", stats.EstimateFailed),
		observability.Int("noValue", stats.NoValue),
		observability.Bool("quotaExhausted", stats.QuotaExhausted),
		observability.Int("skippedQuota", stats.SkippedQuota),
		observability.Int("cardsPushed", stats.CardsPushed),
		observability.Int("cardsRemoved", stats.CardsRemoved))

	s.statsMu.Lock()
	s.lastRunStats = &stats
	s.statsMu.Unlock()

	s.persistStats(ctx, stats)

	return nil
}
