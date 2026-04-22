package main

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	dhlistingadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/psa"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/dhevents"
	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/pricing/lookup"
	"github.com/guarzo/slabledger/internal/domain/tuning"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// exportReaderComposite satisfies export.ExportReader by composing the three
// required stores. Named explicitly so that adding methods to export.ExportReader
// produces a compile error here if any store doesn't implement the new method.
type exportReaderComposite struct {
	*postgres.SellSheetStore
	*postgres.PurchaseStore
	*postgres.CampaignStore
}

// campaignsInitResult holds all values returned by initializeCampaignsService.
type campaignsInitResult struct {
	service          inventory.Service
	campaignStore    *postgres.CampaignStore
	purchaseStore    *postgres.PurchaseStore
	saleStore        *postgres.SaleStore
	analyticsStore   *postgres.AnalyticsStore
	financeStore     *postgres.FinanceStore
	pricingStore     *postgres.PricingStore
	dhStore          *postgres.DHStore
	sellSheetStore   *postgres.SellSheetStore
	certLookup       inventory.CertLookup
	certEnrichJob    *scheduler.CertEnrichJob    // nil if PSA not configured
	pricingEnrichJob *scheduler.PricingEnrichJob // pricers are attached later once CL/MM schedulers exist
	mmSalesStore     *postgres.MMSalesStore
	dhCompStore      *postgres.DHCompCacheStore
	arbSvc           arbitrage.Service
	portSvc          portfolio.Service
	tuningSvc        tuning.Service
	financeService   finance.Service
	exportService    export.Service
}

