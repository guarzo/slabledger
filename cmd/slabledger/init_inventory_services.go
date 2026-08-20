package main

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	dhlistingadapter "github.com/guarzo/slabledger/internal/adapters/clients/dhlisting"
	"github.com/guarzo/slabledger/internal/adapters/clients/dhprice"
	"github.com/guarzo/slabledger/internal/adapters/clients/psa"
	"github.com/guarzo/slabledger/internal/adapters/clients/psaportal"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/csvimport"
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

// exportReaderComposite satisfies export.ExportReader by composing the
// required stores. Named explicitly so that adding methods to export.ExportReader
// produces a compile error here if any store doesn't implement the new method.
type exportReaderComposite struct {
	*postgres.PurchaseStore
	*postgres.CampaignStore
}

// campaignsInitResult holds all values returned by initializeCampaignsService.
type campaignsInitResult struct {
	service          inventory.Service
	importService    csvimport.Service
	campaignStore    *postgres.CampaignStore
	purchaseStore    *postgres.PurchaseStore
	saleStore        *postgres.SaleStore
	analyticsStore   *postgres.AnalyticsStore
	financeStore     *postgres.FinanceStore
	pricingStore     *postgres.PricingStore
	dhStore          *postgres.DHStore
	pendingItemsRepo *postgres.PendingItemsRepository
	certLookup       inventory.CertLookup
	certEnrichJob    *scheduler.CertEnrichJob    // nil if PSA not configured
	pricingEnrichJob *scheduler.PricingEnrichJob // pricers are attached later once CL schedulers exist
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
	dhClient *dh.Client,
	eventRecorder dhevents.Recorder,
	cardIDMappingRepo *postgres.CardIDMappingRepository,
) campaignsInitResult {
	// Create individual stores instead of composite repository
	campaignStore := postgres.NewCampaignStore(db.DB, logger)
	purchaseStore := postgres.NewPurchaseStore(db.DB, logger)
	saleStore := postgres.NewSaleStore(db.DB, logger)
	analyticsStore := postgres.NewAnalyticsStore(db.DB, logger)
	financeStore := postgres.NewFinanceStore(db.DB, logger)
	pricingStore := postgres.NewPricingStore(db.DB, logger)
	dhStore := postgres.NewDHStore(db.DB, logger)
	pendingItemsRepo := postgres.NewPendingItemsRepository(db.DB)

	priceLookupAdapter := lookup.NewAdapter(priceProvImpl)
	campaignOpts := []inventory.ServiceOption{
		inventory.WithPriceLookup(priceLookupAdapter),
		inventory.WithPendingItemRepository(pendingItemsRepo),
		inventory.WithIDGenerator(uuid.NewString),
		inventory.WithMaxSnapshotRetries(cfg.SnapshotEnrich.MaxRetries),
	}

	// PSA cert lookup (optional)
	var certLookup inventory.CertLookup
	var certEnrichJobForSvc *scheduler.CertEnrichJob
	// Held as an interface as well, so a nil job stays a nil interface when it is
	// passed to csvimport.Deps rather than a typed-nil that defeats its nil checks.
	var certEnrichQueue inventory.CertEnrichEnqueuer
	if cfg.Adapters.PSAToken != "" {
		psaClient := psa.NewClient(cfg.Adapters.PSAToken, logger)
		certAdapter := psa.NewCertAdapter(psaClient)
		certLookup = certAdapter
		campaignOpts = append(campaignOpts, inventory.WithCertLookup(certAdapter))
		// CertEnrichJob must be created before NewService so it can be injected via
		// WithCertEnrichEnqueuer. It will also be registered with the scheduler group below.
		certEnrichJobForSvc = scheduler.NewCertEnrichJob(certAdapter, purchaseStore, logger)
		certEnrichQueue = certEnrichJobForSvc
		campaignOpts = append(campaignOpts, inventory.WithCertEnrichEnqueuer(certEnrichJobForSvc))
		logger.Info(ctx, "PSA cert lookup and cert enrichment enabled")

		if cfg.Maintenance.BackfillImages {
			enqueueImageBackfill(ctx, purchaseStore, certEnrichJobForSvc, logger)
		}
	}

	if intelRepo != nil {
		campaignOpts = append(campaignOpts, inventory.WithIntelligenceRepo(intelRepo))
	}

	// Comp analytics — composite provider: CL → DH fallback chain.
	clSalesStore := postgres.NewCLSalesStore(db.DB)
	dhCompStore := postgres.NewDHCompCacheStore(db.DB)
	compositeComp := inventory.NewCompositeCompProvider(clSalesStore, dhCompStore)
	campaignOpts = append(campaignOpts, inventory.WithCompSummaryProvider(compositeComp))

	// Structured logger — required so phase-timing diagnostic logs in
	// GetInventoryAging / GetGlobalInventoryAging actually emit (guarded by
	// `if s.logger != nil`). Without this, we're blind to where inventory
	// page time goes.
	campaignOpts = append(campaignOpts, inventory.WithLogger(logger))

	// DH event recorder — records dh_state_events for enrollment and card-id-resolution.
	if eventRecorder != nil {
		campaignOpts = append(campaignOpts, inventory.WithEventRecorder(eventRecorder))
	}

	// DH sale recorder — records (and, on un-sell, voids) sales on DH via the
	// purpose-built sale endpoint.
	if dhClient != nil && dhClient.EnterpriseAvailable() {
		campaignOpts = append(campaignOpts, inventory.WithDHSaleRecorder(dhlistingadapter.NewInventoryAdapter(dhClient).WithLogger(logger)))
	}

	// DH cert → card_id resolver. Feeds batchResolveCardIDs in the inventory
	// service after PSA/CL imports so dh_card_id gets persisted.
	var cardIDResolver inventory.CardIDResolver
	if dhClient != nil && dhClient.EnterpriseAvailable() {
		cardIDResolver = newDHCardIDResolverAdapter(dhClient, logger)
		campaignOpts = append(campaignOpts, inventory.WithCardIDResolver(cardIDResolver))
	}

	// Pricing enrichment job — on-demand CL pricing triggered by intake.
	// Pricers are attached later in initializeSchedulers once the CL
	// scheduler exists; until then the enqueuer is a no-op so injecting it
	// here is safe even when CL is disabled.
	pricingEnrichJob := scheduler.NewPricingEnrichJob(purchaseStore, logger)
	campaignOpts = append(campaignOpts, inventory.WithPricingEnqueuer(pricingEnrichJob))

	// PSA campaign resolver — makes PSA's own attribution authoritative on the
	// import path (service_import_psa.go), which is what both the PSA sync
	// scheduler and manual CSV upload run through. Constructed exactly as
	// cmd/psa-harvest/main.go does. Wired unconditionally: when the campaign
	// snapshot is missing or stale the resolver returns an error and the import
	// falls back to inference, which is the pre-existing behavior.
	psaResolver := psaportal.NewCampaignResolver(postgres.NewPSACampaignSnapshotStore(db.DB), campaignStore, nil)
	campaignOpts = append(campaignOpts, inventory.WithPSACampaignResolver(psaResolver))

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

	// CSV/portal intake. It writes through the same repositories and reaches back
	// into the inventory service for the parts of intake inventory owns (purchase
	// creation, market snapshots, DH events, card-ID backfill).
	importService := csvimport.NewService(csvimport.Deps{
		Campaigns:       campaignStore,
		Purchases:       purchaseStore,
		Sales:           saleStore,
		Finance:         financeStore,
		PendingItems:    pendingItemsRepo,
		Inventory:       campaignsService,
		PriceLookup:     priceLookupAdapter,
		CardIDResolver:  cardIDResolver,
		CertEnrichQueue: certEnrichQueue,
		// pricingEnrichJob (constructed above, ~line 157) is unconditional and
		// never nil, unlike certEnrichQueue — it has no PSA-token gate, so there
		// is no typed-nil hazard here and no interface-holder variable is needed
		// to guard against one (contrast the certEnrichQueue comment above it).
		PricingQueue: pricingEnrichJob,
		PSAResolver:  psaResolver,
		IDGen:        uuid.NewString,
		Logger:       logger,
	})

	arbOpts := []arbitrage.ServiceOption{
		arbitrage.WithPriceLookup(priceLookupAdapter),
		arbitrage.WithLogger(logger),
		arbitrage.WithProjectionCache(5 * time.Minute),
	}
	if dhClient != nil && dhClient.EnterpriseAvailable() && cardIDMappingRepo != nil {
		arbOpts = append(arbOpts,
			arbitrage.WithBatchPricer(dhprice.NewBatchAdapter(dhClient, cardIDMappingRepo)),
		)
	}

	arbSvc := arbitrage.NewService(
		campaignStore,  // CampaignRepository
		purchaseStore,  // PurchaseRepository
		analyticsStore, // AnalyticsRepository
		financeStore,   // FinanceRepository
		arbOpts...,
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
		PurchaseStore: purchaseStore,
		CampaignStore: campaignStore,
	}
	exportSvc = export.New(exportReader, exportOpts...)

	// Create finance service
	financeSvc := finance.New(financeStore, uuid.NewString)

	return campaignsInitResult{
		service:          campaignsService,
		importService:    importService,
		campaignStore:    campaignStore,
		purchaseStore:    purchaseStore,
		saleStore:        saleStore,
		analyticsStore:   analyticsStore,
		financeStore:     financeStore,
		pricingStore:     pricingStore,
		dhStore:          dhStore,
		pendingItemsRepo: pendingItemsRepo,
		certLookup:       certLookup,
		certEnrichJob:    certEnrichJobForSvc,
		pricingEnrichJob: pricingEnrichJob,
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
// Safe to run repeatedly for correctness: rows that already have images skip
// the PSA call inside enrichImages. It is enqueued images-only — these rows
// already carry their card metadata, so a cert lookup here would double the
// PSA calls this sweep costs for nothing (SLA-108).
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
		enq.EnqueueImagesOnly(p.CertNumber)
		enqueued++
	}
	logger.Info(ctx, "image backfill: enqueued unsold PSA certs with missing images",
		observability.Int("count", enqueued))
}
