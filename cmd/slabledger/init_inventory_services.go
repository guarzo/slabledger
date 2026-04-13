package main

import (
	"context"

	"github.com/google/uuid"

	"github.com/guarzo/slabledger/internal/adapters/clients/pricelookup"
	"github.com/guarzo/slabledger/internal/adapters/clients/psa"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/arbitrage"
	"github.com/guarzo/slabledger/internal/domain/export"
	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/portfolio"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/tuning"
	"github.com/guarzo/slabledger/internal/platform/config"
)

// exportReaderComposite satisfies export.ExportReader by composing the three
// required stores. Named explicitly so that adding methods to export.ExportReader
// produces a compile error here if any store doesn't implement the new method.
type exportReaderComposite struct {
	*sqlite.SellSheetStore
	*sqlite.PurchaseStore
	*sqlite.CampaignStore
}

// campaignsInitResult holds all values returned by initializeCampaignsService.
type campaignsInitResult struct {
	service        inventory.Service
	campaignStore  *sqlite.CampaignStore
	purchaseStore  *sqlite.PurchaseStore
	saleStore      *sqlite.SaleStore
	analyticsStore *sqlite.AnalyticsStore
	financeStore   *sqlite.FinanceStore
	pricingStore   *sqlite.PricingStore
	dhStore        *sqlite.DHStore
	snapshotStore  *sqlite.SnapshotStore
	sellSheetStore *sqlite.SellSheetStore
	certLookup     inventory.CertLookup
	certEnrichJob  *scheduler.CertEnrichJob // nil if PSA not configured
	arbSvc         arbitrage.Service
	portSvc        portfolio.Service
	tuningSvc      tuning.Service
	financeService finance.Service
	exportService  export.Service
}

// initializeCampaignsService creates the campaigns service with all options
// wired, including price lookup and PSA cert lookup. It also creates the
// arbitrage, portfolio, and tuning services that delegate to the same
// repositories.
func initializeCampaignsService(
	ctx context.Context,
	cfg *config.Config,
	logger observability.Logger,
	db *sqlite.DB,
	priceProvImpl pricing.PriceProvider,
	intelRepo *sqlite.MarketIntelligenceRepository,
	mmStore *sqlite.MarketMoversStore,
) campaignsInitResult {
	// Create individual stores instead of composite repository
	campaignStore := sqlite.NewCampaignStore(db.DB, logger)
	purchaseStore := sqlite.NewPurchaseStore(db.DB, logger)
	saleStore := sqlite.NewSaleStore(db.DB, logger)
	analyticsStore := sqlite.NewAnalyticsStore(db.DB, logger)
	financeStore := sqlite.NewFinanceStore(db.DB, logger)
	pricingStore := sqlite.NewPricingStore(db.DB, logger)
	dhStore := sqlite.NewDHStore(db.DB, logger)
	snapshotStore := sqlite.NewSnapshotStore(db.DB, logger)
	sellSheetStore := sqlite.NewSellSheetStore(db.DB, logger)

	priceLookupAdapter := pricelookup.NewAdapter(priceProvImpl)
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
	}

	// History recorders — track CL values and population changes during CSV imports.
	campaignOpts = append(campaignOpts,
		inventory.WithCLValueRecorder(snapshotStore),
		inventory.WithPopulationRecorder(snapshotStore),
	)

	if intelRepo != nil {
		campaignOpts = append(campaignOpts, inventory.WithIntelligenceRepo(intelRepo))
	}

	// Card Ladder comp analytics — CLSalesStore only needs *sql.DB (always available).
	clSalesStore := sqlite.NewCLSalesStore(db.DB)
	campaignOpts = append(campaignOpts, inventory.WithCompSummaryProvider(clSalesStore))

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

	campaignsService := inventory.NewService(
		campaignStore,  // CampaignRepository
		purchaseStore,  // PurchaseRepository
		saleStore,      // SaleRepository
		analyticsStore, // AnalyticsRepository
		financeStore,   // FinanceRepository
		pricingStore,   // PricingRepository
		dhStore,        // DHRepository
		snapshotStore,  // SnapshotRepository
		campaignOpts...,
	)

	arbSvc := arbitrage.NewService(
		campaignStore,  // CampaignRepository
		purchaseStore,  // PurchaseRepository
		analyticsStore, // AnalyticsRepository
		financeStore,   // FinanceRepository
		arbitrage.WithPriceLookup(priceLookupAdapter),
		arbitrage.WithLogger(logger),
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
	if intelRepo != nil {
		exportOpts = append(exportOpts, export.WithIntelligenceRepo(intelRepo))
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
		service:        campaignsService,
		campaignStore:  campaignStore,
		purchaseStore:  purchaseStore,
		saleStore:      saleStore,
		analyticsStore: analyticsStore,
		financeStore:   financeStore,
		pricingStore:   pricingStore,
		dhStore:        dhStore,
		snapshotStore:  snapshotStore,
		sellSheetStore: sellSheetStore,
		certLookup:     certLookup,
		certEnrichJob:  certEnrichJobForSvc,
		arbSvc:         arbSvc,
		portSvc:        portSvc,
		tuningSvc:      tuningSvc,
		financeService: financeSvc,
		exportService:  exportSvc,
	}
}