// initializeCampaignsService creates the campaigns service with all options
// wired, including price lookup and PSA cert lookup. It also creates the
// arbitrage, portfolio, and tuning services that delegate to the same
// repositories.
func initializeCampaignsService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *postgres.DB,
	priceProvImpl pricing.PriceProvider,
	intelRepo *postgres.MarketIntelligenceRepository,
	mmStore *postgres.MarketMoversStore,
	dhClient *dh.Client,
	eventRecorder dhevents.Recorder,
) campaignsInitResult {
	// Create individual stores instead of composite repository
	campaignStore := postgres.NewCampaignStore(db.DB, logger)
	purchaseStore := postgres.NewPurchaseStore(db.DB, logger)
	saleStore := postgres.NewSaleStore(db.DB, logger)
	analyticsStore := postgres.NewAnalyticsStore(db.DB, logger)
	financeStore := postgres.NewFinanceStore(db.DB, logger)
	pricingStore := postgres.NewPricingStore(db.DB, logger)
	dhStore := postgres.NewDHStore(db.DB, logger)
	sellSheetStore := postgres.NewSellSheetStore(db.DB, logger)

	priceLookupAdapter := lookup.NewAdapter(priceProvImpl)
	campaignOpts := []inventory.ServiceOption{
		inventory.WithPriceLookup(priceLookupAdapter),
		inventory.WithIDGenerator(uuid.NewString),
		inventory.WithMaxSnapshotRetries(cfg.SnapshotEnrich.MaxRetries),
	}

	// PSA cert lookup (optional)
	var certLookup inventory.CertLookup
	var certEnrichJobForSvc *scheduler.CertEnrichJob
	if cfg.Adapters.PSAToken != "" {
		psaClient := psa.NewClient(cfg.Adapters.PSAToken, logger)
		certAdapter := psa.NewCertAdapter(psaClient)
		certLookup = certAdapter
		campaignOpts = append(campaignOpts, inventory.WithCertLookup(certAdapter))
		// CertEnrichJob must be created before NewService so it can be injected via
		// WithCertEnrichEnqueuer. It will also be registered with the scheduler group below.
		certEnrichJobForSvc = scheduler.NewCertEnrichJob(certAdapter, purchaseStore, logger)
		campaignOpts = append(campaignOpts, inventory.WithCertEnrichEnqueuer(certEnrichJobForSvc))
		logger.Info(ctx, "PSA cert lookup and cert enrichment enabled")

		if cfg.Maintenance.BackfillImages {
			enqueueImageBackfill(ctx, purchaseStore, certEnrichJobForSvc, logger)
		}
	}

	if intelRepo != nil {
		campaignOpts = append(campaignOpts, inventory.WithIntelligenceRepo(intelRepo))
	}

	// Comp analytics — composite provider: CL → MM → DH fallback chain.
	clSalesStore := postgres.NewCLSalesStore(db.DB)
	mmSalesStore := postgres.NewMMSalesStore(db.DB)
	dhCompStore := postgres.NewDHCompCacheStore(db.DB)
	compositeComp := inventory.NewCompositeCompProvider(clSalesStore, mmSalesStore, dhCompStore)
	campaignOpts = append(campaignOpts, inventory.WithCompSummaryProvider(compositeComp))

	// Structured logger — required so phase-timing diagnostic logs in
	// GetInventoryAging / GetGlobalInventoryAging actually emit (guarded by
	// `if s.logger != nil`). Without this, we're blind to where inventory
	// page time goes.
	campaignOpts = append(campaignOpts, inventory.WithLogger(logger))

	// Market Movers search title enrichment for CSV export (optional).
	if mmStore != nil {
		mmAdapter := inventory.MMMappingFunc(func(ctx context.Context) (map[string]string, error) {
			mappings, err := mmStore.ListMappings(ctx)
			if err != nil {
				return nil, err
			}
			result := make(map[string]string, len(mappings))
			for _, m := range mappings {
				if m.SearchTitle != "" {
					result[m.SlabSerial] = m.SearchTitle
				}
			}
			return result, nil
		})
		campaignOpts = append(campaignOpts, inventory.WithMMMappings(mmAdapter))
	}

	// DH event recorder — records dh_state_events for enrollment and card-id-resolution.
	if eventRecorder != nil {
		campaignOpts = append(campaignOpts, inventory.WithEventRecorder(eventRecorder))
	}

	// DH sold notifier — retires items on DH when a sale is recorded locally.
	if dhClient != nil && dhClient.EnterpriseAvailable() {
		campaignOpts = append(campaignOpts,
			inventory.WithDHSoldNotifier(dhlistingadapter.NewInventoryAdapter(dhClient)),
		)
	}

	// DH cert → card_id resolver. Feeds batchResolveCardIDs in the inventory
	// service after PSA/CL imports so dh_card_id gets persisted.
	if dhClient != nil && dhClient.EnterpriseAvailable() {
		campaignOpts = append(campaignOpts,
			inventory.WithCardIDResolver(newDHCardIDResolverAdapter(dhClient, logger)),
		)
	}

	// Pricing enrichment job — on-demand CL+MM pricing triggered by intake.
	// Pricers are attached later in initializeSchedulers once the CL/MM
	// schedulers exist; until then the enqueuer is a no-op so injecting it
	// here is safe even when CL/MM are disabled.
	pricingEnrichJob := scheduler.NewPricingEnrichJob(purchaseStore, logger)
	campaignOpts = append(campaignOpts, inventory.WithPricingEnqueuer(pricingEnrichJob))

	campaignsService := inventory.NewService(
		campaignStore,  // CampaignRepository
		purchaseStore,  // PurchaseRepository
		saleStore,      // SaleRepository
		analyticsStore, // AnalyticsRepository
		financeStore,   // FinanceRepository
		pricingStore,   // PricingRepository
		dhStore,        // DHRepository
		campaignOpts...,
	)

	arbSvc := arbitrage.NewService(
		campaignStore,  // CampaignRepository
		purchaseStore,  // PurchaseRepository
		analyticsStore, // AnalyticsRepository
		financeStore,   // FinanceRepository
		arbitrage.WithPriceLookup(priceLookupAdapter),
		arbitrage.WithLogger(logger),
		arbitrage.WithProjectionCache(5*time.Minute),
	)

	portSvc := portfolio.NewService(
		campaignStore,  // CampaignRepository
		analyticsStore, // AnalyticsRepository
		financeStore,   // FinanceRepository
		logger,
	)

	tuningSvc := tuning.NewService(
		campaignStore,  // CampaignRepository
		analyticsStore, // AnalyticsRepository
		logger,
	)

	// Create export service
	var exportSvc export.Service
	exportOpts := []export.Option{
		export.WithLogger(logger),
	}
	// Create a minimal composite wrapper to satisfy ExportReader interface
	exportReader := &exportReaderComposite{
		SellSheetStore: sellSheetStore,
		PurchaseStore:  purchaseStore,
		CampaignStore:  campaignStore,
	}
	exportSvc = export.New(exportReader, exportOpts...)

	// Create finance service
	financeSvc := finance.New(financeStore, uuid.NewString)

	return campaignsInitResult{
		service:          campaignsService,
		campaignStore:    campaignStore,
		purchaseStore:    purchaseStore,
		saleStore:        saleStore,
		analyticsStore:   analyticsStore,
		financeStore:     financeStore,
		pricingStore:     pricingStore,
		dhStore:          dhStore,
		sellSheetStore:   sellSheetStore,
		certLookup:       certLookup,
		certEnrichJob:    certEnrichJobForSvc,
		pricingEnrichJob: pricingEnrichJob,
		mmSalesStore:     mmSalesStore,
		dhCompStore:      dhCompStore,
		arbSvc:           arbSvc,
		portSvc:          portSvc,
		tuningSvc:        tuningSvc,
		financeService:   financeSvc,
		exportService:    exportSvc,
	}
}

// enqueueImageBackfill re-enqueues unsold PSA purchases with empty image URLs
// onto the cert-enrichment queue so the async worker fills them from PSA.
// Safe to run repeatedly: rows that already have images will skip the PSA call
// inside enrichImages.
func enqueueImageBackfill(ctx context.Context, repo inventory.PurchaseRepository, enq *scheduler.CertEnrichJob, logger observability.Logger) {
	unsold, err := repo.ListAllUnsoldPurchases(ctx)
	if err != nil {
		logger.Warn(ctx, "image backfill: list unsold purchases failed", observability.Err(err))
		return
	}
	var enqueued int
	for _, p := range unsold {
		if p.Grader != "PSA" || p.CertNumber == "" {
			continue
		}
		if p.FrontImageURL != "" || p.BackImageURL != "" {
			continue
		}
		enq.Enqueue(p.CertNumber)
		enqueued++
	}
	logger.Info(ctx, "image backfill: enqueued unsold PSA certs with missing images",
		observability.Int("count", enqueued))
}
