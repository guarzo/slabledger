package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// MMPurchaseLister lists unsold purchases for Market Movers value sync.
type MMPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]inventory.Purchase, error)
}

// MMValueUpdater updates Market Movers values on purchases.
type MMValueUpdater interface {
	// UpdatePurchaseMMValue updates only the avg price (used by the CSV import path).
	UpdatePurchaseMMValue(ctx context.Context, purchaseID string, mmValueCents int) error
	// UpdatePurchaseMMSignals updates all MM signals in one statement (used by the scheduler).
	UpdatePurchaseMMSignals(ctx context.Context, id string, mmValueCents int, mmTrendPct float64, mmSales30d, mmActiveLowCents int) error
	// UpdatePurchaseMMError records or clears the last mapping/pricing failure reason.
	// Pass reason="" and reasonAt="" to clear on success.
	UpdatePurchaseMMError(ctx context.Context, id, reason, reasonAt string) error
}

// MM failure reason tags. Short, stable strings — not free-form messages —
// so the /failures admin endpoint can group and display them.
const (
	MMReasonNoCardName        = "no_card_name"
	MMReasonNoCertResults     = "no_cert_results"
	MMReasonCertTokenMismatch = "cert_token_mismatch"
	MMReasonNoNameResults     = "no_name_results"
	MMReasonNameTokenMismatch = "name_token_mismatch"
	MMReasonNoSalesData       = "no_30d_sales"
	MMReasonAPIError          = "api_error"
)

// MMRunStats holds the counters from the most recent Market Movers refresh run.
type MMRunStats struct {
	LastRunAt       time.Time `json:"lastRunAt"`
	DurationMs      int64     `json:"durationMs"`
	Updated         int       `json:"updated"`
	NewMappings     int       `json:"newMappings"`
	Skipped         int       `json:"skipped"`
	SearchFailed    int       `json:"searchFailed"`
	TotalPurchases  int       `json:"totalPurchases"`
	TokenMismatches int       `json:"tokenMismatches"` // cert or name search returned hits but all rejected by tokenMatchesTitle
	NoSalesData     int       `json:"noSalesData"`     // mapping existed but 30-day avg was 0
	UploadedLastRun int       `json:"uploadedLastRun"` // placeholder: MM refresh never auto-pushes today
	DeletedLastRun  int       `json:"deletedLastRun"`  // placeholder: MM refresh never auto-deletes today
}

// MarketMoversRefreshScheduler refreshes MM values from the Market Movers API daily.
type MarketMoversRefreshScheduler struct {
	StopHandle
	clientMu       sync.RWMutex
	statsMu        sync.RWMutex
	client         *marketmovers.Client
	store          *sqlite.MarketMoversStore
	purchaseLister MMPurchaseLister
	valueUpdater   MMValueUpdater
	logger         observability.Logger
	config         config.MarketMoversConfig
	lastRunStats   *MMRunStats
}

// GetLastRunStats returns a copy of the stats from the most recent refresh run,
// or nil if no run has completed yet.
func (s *MarketMoversRefreshScheduler) GetLastRunStats() *MMRunStats {
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
func (s *MarketMoversRefreshScheduler) SetClient(client *marketmovers.Client) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	s.client = client
}

// getClient returns the current API client under the lock.
func (s *MarketMoversRefreshScheduler) getClient() *marketmovers.Client {
	s.clientMu.RLock()
	defer s.clientMu.RUnlock()
	return s.client
}

// NewMarketMoversRefreshScheduler creates a new Market Movers refresh scheduler.
func NewMarketMoversRefreshScheduler(
	client *marketmovers.Client,
	store *sqlite.MarketMoversStore,
	purchaseLister MMPurchaseLister,
	valueUpdater MMValueUpdater,
	logger observability.Logger,
	cfg config.MarketMoversConfig,
) *MarketMoversRefreshScheduler {
	return &MarketMoversRefreshScheduler{
		StopHandle:     NewStopHandle(),
		client:         client,
		store:          store,
		purchaseLister: purchaseLister,
		valueUpdater:   valueUpdater,
		logger:         logger,
		config:         cfg,
	}
}

