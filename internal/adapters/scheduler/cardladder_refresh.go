package scheduler

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
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
	LastRunAt       time.Time `json:"lastRunAt"`
	DurationMs      int64     `json:"durationMs"`
	Updated         int       `json:"updated"`
	Mapped          int       `json:"mapped"`  // Count of successful SaveMapping calls; logically separate from Updated. SaveMapping failures (which only log) don't block Updated, and zero-value cards short-circuit to NoValue without incrementing Updated, so the two counters can diverge in either direction.
	Skipped         int       `json:"skipped"` // CL cards that did not match a purchase (CL-side perspective).
	TotalCLCards    int       `json:"totalCLCards"`
	CardsPushed     int       `json:"cardsPushed"`
	CardsRemoved    int       `json:"cardsRemoved"`
	OrphanMappings  int       `json:"orphanMappings"`  // Persistent mappings that neither matched a CL card this run nor correspond to a sold purchase.
	OrphansRepushed int       `json:"orphansRepushed"` // Orphan cards re-pushed to the CL collection this run.
	NoImageMatch    int       `json:"noImageMatch"`    // Unsold purchases with no CL card matched via image URL.
	NoCertMatch     int       `json:"noCertMatch"`     // Unsold purchases that also failed cert-regex fallback.
	NoValue         int       `json:"noValue"`         // Matched but both CL collection and cards catalog reported $0.
	CatalogFallback int       `json:"catalogFallback"` // Matched at $0 in collection but CL cards catalog had a non-zero value.
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

