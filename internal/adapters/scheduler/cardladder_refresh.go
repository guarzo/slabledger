package scheduler

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/constants"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
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
	CardsPushed       int       `json:"cardsPushed"`       // Cards pushed to CL remote collection (UI hygiene).
	CardsRemoved      int       `json:"cardsRemoved"`      // Sold cards removed from CL remote collection (UI hygiene).
}

// CardLadderRefreshScheduler refreshes CL values from the Card Ladder API daily.
type CardLadderRefreshScheduler struct {
	StopHandle
	statsMu        sync.RWMutex
	clientMu       sync.RWMutex
	client         *cardladder.Client
	store          *sqlite.CardLadderStore
	purchaseLister CardLadderPurchaseLister
	valueUpdater   CardLadderValueUpdater
	gemRateUpdater CardLadderGemRateUpdater
	syncUpdater    CardLadderSyncUpdater // optional: sets cl_synced_at on push
	salesStore     *sqlite.CLSalesStore
	dhPushUpdater  DHPushStatusUpdater // optional: re-enrolls changed items for DH push
	eventRec       dhevents.Recorder   // optional: records DH state-transition events
	logger         observability.Logger
	config         config.CardLadderConfig
	lastRunStats   *CLRunStats
}

// GetLastRunStats returns a copy of the stats from the most recent refresh run,
// or nil if no run has completed yet.
func (s *CardLadderRefreshScheduler) GetLastRunStats() *CLRunStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	if s.lastRunStats == nil {
		return nil
	}
	cp := *s.lastRunStats
	return &cp
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
	store *sqlite.CardLadderStore,
	purchaseLister CardLadderPurchaseLister,
	valueUpdater CardLadderValueUpdater,
	gemRateUpdater CardLadderGemRateUpdater,
	salesStore *sqlite.CLSalesStore,
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
		s.runOnce(ctx) //nolint:errcheck
	})
}