// Start begins the daily scheduler loop.
func (s *MarketMoversRefreshScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		s.logger.Info(ctx, "Market Movers refresh scheduler disabled")
		return
	}

	RunLoop(ctx, LoopConfig{
		Name:         "market-movers-refresh",
		Interval:     24 * time.Hour,
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
func (s *MarketMoversRefreshScheduler) RunOnce(ctx context.Context) error {
	return s.runOnce(ctx)
}

func (s *MarketMoversRefreshScheduler) runOnce(ctx context.Context) error {
	start := time.Now()
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		s.logger.Error(ctx, "MM refresh: failed to load config", observability.Err(err))
		return err
	}
	if cfg == nil {
		s.logger.Debug(ctx, "MM refresh: not configured, skipping")
		return nil
	}

	// Capture client reference once and hold it for the entire refresh operation
	// to prevent race conditions if SetClient() is called during execution.
	client := s.getClient()
	if client == nil {
		s.logger.Warn(ctx, "MM refresh: client not initialized, skipping (credentials may have been set via UI — restart or save credentials again)")
		return nil
	}

	// List all unsold purchases
	purchases, err := s.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		s.logger.Error(ctx, "MM refresh: failed to list purchases", observability.Err(err))
		return err
	}

	// Load all existing mappings keyed by cert
	existingMappings, err := s.store.ListMappings(ctx)
	if err != nil {
		s.logger.Warn(ctx, "MM refresh: failed to list mappings", observability.Err(err))
	}
	mappingByCert := make(map[string]sqlite.MMCardMapping, len(existingMappings))
	for _, m := range existingMappings {
		mappingByCert[m.SlabSerial] = m
	}

	var counts struct {
		updated, mapped, skipped, searchFailed int
		tokenMismatches, noSalesData           int
	}

	// Look back 30 days for daily stats
	dateFrom := time.Now().UTC().AddDate(0, 0, -30)

	for i := range purchases {
		p := &purchases[i]

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Resolve collectible ID — use cached mapping or search the API
		mapping, hasCached := mappingByCert[p.CertNumber]
		if !hasCached {
			cid, mid, searchTitle, reason, err := s.resolveCollectibleID(ctx, p)
			if err != nil {
				s.logger.Warn(ctx, "MM refresh: failed to resolve collectible ID",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
				counts.searchFailed++
				s.recordMMError(ctx, p.ID, MMReasonAPIError)
				continue
			}
			if cid == 0 {
				counts.skipped++
				if reason == MMReasonCertTokenMismatch || reason == MMReasonNameTokenMismatch {
					counts.tokenMismatches++
				}
				s.recordMMError(ctx, p.ID, reason)
				continue
			}
			mapping = sqlite.MMCardMapping{SlabSerial: p.CertNumber, MMCollectibleID: cid, MasterID: mid, SearchTitle: searchTitle}
			if err := s.store.SaveMapping(ctx, p.CertNumber, cid, mid, searchTitle); err != nil {
				s.logger.Warn(ctx, "MM refresh: failed to save mapping",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
			} else {
				mappingByCert[p.CertNumber] = mapping
				counts.mapped++
			}
		}

		// Fetch 30-day daily stats — derive avg price, trend %, and sales volume in one call
		stats, err := client.FetchDailyStats(ctx, mapping.MMCollectibleID, dateFrom)
		if err != nil {
			s.logger.Warn(ctx, "MM refresh: failed to fetch daily stats",
				observability.String("cert", p.CertNumber),
				observability.Int64("collectibleId", mapping.MMCollectibleID),
				observability.Err(err))
			s.recordMMError(ctx, p.ID, MMReasonAPIError)
			continue
		}

		avgPrice, trendPct, sales30d := computeMMSignals(stats.DailyStats)
		if avgPrice <= 0 {
			// Mapping exists but no sales data in the 30-day window. Persist so
			// admin UI can show "mapped but no price" cards distinct from
			// "unmappable" cards.
			counts.noSalesData++
			s.recordMMError(ctx, p.ID, MMReasonNoSalesData)
			continue
		}

		mmValueCents := mathutil.ToCentsInt(avgPrice)
		activeLowCents := s.fetchActiveLowCents(ctx, client, mapping.MMCollectibleID, p.CertNumber)

		if err := s.valueUpdater.UpdatePurchaseMMSignals(ctx, p.ID, mmValueCents, trendPct, sales30d, activeLowCents); err != nil {
			s.logger.Warn(ctx, "MM refresh: failed to update MM signals",
				observability.String("cert", p.CertNumber),
				observability.Err(err))
			s.recordMMError(ctx, p.ID, MMReasonAPIError)
			continue
		}
		counts.updated++
		// Clear any prior error on this purchase now that it's successfully priced.
		s.recordMMError(ctx, p.ID, "")
	}

	s.logger.Info(ctx, "MM refresh: complete",
		observability.Int("updated", counts.updated),
		observability.Int("newMappings", counts.mapped),
		observability.Int("skipped", counts.skipped),
		observability.Int("tokenMismatches", counts.tokenMismatches),
		observability.Int("noSalesData", counts.noSalesData),
		observability.Int("searchFailed", counts.searchFailed),
		observability.Int("totalPurchases", len(purchases)))

	s.statsMu.Lock()
	s.lastRunStats = &MMRunStats{
		LastRunAt:       start,
		DurationMs:      time.Since(start).Milliseconds(),
		Updated:         counts.updated,
		NewMappings:     counts.mapped,
		Skipped:         counts.skipped,
		SearchFailed:    counts.searchFailed,
		TotalPurchases:  len(purchases),
		TokenMismatches: counts.tokenMismatches,
		NoSalesData:     counts.noSalesData,
	}
	s.statsMu.Unlock()

	return nil
}

// recordMMError persists a failure reason (or clears it when reason=="") on a
// purchase. Never fails the refresh loop — diagnostics are best-effort — but
// the admin UI depends on these rows, so a persistence failure is logged at
// Warn level so operators see it.
func (s *MarketMoversRefreshScheduler) recordMMError(ctx context.Context, purchaseID, reason string) {
	var reasonAt string
	if reason != "" {
		reasonAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := s.valueUpdater.UpdatePurchaseMMError(ctx, purchaseID, reason, reasonAt); err != nil {
		s.logger.Warn(ctx, "MM refresh: failed to persist error reason",
			observability.String("purchaseId", purchaseID),
			observability.String("reason", reason),
			observability.Err(err))
	}
}

// resolveCollectibleID searches Market Movers for the card and returns its collectible ID,
// master ID (grade-agnostic variant identifier, 0 if unknown), and the canonical SearchTitle.
// When the search yields nothing usable, collectibleID is 0 and reason is set to one of
// the MMReason* constants so the caller can persist it for the admin UI.
//
// Strategy:
//  1. Search by cert number — we embed the cert in the MM export Notes column, and MM
//     indexes PSA cert numbers so this is the most precise lookup available.
//  2. Fall back to a "{CardName} {Grader} {Grade}" text query if the cert search yields
//     no result that matches the card name.
//
// Any candidate returned by either path is validated via tokenized title matching (see
// tokenMatchesTitle) before the ID is cached.
func (s *MarketMoversRefreshScheduler) resolveCollectibleID(ctx context.Context, p *inventory.Purchase) (collectibleID, masterID int64, searchTitle, reason string, err error) {
	if p.CardName == "" {
		return 0, 0, "", MMReasonNoCardName, nil
	}

	// 1. Try cert number first. Cert-search miss is NOT terminal — fall through
	// to name search so we still get a chance to map.
	certReason := ""
	if p.CertNumber != "" {
		cid, mid, title, r, cerr := s.searchByCert(ctx, p)
		if cerr != nil {
			s.logger.Warn(ctx, "MM: cert-based search failed, falling back to name search",
				observability.String("cert", p.CertNumber),
				observability.Err(cerr))
		} else if cid != 0 {
			s.logger.Info(ctx, "MM: resolved collectible via cert search",
				observability.String("cert", p.CertNumber),
				observability.Int64("collectibleId", cid))
			return cid, mid, title, "", nil
		} else {
			certReason = r
		}
	}

	// 2. Fall back to name + grade search with relevance validation.
	cid, mid, title, nameReason, err := s.searchByNameGrade(ctx, p)
	if err != nil || cid != 0 {
		return cid, mid, title, "", err
	}

	// Both paths failed. Prefer the more specific token-mismatch reason
	// (which tells us MM DID have candidates) over the no-results reason.
	combined := nameReason
	if certReason == MMReasonCertTokenMismatch || nameReason == "" {
		combined = certReason
	}
	return 0, 0, "", combined, nil
}

// searchByCert searches MM using the PSA cert number as the query. Returns a
// reason code when no usable result is found (0 results vs. all-rejected).
func (s *MarketMoversRefreshScheduler) searchByCert(ctx context.Context, p *inventory.Purchase) (collectibleID, masterID int64, searchTitle, reason string, err error) {
	results, err := s.getClient().SearchCollectibles(ctx, p.CertNumber, 0, 3)
	if err != nil {
		return 0, 0, "", "", fmt.Errorf("search by cert: %w", err)
	}
	if len(results.Items) == 0 {
		return 0, 0, "", MMReasonNoCertResults, nil
	}
	for _, r := range results.Items {
		if tokenMatchesTitle(p.CardName, r.Item.SearchTitle) {
			return r.Item.ID, r.Item.MasterID, r.Item.SearchTitle, "", nil
		}
	}
	// Promoted from Debug: this is the evidence we need to calibrate
	// tokenMatchesTitle in the follow-up fix PR.
	s.logger.Info(ctx, "MM: cert search all results rejected by token match",
		observability.String("cert", p.CertNumber),
		observability.String("cardName", p.CardName),
		observability.String("sampleResultTitle", results.Items[0].Item.SearchTitle),
		observability.Int("resultCount", len(results.Items)))
	return 0, 0, "", MMReasonCertTokenMismatch, nil
}

// searchByNameGrade searches MM using "{CardName} {Grader} {Grade}" and validates
// that the top result's SearchTitle contains the card name before returning the IDs.
// Returns a reason code when no usable result is found.
func (s *MarketMoversRefreshScheduler) searchByNameGrade(ctx context.Context, p *inventory.Purchase) (collectibleID, masterID int64, searchTitle, reason string, err error) {
	grader := p.Grader
	if grader == "" {
		grader = "PSA"
	}
	// Omit grade when it is zero (unset) to avoid a spurious "0" token in the query.
	var query string
	if p.GradeValue == 0 {
		query = fmt.Sprintf("%s %s", p.CardName, grader)
	} else {
		query = fmt.Sprintf("%s %s %s", p.CardName, grader, mathutil.FormatGrade(p.GradeValue))
	}

	results, err := s.getClient().SearchCollectibles(ctx, query, 0, 5)
	if err != nil {
		return 0, 0, "", "", fmt.Errorf("search by name: %w", err)
	}
	if len(results.Items) == 0 {
		return 0, 0, "", MMReasonNoNameResults, nil
	}

	// Validate relevance: reject the top result if tokenized matching fails,
	// since a mismatch means MM returned a completely unrelated card.
	top := results.Items[0]
	if !tokenMatchesTitle(p.CardName, top.Item.SearchTitle) {
		// Promoted from Debug: needed to diagnose mapping failures from logs.
		s.logger.Info(ctx, "MM: name search top result rejected by token match",
			observability.String("cert", p.CertNumber),
			observability.String("cardName", p.CardName),
			observability.String("query", query),
			observability.String("resultTitle", top.Item.SearchTitle))
		return 0, 0, "", MMReasonNameTokenMismatch, nil
	}

	s.logger.Info(ctx, "MM: resolved collectible via name search",
		observability.String("cert", p.CertNumber),
		observability.String("query", query),
		observability.String("resultTitle", top.Item.SearchTitle),
		observability.Int64("collectibleId", top.Item.ID))
	return top.Item.ID, top.Item.MasterID, top.Item.SearchTitle, "", nil
}

// fetchActiveLowCents returns the lowest active Buy-It-Now price for a collectible in cents.
// Returns 0 if no BIN listings are found or the call fails (non-fatal — active price is
// supplementary data and should not block the rest of the refresh).
func (s *MarketMoversRefreshScheduler) fetchActiveLowCents(ctx context.Context, client *marketmovers.Client, collectibleID int64, cert string) int {
	resp, err := client.FetchActiveSales(ctx, []int64{collectibleID}, []string{"BuyItNow"}, 0, 10)
	if err != nil {
		s.logger.Debug(ctx, "MM refresh: failed to fetch active sales (non-fatal)",
			observability.String("cert", cert),
			observability.Int64("collectibleId", collectibleID),
			observability.Err(err))
		return 0
	}

	var lowestBIN float64
	for _, item := range resp.Items {
		if !item.IsBuyItNowAvailable || item.BuyItNowPrice <= 0 {
			continue
		}
		if lowestBIN == 0 || item.BuyItNowPrice < lowestBIN {
			lowestBIN = item.BuyItNowPrice
		}
	}

	return mathutil.ToCentsInt(lowestBIN)
}