var certFromImageRe = regexp.MustCompile(`/cert/(\d+)/`)

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

	// Fetch all collection cards
	cards, err := client.FetchAllCollection(ctx, cfg.CollectionID)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to fetch collection", observability.Err(err))
		return err
	}
	s.logger.Info(ctx, "CL refresh: fetched collection",
		observability.Int("cardCount", len(cards)))

	// Fetch gemRateId data from Firestore
	var firestoreData map[string]cardladder.FirestoreCardData
	if cfg.FirebaseUID != "" {
		firestoreData, err = client.FetchFirestoreCards(ctx, cfg.FirebaseUID, cfg.CollectionID)
		if err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to fetch Firestore card data", observability.Err(err))
			// Continue without gemRateId data — values still sync, just no sales comps
		} else {
			s.logger.Info(ctx, "CL refresh: fetched Firestore data",
				observability.Int("cardsWithGemRate", len(firestoreData)))
		}
	}

	// Load all unsold purchases for image URL matching
	purchases, err := s.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to list purchases", observability.Err(err))
		return err
	}

	// Build image URL → purchase map for matching
	imageToPurchase := make(map[string]*inventory.Purchase, len(purchases))
	certToPurchase := make(map[string]*inventory.Purchase, len(purchases))
	for i := range purchases {
		p := &purchases[i]
		if p.FrontImageURL != "" {
			imageToPurchase[p.FrontImageURL] = p
		}
		if p.CertNumber != "" {
			certToPurchase[p.CertNumber] = p
		}
	}

	// Load existing mappings
	mappingsLoadErr := false
	existingMappings, err := s.store.ListMappings(ctx)
	if err != nil {
		s.logger.Warn(ctx, "CL refresh: failed to load card mappings — CL→purchase resolution, sales-comp, and collection reconciliation phases will be degraded or skipped this run", observability.Err(err))
		mappingsLoadErr = true
	}
	mappingByCLCardID := make(map[string]*sqlite.CLCardMapping, len(existingMappings))
	for i := range existingMappings {
		mappingByCLCardID[existingMappings[i].CLCollectionCardID] = &existingMappings[i]
	}

	// Batch-fetch live catalog values. The collectioncards index we already
	// loaded carries a stale `currentValue` snapshot; the `cards` index is
	// what CL's UI renders as "CL Value" and is the authoritative live price.
	catalogValues := s.fetchCatalogValues(ctx, client, collectGemRateIDs(firestoreData, existingMappings))
	s.logger.Info(ctx, "CL refresh: catalog values loaded",
		observability.Int("catalogEntries", len(catalogValues)))

	updated, mapped, skipped, noValue, catalogFallback := 0, 0, 0, 0, 0

	// Track which purchase IDs had a successful CL card match + value update
	// this run. Used for the second-pass error persistence over unmatched
	// purchases (so the admin UI shows per-card failure reasons).
	matchedPurchaseIDs := make(map[string]bool, len(purchases))
	// Track which existing mapping SlabSerials resolved to a CL card hit
	// this run. Any mapping NOT in this set but whose cert IS still in
	// unsoldCerts is an "orphan": a stored mapping that no longer points
	// to a live CL remote card — likely cleanup work the follow-up PR
	// will tackle after we see the logged samples.
	resolvedMappings := make(map[string]bool, len(existingMappings))

	for _, card := range cards {
		// Try to find the matching purchase
		var purchase *inventory.Purchase

		// First check if we have a cached mapping
		if m, ok := mappingByCLCardID[card.CollectionCardID]; ok {
			purchase = certToPurchase[m.SlabSerial]
			if purchase != nil {
				resolvedMappings[m.SlabSerial] = true
			}
		}

		// Primary match: image URL
		if purchase == nil && card.Image != "" {
			purchase = imageToPurchase[card.Image]
		}

		// Fallback: extract cert from image URL path
		if purchase == nil && card.Image != "" {
			if matches := certFromImageRe.FindStringSubmatch(card.Image); len(matches) > 1 {
				purchase = certToPurchase[matches[1]]
			}
		}

		if purchase == nil {
			skipped++
			continue
		}

		// Save/update mapping
		// Use Firestore gemRate data if available, otherwise preserve existing mapping values
		condition := card.Condition
		gemRateID := ""
		if fd, ok := firestoreData[card.CollectionCardID]; ok {
			gemRateID = fd.GemRateID
			if fd.GemRateCondition != "" {
				condition = fd.GemRateCondition
			}
		} else if existing, ok := mappingByCLCardID[card.CollectionCardID]; ok {
			// No Firestore entry — preserve previously stored gemRateID
			gemRateID = existing.CLGemRateID
		}

		if err := s.store.SaveMapping(ctx, purchase.CertNumber, card.CollectionCardID, gemRateID, condition); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to save mapping",
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
		} else {
			mapped++
		}

		// Persist gemRateID on purchase if resolved
		if gemRateID != "" && s.gemRateUpdater != nil && purchase.GemRateID == "" {
			if err := s.gemRateUpdater.UpdatePurchaseGemRateID(ctx, purchase.ID, gemRateID); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to persist gemRateID",
					observability.String("cert", purchase.CertNumber),
					observability.Err(err))
			} else {
				purchase.GemRateID = gemRateID // keep in-memory slice in sync for gap-fill phase
			}
		}

		// Update CL value. Prefer the live catalog value (matches CL's UI);
		// fall back to the collection snapshot when the catalog has no entry
		// for this (gemRateID, condition) pair. card.Condition is the
		// grade-string form ("PSA 9") used by the catalog index — NOT the
		// Firestore-sourced "g9" form captured in `condition` above.
		effectiveCLValue := pickCLValue(catalogValues, gemRateID, card.Condition, card.CurrentValue)
		newCLCents := mathutil.ToCentsInt(effectiveCLValue)
		fellBack := false
		if newCLCents <= 0 {
			// Collection index reports $0. Try the CL cards catalog (grade-specific,
			// market-wide view) — often has a usable value when the user's specific
			// collection entry is stuck at zero. Pass the canonical gemRateID and
			// condition already resolved above so the catalog lookup matches what
			// we just saved to the mapping.
			fallbackCents, fbErr := s.fetchCatalogFallbackValue(ctx, client, purchase, gemRateID, condition)
			if fbErr != nil {
				// API failure — distinct from a genuine catalog miss. Tag the row
				// so ops can see whether CL is misbehaving vs actually empty.
				matchedPurchaseIDs[purchase.ID] = true
				s.recordCLError(ctx, purchase.ID, CLReasonAPIError)
				continue
			}
			if fallbackCents <= 0 {
				// Catalog also has nothing. Persist so the admin UI can show
				// "matched without a price" distinct from "unmapped". Mark as
				// matched so the second pass doesn't overwrite the `no_value`
				// reason with `no_image_match`.
				noValue++
				matchedPurchaseIDs[purchase.ID] = true
				s.recordCLError(ctx, purchase.ID, CLReasonNoValue)
				continue
			}
			newCLCents = fallbackCents
			fellBack = true
			catalogFallback++
		}

		oldCLCents := purchase.CLValueCents
		if err := s.valueUpdater.UpdatePurchaseCLValue(ctx, purchase.ID, newCLCents, purchase.Population); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to update CL value",
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
			continue
		}

		// Re-enroll for DH push when market value changes. Two qualifying cases:
		//  1. Already-pushed rows (DHInventoryID != 0) — so DH picks up the new price.
		//  2. Received-but-unmatched rows — so a fresh cert resolve is attempted
		//     with the new market value, which may push it above a price floor.
		if s.dhPushUpdater != nil && newCLCents != oldCLCents && shouldReenrollForCLChange(purchase) {
			if err := s.dhPushUpdater.UpdatePurchaseDHPushStatus(ctx, purchase.ID, inventory.DHPushStatusPending); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to re-enroll for DH push",
					observability.String("cert", purchase.CertNumber),
					observability.Err(err))
			} else {
				s.recordEvent(ctx, dhevents.Event{
					PurchaseID:    purchase.ID,
					CertNumber:    purchase.CertNumber,
					Type:          dhevents.TypeEnrolled,
					NewPushStatus: inventory.DHPushStatusPending,
					Source:        dhevents.SourceCLRefresh,
				})
			}
		}

		updated++
		matchedPurchaseIDs[purchase.ID] = true
		resolvedMappings[purchase.CertNumber] = true
		if fellBack {
			// Tag rows priced from the catalog fallback so operators can see
			// which rows are using the workaround, and how many.
			s.recordCLError(ctx, purchase.ID, CLReasonCatalogFallback)
		} else {
			// Clear any prior CL error now that this purchase is successfully priced.
			s.recordCLError(ctx, purchase.ID, "")
		}
	}

	// Second pass: for every unsold purchase that did NOT get matched this run,
	// persist a failure reason tag so the admin UI can group and show why.
	// This is how we know which cards to investigate in the follow-up PR.
	noImageMatch, noCertMatch := 0, 0
	for i := range purchases {
		p := &purchases[i]
		if matchedPurchaseIDs[p.ID] {
			continue
		}
		reason := CLReasonNoImageMatch
		if p.CertNumber == "" {
			reason = CLReasonNoCertMatch
			noCertMatch++
		} else {
			noImageMatch++
		}
		s.recordCLError(ctx, p.ID, reason)
	}

	// Orphan audit: persistent mappings whose cert is still in the unsold
	// set but didn't resolve to a CL card this run — the card is missing
	// from the remote CL collection. Re-push these so CL has the card and
	// future sync runs can update values.
	orphanMappings := 0
	var orphanCerts []string
	for _, m := range existingMappings {
		if resolvedMappings[m.SlabSerial] {
			continue
		}
		if _, stillUnsold := certToPurchase[m.SlabSerial]; stillUnsold {
			orphanMappings++
			orphanCerts = append(orphanCerts, m.SlabSerial)
		}
	}
	orphansRepushed := 0
	if orphanMappings > 0 && cfg.FirebaseUID != "" {
		s.logger.Info(ctx, "CL refresh: re-pushing orphan cards missing from collection",
			observability.Int("orphanMappings", orphanMappings))
		// Note: pushSingleCard calls ResolveAndCreateCard which always creates
		// a new Firestore doc. If the remote card exists but our fetch missed it,
		// this could create a duplicate. CL has no "create-if-not-exists" API.
		// SaveMapping (upsert by slab_serial PK) overwrites the old mapping with
		// the new doc ID, so locally we stay consistent. A Firestore-side dedup
		// would require a new CL client method to query by cert first.
		for _, cert := range orphanCerts {
			if ctx.Err() != nil {
				break
			}
			p := certToPurchase[cert]
			grader := strings.ToLower(p.Grader)
			if grader == "" {
				grader = "psa"
			}
			if err := s.pushSingleCard(ctx, client, cfg.FirebaseUID, cfg.CollectionID, p, grader); err != nil {
				s.logger.Warn(ctx, "CL refresh: failed to re-push orphan card",
					observability.String("cert", cert),
					observability.Err(err))
				continue
			}
			orphansRepushed++
		}
		s.logger.Info(ctx, "CL refresh: orphan re-push complete",
			observability.Int("orphans", orphanMappings),
			observability.Int("repushed", orphansRepushed))
	}

	// Phase 2: fetch sales comps for mapped cards with gemRateIDs.
	// Note: newly created mappings from this run are intentionally deferred
	// to the next refresh cycle to avoid extra API calls during initial sync.
	if s.salesStore != nil {
		s.refreshSalesComps(ctx, client, existingMappings)
	}

	// Phase 3: gap-fill gemRateIDs for purchases not matched via collection/Firestore.
	if s.gemRateUpdater != nil {
		s.gapFillGemRateIDs(ctx, client, purchases)
	}

	// Phase 4: push new unsold purchases (with certs) that aren't yet in the CL collection.
	// Reload mappings so any new mappings created by the main loop are included in the
	// dedup check — using the stale snapshot would cause the main loop's newly-mapped
	// certs to appear unmapped and get pushed a second time (duplicate Firestore docs).
	// Skip entirely if the original ListMappings call failed: we cannot safely determine
	// which certs are already in the remote collection.
	cardsPushed := 0
	if cfg.FirebaseUID != "" && !mappingsLoadErr {
		freshMappings, freshErr := s.store.ListMappings(ctx)
		if freshErr != nil {
			s.logger.Warn(ctx, "CL push: skipping — could not reload mappings for dedup check", observability.Err(freshErr))
		} else {
			cardsPushed = s.pushNewCards(ctx, client, cfg.FirebaseUID, cfg.CollectionID, purchases, freshMappings)
		}
	}

	// Phase 5: remove sold cards from the CL collection.
	// Uses existingMappings (the pre-loop snapshot) intentionally: sold cards were already
	// absent from Firestore before this run began, so freshness is not required for deletes.
	// Phase 4 needed a fresh snapshot to avoid re-pushing cards just mapped in the main loop;
	// Phase 5 has no equivalent hazard — it only deletes mappings whose cert is no longer
	// in the unsold set, which cannot change within a single run.
	cardsRemoved := 0
	if cfg.FirebaseUID != "" {
		cardsRemoved = s.removeSoldCards(ctx, client, cfg.FirebaseUID, cfg.CollectionID, purchases, existingMappings)
	}

	s.logger.Info(ctx, "CL refresh: complete",
		observability.Int("updated", updated),
		observability.Int("mapped", mapped),
		observability.Int("skipped", skipped),
		observability.Int("totalCLCards", len(cards)),
		observability.Int("pushed", cardsPushed),
		observability.Int("removed", cardsRemoved),
		observability.Int("orphanMappings", orphanMappings),
		observability.Int("orphansRepushed", orphansRepushed),
		observability.Int("noImageMatch", noImageMatch),
		observability.Int("noCertMatch", noCertMatch),
		observability.Int("noValue", noValue),
		observability.Int("catalogFallback", catalogFallback))

	s.statsMu.Lock()
	s.lastRunStats = &CLRunStats{
		LastRunAt:       start,
		DurationMs:      time.Since(start).Milliseconds(),
		Updated:         updated,
		Mapped:          mapped,
		Skipped:         skipped,
		TotalCLCards:    len(cards),
		CardsPushed:     cardsPushed,
		CardsRemoved:    cardsRemoved,
		OrphanMappings:  orphanMappings,
		OrphansRepushed: orphansRepushed,
		NoImageMatch:    noImageMatch,
		NoCertMatch:     noCertMatch,
		NoValue:         noValue,
		CatalogFallback: catalogFallback,
	}
	s.statsMu.Unlock()

	return nil
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