// RunOnce runs a single refresh cycle. Exported for manual trigger.
func (s *CardLadderRefreshScheduler) RunOnce(ctx context.Context) error {
	return s.runOnce(ctx)
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
	mappingByCert := make(map[string]sqlite.CLCardMapping, 1)
	if m, mErr := s.store.GetMapping(ctx, p.CertNumber); mErr == nil && m != nil {
		mappingByCert[p.CertNumber] = *m
	}

	gemRateID, condition, ok := s.resolveGemRate(ctx, client, p, mappingByCert)
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
func (s *CardLadderRefreshScheduler) runOnce(ctx context.Context) error {
	start := time.Now()

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
	mappingByCert := make(map[string]sqlite.CLCardMapping, len(existingMappings))
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

		gemRateID, condition, ok := s.resolveGemRate(ctx, client, p, mappingByCert)
		if !ok {
			stats.CertResolveFailed++
			continue
		}
		if _, cached := mappingByCert[p.CertNumber]; !cached {
			stats.Resolved++
		}
		resolvedPurchases = append(resolvedPurchases, resolved{p, gemRateID, condition})
	}

	// Phase 2+3: fetch the live CardEstimate for each resolved (gemRateID,
	// condition) pair and apply it. One callable per card — the public `cards`
	// search index doesn't reliably surface the gemRateIDs BuildCollectionCard
	// returns, so we hit the canonical estimate function directly.
	for _, r := range resolvedPurchases {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		value, err := s.fetchCLEstimate(ctx, client, r.gemRateID, r.condition, r.purchase.CardName)
		if err != nil {
			stats.EstimateFailed++
			s.recordCLError(ctx, r.purchase.ID, CLReasonAPIError)
			continue
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

	// Phase 4: refresh sales comps (uses the fresh mapping set).
	freshMappings, _ := s.store.ListMappings(ctx)
	if s.salesStore != nil {
		s.refreshSalesComps(ctx, client, freshMappings)
	}

	// Phase 5: push new certs to the CL remote collection for UI hygiene.
	// Not required for pricing (that happens via BuildCollectionCard above) —
	// only matters so the user's CL collection view reflects our inventory.
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
		observability.Int("cardsPushed", stats.CardsPushed),
		observability.Int("cardsRemoved", stats.CardsRemoved))

	s.statsMu.Lock()
	s.lastRunStats = &stats
	s.statsMu.Unlock()

	return nil
}

// resolveGemRate returns the (gemRateID, condition) pair for a purchase, using
// the cached mapping when possible and calling BuildCollectionCard when not.
// On success it persists the mapping and the purchase's gemRateID. On failure
// it records a CLReasonAPIError tag and returns ok=false.
func (s *CardLadderRefreshScheduler) resolveGemRate(
	ctx context.Context,
	client *cardladder.Client,
	p *inventory.Purchase,
	mappingByCert map[string]sqlite.CLCardMapping,
) (gemRateID, condition string, ok bool) {
	// Bypass the cache when set_name is generic so BuildCollectionCard fires
	// and the repair block below (which needs resp.Set) can run. Without this,
	// rows with pre-existing cached mappings (typical for certs whose previous
	// CL resolution only populated gemRateID+condition) would keep their
	// generic set_name forever.
	if m, cached := mappingByCert[p.CertNumber]; cached && m.CLGemRateID != "" && m.CLCondition != "" && !constants.IsGenericSetName(p.SetName) {
		return m.CLGemRateID, m.CLCondition, true
	}

	grader := strings.ToLower(p.Grader)
	if grader == "" {
		grader = "psa"
	}

	resp, err := client.BuildCollectionCard(ctx, p.CertNumber, grader)
	if err != nil {
		s.logger.Warn(ctx, "CL refresh: BuildCollectionCard failed",
			observability.String("cert", p.CertNumber),
			observability.Err(err))
		s.recordCLError(ctx, p.ID, CLReasonAPIError)
		return "", "", false
	}
	if resp.GemRateID == "" || resp.Condition == "" {
		s.logger.Warn(ctx, "CL refresh: BuildCollectionCard returned no gemRateID or condition",
			observability.String("cert", p.CertNumber),
			observability.String("gemRateId", resp.GemRateID),
			observability.String("condition", resp.Condition))
		s.recordCLError(ctx, p.ID, CLReasonCertResolveFailed)
		return "", "", false
	}

	if err := s.store.SaveMappingPricing(ctx, p.CertNumber, resp.GemRateID, resp.Condition); err != nil {
		s.logger.Warn(ctx, "CL refresh: failed to save pricing mapping",
			observability.String("cert", p.CertNumber),
			observability.Err(err))
		// Soft failure — we can still price this run, but the mapping won't
		// be cached next run. Return the resolved values anyway.
	} else {
		mappingByCert[p.CertNumber] = sqlite.CLCardMapping{
			SlabSerial:  p.CertNumber,
			CLGemRateID: resp.GemRateID,
			CLCondition: resp.Condition,
		}
	}
	if s.gemRateUpdater != nil {
		if p.GemRateID == "" {
			if err := s.gemRateUpdater.UpdatePurchaseGemRateID(ctx, p.ID, resp.GemRateID); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to persist gemRateID on purchase",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
			} else {
				p.GemRateID = resp.GemRateID
			}
		}
		if resp.Player != "" || resp.Variation != "" || resp.Category != "" {
			if err := s.gemRateUpdater.UpdatePurchaseCLCardMetadata(ctx, p.ID, resp.Player, resp.Variation, resp.Category); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to persist card metadata",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
			}
		}
		// Repair set_name when PSA returned a generic value (e.g. "TCG Cards"
		// for older certs). CL's Set field carries the real set for any cert
		// CL can resolve, so adopt it only when the current value is generic.
		if constants.IsGenericSetName(p.SetName) && !constants.IsGenericSetName(resp.Set) {
			if err := s.gemRateUpdater.UpdatePurchaseSetName(ctx, p.ID, resp.Set); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to persist set name from CL",
					observability.String("cert", p.CertNumber),
					observability.String("clSet", resp.Set),
					observability.Err(err))
			} else {
				p.SetName = resp.Set
			}
		}
	}
	return resp.GemRateID, resp.Condition, true
}

// shouldReenrollForCLChange returns true when a CL value change should
// trigger DH push-pipeline re-enrollment. Two qualifying cases:
//  1. Already-pushed rows (DHInventoryID != 0) — re-enrolled so DH picks up
//     the new price.
//  2. Received-but-unmatched rows — re-enrolled so a fresh cert resolve is
//     attempted with the new market value, which may push it above a floor.
func shouldReenrollForCLChange(p *inventory.Purchase) bool {
	if p.DHInventoryID != 0 {
		return true
	}
	if p.ReceivedAt != nil && (p.DHPushStatus == inventory.DHPushStatusUnmatched || p.DHPushStatus == "") {
		return true
	}
	return false
}
