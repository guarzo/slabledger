package scheduler

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// CardLadderPurchaseLister lists unsold purchases with their image URLs.
type CardLadderPurchaseLister interface {
	ListAllUnsoldPurchases(ctx context.Context) ([]campaigns.Purchase, error)
}

// CardLadderValueUpdater updates CL values on purchases.
type CardLadderValueUpdater interface {
	UpdatePurchaseCLValue(ctx context.Context, purchaseID string, clValueCents, population int) error
}

// CardLadderRefreshScheduler refreshes CL values from the Card Ladder API daily.
type CardLadderRefreshScheduler struct {
	StopHandle
	client         *cardladder.Client
	store          *sqlite.CardLadderStore
	purchaseLister CardLadderPurchaseLister
	valueUpdater   CardLadderValueUpdater
	clRecorder     campaigns.CLValueHistoryRecorder
	salesStore     *sqlite.CLSalesStore
	logger         observability.Logger
	config         config.CardLadderConfig
}

// NewCardLadderRefreshScheduler creates a new CL refresh scheduler.
func NewCardLadderRefreshScheduler(
	client *cardladder.Client,
	store *sqlite.CardLadderStore,
	purchaseLister CardLadderPurchaseLister,
	valueUpdater CardLadderValueUpdater,
	clRecorder campaigns.CLValueHistoryRecorder,
	salesStore *sqlite.CLSalesStore,
	logger observability.Logger,
	cfg config.CardLadderConfig,
) *CardLadderRefreshScheduler {
	cfg.ApplyDefaults()
	return &CardLadderRefreshScheduler{
		StopHandle:     NewStopHandle(),
		client:         client,
		store:          store,
		purchaseLister: purchaseLister,
		valueUpdater:   valueUpdater,
		clRecorder:     clRecorder,
		salesStore:     salesStore,
		logger:         logger,
		config:         cfg,
	}
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
var gradeDigitsRe = regexp.MustCompile(`(\d+)`)

func (s *CardLadderRefreshScheduler) runOnce(ctx context.Context) error {
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
	cards, err := s.client.FetchAllCollection(ctx, cfg.CollectionID)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to fetch collection", observability.Err(err))
		return err
	}
	s.logger.Info(ctx, "CL refresh: fetched collection",
		observability.Int("cardCount", len(cards)))

	// Load all unsold purchases for image URL matching
	purchases, err := s.purchaseLister.ListAllUnsoldPurchases(ctx)
	if err != nil {
		s.logger.Error(ctx, "CL refresh: failed to list purchases", observability.Err(err))
		return err
	}

	// Build image URL → purchase map for matching
	imageToPurchase := make(map[string]*campaigns.Purchase, len(purchases))
	certToPurchase := make(map[string]*campaigns.Purchase, len(purchases))
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
	existingMappings, err := s.store.ListMappings(ctx)
	if err != nil {
		s.logger.Warn(ctx, "CL refresh: failed to list mappings", observability.Err(err))
	}
	mappingByCLCardID := make(map[string]*sqlite.CLCardMapping, len(existingMappings))
	for i := range existingMappings {
		mappingByCLCardID[existingMappings[i].CLCollectionCardID] = &existingMappings[i]
	}

	updated, mapped, skipped := 0, 0, 0
	today := time.Now().UTC().Format("2006-01-02")

	for _, card := range cards {
		// Try to find the matching purchase
		var purchase *campaigns.Purchase

		// First check if we have a cached mapping
		if m, ok := mappingByCLCardID[card.CollectionCardID]; ok {
			purchase = certToPurchase[m.SlabSerial]
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
		if err := s.store.SaveMapping(ctx, purchase.CertNumber, card.CollectionCardID, "", card.Condition); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to save mapping",
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
		} else {
			mapped++
		}

		// Update CL value
		newCLCents := mathutil.ToCentsInt(card.CurrentValue)
		if newCLCents <= 0 {
			continue
		}

		if err := s.valueUpdater.UpdatePurchaseCLValue(ctx, purchase.ID, newCLCents, purchase.Population); err != nil {
			s.logger.Warn(ctx, "CL refresh: failed to update CL value",
				observability.String("cert", purchase.CertNumber),
				observability.Err(err))
			continue
		}

		// Record history
		if s.clRecorder != nil {
			gradeValue := extractGradeValue(card.Condition)
			if err := s.clRecorder.RecordCLValue(ctx, campaigns.CLValueEntry{
				CertNumber:      purchase.CertNumber,
				CardName:        purchase.CardName,
				SetName:         purchase.SetName,
				CardNumber:      purchase.CardNumber,
				GradeValue:      gradeValue,
				CLValueCents:    newCLCents,
				ObservationDate: today,
				Source:          "api_sync",
			}); err != nil {
				s.logger.Debug(ctx, "CL refresh: failed to record CL value history",
					observability.String("cert", purchase.CertNumber),
					observability.Err(err))
			}
		}
		updated++
	}

	// Phase 2: fetch sales comps for mapped cards with gemRateIDs
	if s.salesStore != nil {
		s.refreshSalesComps(ctx, existingMappings)
	}

	s.logger.Info(ctx, "CL refresh: complete",
		observability.Int("updated", updated),
		observability.Int("mapped", mapped),
		observability.Int("skipped", skipped),
		observability.Int("totalCLCards", len(cards)))
	return nil
}

func (s *CardLadderRefreshScheduler) refreshSalesComps(ctx context.Context, mappings []sqlite.CLCardMapping) {
	fetched := 0
	for _, m := range mappings {
		if m.CLGemRateID == "" || m.CLCondition == "" {
			continue
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := s.client.FetchSalesComps(ctx, m.CLGemRateID, m.CLCondition, "psa", 0, 100)
		if err != nil {
			s.logger.Warn(ctx, "CL sales: fetch failed",
				observability.String("gemRateId", m.CLGemRateID),
				observability.Err(err))
			continue
		}

		for _, comp := range resp.Hits {
			priceCents := mathutil.ToCentsInt(comp.Price)
			saleDate := comp.Date
			if len(saleDate) > 10 {
				saleDate = saleDate[:10]
			}
			if err := s.salesStore.UpsertSaleComp(ctx, sqlite.CLSaleCompRecord{
				GemRateID:   comp.GemRateID,
				ItemID:      comp.ItemID,
				SaleDate:    saleDate,
				PriceCents:  priceCents,
				Platform:    comp.Platform,
				ListingType: comp.ListingType,
				Seller:      comp.Seller,
				ItemURL:     comp.URL,
				SlabSerial:  comp.SlabSerial,
			}); err != nil {
				s.logger.Debug(ctx, "CL sales: upsert failed",
					observability.String("itemId", comp.ItemID),
					observability.Err(err))
			}
		}
		fetched++
	}

	s.logger.Info(ctx, "CL sales: refresh complete",
		observability.Int("cardsProcessed", fetched))
}

// extractGradeValue parses "PSA 9" or "g9" → 9.0
func extractGradeValue(condition string) float64 {
	if m := gradeDigitsRe.FindString(condition); m != "" {
		v, _ := strconv.ParseFloat(m, 64)
		return v
	}
	return 0
}
