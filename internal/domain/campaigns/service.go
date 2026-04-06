package campaigns

import (
	"context"
	"errors"
	"sync"

	"github.com/guarzo/slabledger/internal/domain/intelligence"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ErrCertLookupNotConfigured is returned when cert lookup is requested but no CertLookup was injected.
var ErrCertLookupNotConfigured = errors.New("cert lookup not configured")

// CardIdentity bundles the fields needed to look up a card's market price.
type CardIdentity struct {
	CardName        string
	CardNumber      string
	SetName         string
	PSAListingTitle string // Raw PSA listing title; populated from cert-lookup flows or Purchase.ToCardIdentity()
}

// PriceLookup provides market price data for signal computation and snapshots.
type PriceLookup interface {
	GetLastSoldCents(ctx context.Context, card CardIdentity, grade float64) (int, error)
	GetMarketSnapshot(ctx context.Context, card CardIdentity, grade float64) (*MarketSnapshot, error)
}

// MarketSnapshot captures a point-in-time view of market data for a card at a specific grade.
type MarketSnapshot struct {
	LastSoldCents     int     `json:"lastSoldCents"`
	LastSoldDate      string  `json:"lastSoldDate,omitempty"`
	SaleCount         int     `json:"saleCount,omitempty"`
	GradePriceCents   int     `json:"gradePriceCents"`
	LowestListCents   int     `json:"lowestListCents,omitempty"`
	ActiveListings    int     `json:"activeListings,omitempty"`
	SalesLast30d      int     `json:"salesLast30d,omitempty"`
	SalesLast90d      int     `json:"salesLast90d,omitempty"`
	ConservativeCents int     `json:"conservativeCents,omitempty"` // P25
	MedianCents       int     `json:"medianCents,omitempty"`       // P50
	OptimisticCents   int     `json:"optimisticCents,omitempty"`   // P75
	Trend30d          float64 `json:"trend30d,omitempty"`
	Trend90d          float64 `json:"trend90d,omitempty"`
	Volatility        float64 `json:"volatility,omitempty"`

	// Extended percentiles
	P10Cents       int `json:"p10Cents,omitempty"`
	P90Cents       int `json:"p90Cents,omitempty"`
	DistSampleSize int `json:"distSampleSize,omitempty"` // Number of sales in distribution
	DistPeriodDays int `json:"distPeriodDays,omitempty"` // Lookback window in days

	// Sales velocity
	DailyVelocity   float64 `json:"dailyVelocity,omitempty"`
	WeeklyVelocity  float64 `json:"weeklyVelocity,omitempty"`
	MonthlyVelocity int     `json:"monthlyVelocity,omitempty"`

	// Short-term signal
	Avg7DayCents int `json:"avg7DayCents,omitempty"`

	// Fusion metadata
	SourceCount      int     `json:"sourceCount,omitempty"`
	FusionConfidence float64 `json:"fusionConfidence,omitempty"`

	// Per-source pricing data
	SourcePrices []SourcePrice `json:"sourcePrices,omitempty"`

	// Estimated value (from a secondary source) — kept separate from LastSoldCents
	// so actual sale data is never overwritten by model estimates.
	EstimatedValueCents int    `json:"estimatedValueCents,omitempty"`
	EstimateSource      string `json:"estimateSource,omitempty"`

	// IsEstimated indicates the grade price was derived from an adjacent grade (not direct data).
	IsEstimated bool `json:"isEstimated,omitempty"`
	// PricingGap indicates all core price fields are zero despite having pricing data.
	PricingGap bool `json:"pricingGap,omitempty"`

	// CL reference data (for snapshot accuracy validation)
	CLValueCents    int     `json:"clValueCents,omitempty"`    // CL value at time of purchase
	CLDeviationPct  float64 `json:"clDeviationPct,omitempty"`  // abs(median - cl) / cl before any correction
	CLAnchorApplied bool    `json:"clAnchorApplied,omitempty"` // true if CL was used to correct unreliable snapshot
}

// SourcePrice contains pricing data from a single data source.
type SourcePrice struct {
	Source       string  `json:"source"`                 // e.g. "PriceCharting", "eBay"
	PriceCents   int     `json:"priceCents"`             // This source's price for the grade
	SaleCount    int     `json:"saleCount,omitempty"`    // Number of sales this is based on
	Trend        string  `json:"trend,omitempty"`        // "up", "down", "stable"
	Confidence   string  `json:"confidence,omitempty"`   // "high", "medium", "low"
	MinCents     int     `json:"minCents,omitempty"`     // Range low
	MaxCents     int     `json:"maxCents,omitempty"`     // Range high
	Avg7DayCents int     `json:"avg7DayCents,omitempty"` // 7-day rolling average
	Volume7Day   float64 `json:"volume7Day,omitempty"`   // 7-day daily volume
}

// CardIDResolver resolves cert numbers to external card IDs in batch.
type CardIDResolver interface {
	ResolveCardIDsByCerts(ctx context.Context, certs []string, grader string) (map[string]string, error)
}

// CertLookup resolves PSA certificate numbers to card details.
type CertLookup interface {
	LookupCert(ctx context.Context, certNumber string) (*CertInfo, error)
}

// CertInfo contains card details resolved from a PSA certificate number.
type CertInfo struct {
	CertNumber string  `json:"certNumber"`
	CardName   string  `json:"cardName"`
	Grade      float64 `json:"grade"`
	Year       string  `json:"year"`
	Brand      string  `json:"brand"`
	Category   string  `json:"category,omitempty"` // PSA set/category (e.g., "CELEBRATIONS")
	Subject    string  `json:"subject"`
	Variety    string  `json:"variety,omitempty"`
	CardNumber string  `json:"cardNumber,omitempty"`
	Population int     `json:"population"`
	PopHigher  int     `json:"popHigher"`
}

// Service defines the business logic for campaign operations.
type Service interface {
	CreateCampaign(ctx context.Context, c *Campaign) error
	GetCampaign(ctx context.Context, id string) (*Campaign, error)
	ListCampaigns(ctx context.Context, activeOnly bool) ([]Campaign, error)
	UpdateCampaign(ctx context.Context, c *Campaign) error
	DeleteCampaign(ctx context.Context, id string) error

	CreatePurchase(ctx context.Context, p *Purchase) error
	GetPurchase(ctx context.Context, id string) (*Purchase, error)
	DeletePurchase(ctx context.Context, id string) error
	ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Purchase, error)

	CreateSale(ctx context.Context, s *Sale, campaign *Campaign, purchase *Purchase) error
	CreateBulkSales(ctx context.Context, campaignID string, channel SaleChannel, saleDate string, items []BulkSaleInput) (*BulkSaleResult, error)
	ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Sale, error)
	DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error

	// Global (cross-campaign) operations
	RefreshCLValuesGlobal(ctx context.Context, rows []CLExportRow) (*GlobalCLRefreshResult, error)
	ImportCLExportGlobal(ctx context.Context, rows []CLExportRow) (*GlobalImportResult, error)
	ImportPSAExportGlobal(ctx context.Context, rows []PSAExportRow) (*PSAImportResult, error)
	ExportCLFormatGlobal(ctx context.Context, missingCLOnly bool) ([]CLExportEntry, error)
	ReassignPurchase(ctx context.Context, purchaseID string, newCampaignID string) error

	// External purchases
	EnsureExternalCampaign(ctx context.Context) (*Campaign, error)
	ImportExternalCSV(ctx context.Context, rows []ShopifyExportRow) (*ExternalImportResult, error)

	// Orders sales import
	ImportOrdersSales(ctx context.Context, rows []OrdersExportRow) (*OrdersImportResult, error)
	ConfirmOrdersSales(ctx context.Context, items []OrdersConfirmItem) (*BulkSaleResult, error)

	// Credit & Invoice management
	GetCreditSummary(ctx context.Context) (*CreditSummary, error)
	GetCashflowConfig(ctx context.Context) (*CashflowConfig, error)
	UpdateCashflowConfig(ctx context.Context, cfg *CashflowConfig) error
	ListInvoices(ctx context.Context) ([]Invoice, error)
	UpdateInvoice(ctx context.Context, inv *Invoice) error

	// Portfolio health
	GetPortfolioHealth(ctx context.Context) (*PortfolioHealth, error)
	GetPortfolioChannelVelocity(ctx context.Context) ([]ChannelVelocity, error)

	// Cert lookup
	LookupCert(ctx context.Context, certNumber string) (*CertInfo, *MarketSnapshot, error)
	QuickAddPurchase(ctx context.Context, campaignID string, req QuickAddRequest) (*Purchase, error)

	// Analytics
	GetCampaignPNL(ctx context.Context, campaignID string) (*CampaignPNL, error)
	GetPNLByChannel(ctx context.Context, campaignID string) ([]ChannelPNL, error)
	GetDailySpend(ctx context.Context, campaignID string, days int) ([]DailySpend, error)
	GetDaysToSellDistribution(ctx context.Context, campaignID string) ([]DaysToSellBucket, error)
	GetInventoryAging(ctx context.Context, campaignID string) ([]AgingItem, error)
	GetGlobalInventoryAging(ctx context.Context) ([]AgingItem, error)
	GetFlaggedInventory(ctx context.Context) ([]AgingItem, error)

	// Sell sheet
	GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*SellSheet, error)
	GenerateGlobalSellSheet(ctx context.Context) (*SellSheet, error)
	GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*SellSheet, error)

	// Tuning
	GetCampaignTuning(ctx context.Context, campaignID string) (*TuningResponse, error)

	// Portfolio insights & suggestions
	GetPortfolioInsights(ctx context.Context) (*PortfolioInsights, error)
	GetCampaignSuggestions(ctx context.Context) (*SuggestionsResponse, error)

	// Revocation
	FlagForRevocation(ctx context.Context, segmentLabel, segmentDimension, reason string) (*RevocationFlag, error)
	ListRevocationFlags(ctx context.Context) ([]RevocationFlag, error)
	GenerateRevocationEmail(ctx context.Context, flagID string) (string, error)

	// Capital timeline
	GetCapitalTimeline(ctx context.Context) (*CapitalTimeline, error)

	// Weekly review
	GetWeeklyReviewSummary(ctx context.Context) (*WeeklyReviewSummary, error)

	// Crack arbitrage
	GetCrackCandidates(ctx context.Context, campaignID string) ([]CrackAnalysis, error)
	GetCrackOpportunities(ctx context.Context) ([]CrackAnalysis, error)

	// Acquisition arbitrage
	GetAcquisitionTargets(ctx context.Context) ([]AcquisitionOpportunity, error)

	// Expected value
	GetExpectedValues(ctx context.Context, campaignID string) (*EVPortfolio, error)
	EvaluatePurchase(ctx context.Context, campaignID string, cardName string, grade float64, buyCostCents int) (*ExpectedValue, error)

	// Activation checklist
	GetActivationChecklist(ctx context.Context, campaignID string) (*ActivationChecklist, error)

	// Monte Carlo projection
	RunProjection(ctx context.Context, campaignID string) (*MonteCarloComparison, error)

	// Buy cost correction
	UpdateBuyCost(ctx context.Context, purchaseID string, buyCostCents int) error

	// Price overrides & AI suggestions
	SetPriceOverride(ctx context.Context, purchaseID string, priceCents int, source string) error
	SetAISuggestedPrice(ctx context.Context, purchaseID string, priceCents int) error
	AcceptAISuggestion(ctx context.Context, purchaseID string) error
	DismissAISuggestion(ctx context.Context, purchaseID string) error
	GetPriceOverrideStats(ctx context.Context) (*PriceOverrideStats, error)

	// Price review
	SetReviewedPrice(ctx context.Context, purchaseID string, priceCents int, source string) error
	GetReviewStats(ctx context.Context, campaignID string) (ReviewStats, error)
	GetGlobalReviewStats(ctx context.Context) (ReviewStats, error)

	// Price flags
	CreatePriceFlag(ctx context.Context, purchaseID string, userID int64, reason string) (int64, error)
	ListPriceFlags(ctx context.Context, status string) ([]PriceFlagWithContext, error)
	ResolvePriceFlag(ctx context.Context, flagID int64, resolvedBy int64) error

	// Cert entry
	ImportCerts(ctx context.Context, certNumbers []string) (*CertImportResult, error)
	GetPurchasesByCertNumbers(ctx context.Context, certNumbers []string) (map[string]*Purchase, error)
	ScanCert(ctx context.Context, certNumber string) (*ScanCertResult, error)
	ResolveCert(ctx context.Context, certNumber string) (*CertInfo, error)

	// eBay export
	ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*EbayExportListResponse, error)
	GenerateEbayCSV(ctx context.Context, items []EbayExportGenerateItem) ([]byte, error)

	// Shopify price sync
	MatchShopifyPrices(ctx context.Context, items []ShopifyPriceSyncItem) (*ShopifyPriceSyncResponse, error)

	// Snapshot refresh (used by background scheduler)
	RefreshPurchaseSnapshot(ctx context.Context, purchaseID string, card CardIdentity, grade float64, clValueCents int) bool
	ProcessPendingSnapshots(ctx context.Context, limit int) (processed, skipped, failed int)
	RetryFailedSnapshots(ctx context.Context, limit int) (processed, skipped, failed int)

	// Close shuts down background workers.
	Close()
}

type service struct {
	repo               Repository
	priceProv          PriceLookup
	certLookup         CertLookup
	cardIDResolver     CardIDResolver
	logger             observability.Logger
	baseCtx            context.Context // lifecycle context for background workers
	idGen              func() string   // generates unique IDs; must be injected via WithIDGenerator
	maxSnapshotRetries int             // max retry attempts for failed snapshots (0 = unlimited)

	// History recorders (optional — best-effort logging, never block imports)
	popRecorder PopulationHistoryRecorder
	clRecorder  CLValueHistoryRecorder

	intelRepo intelligence.Repository // optional — DH market intelligence for price-sync enrichment

	// certEnrichCh is a bounded channel for cert enrichment requests.
	// A single background worker processes cert numbers sequentially,
	// respecting PSA API rate limits (100/day).
	certEnrichCh     chan string
	certEnrichCancel context.CancelFunc
	wg               sync.WaitGroup // tracks background goroutines (e.g. batchResolveCardIDs)

	// Crack candidate cache — refreshed in background, read by inventory endpoint.
	crackCacheMu     sync.RWMutex
	crackCacheSet    map[string]bool
	crackCacheCancel context.CancelFunc
}

// ServiceOption configures optional service dependencies.
type ServiceOption func(*service)

// WithPriceLookup enables market signal computation on inventory aging.
func WithPriceLookup(pl PriceLookup) ServiceOption {
	return func(s *service) { s.priceProv = pl }
}

// WithCertLookup enables PSA cert number resolution.
func WithCertLookup(cl CertLookup) ServiceOption {
	return func(s *service) { s.certLookup = cl }
}

// WithCardIDResolver enables batch cert→card_id resolution after imports.
func WithCardIDResolver(r CardIDResolver) ServiceOption {
	return func(s *service) { s.cardIDResolver = r }
}

// WithLogger enables structured logging for the campaigns service.
func WithLogger(l observability.Logger) ServiceOption {
	return func(s *service) { s.logger = l }
}

// WithBaseContext sets the lifecycle context for background workers.
// If not set, context.Background() is used.
func WithBaseContext(ctx context.Context) ServiceOption {
	return func(s *service) { s.baseCtx = ctx }
}

// WithMaxSnapshotRetries sets the maximum number of retry attempts for failed
// snapshot enrichment. After this many failures, status moves to "exhausted".
// A value of 0 means unlimited retries (not recommended).
func WithMaxSnapshotRetries(n int) ServiceOption {
	return func(s *service) { s.maxSnapshotRetries = n }
}

// WithIDGenerator sets the ID generator for creating unique entity IDs.
// Must be provided by the composition root (e.g., uuid.NewString).
func WithIDGenerator(fn func() string) ServiceOption {
	return func(s *service) { s.idGen = fn }
}

// WithPopulationRecorder enables population history tracking during CSV imports.
func WithPopulationRecorder(r PopulationHistoryRecorder) ServiceOption {
	return func(s *service) { s.popRecorder = r }
}

// WithCLValueRecorder enables CL value history tracking during CSV imports.
func WithCLValueRecorder(r CLValueHistoryRecorder) ServiceOption {
	return func(s *service) { s.clRecorder = r }
}

// WithIntelligenceRepo enables DH market intelligence enrichment in price-sync.
func WithIntelligenceRepo(r intelligence.Repository) ServiceOption {
	return func(s *service) { s.intelRepo = r }
}

func NewService(repo Repository, opts ...ServiceOption) Service {
	s := &service{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	if s.idGen == nil {
		panic("campaigns.NewService: WithIDGenerator is required")
	}
	if s.baseCtx == nil {
		s.baseCtx = context.Background()
	}

	// Start a bounded cert enrichment worker if cert lookup is configured.
	// The channel buffer (200) limits how many cert numbers can be queued;
	// excess submissions are dropped with a warning log.
	if s.certLookup != nil {
		s.certEnrichCh = make(chan string, 200)
		ctx, cancel := context.WithCancel(s.baseCtx)
		s.certEnrichCancel = cancel
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.certEnrichWorker(ctx)
		}()
	}

	// Start background crack candidate cache refresher if price lookup is configured.
	// The inventory endpoint reads from this cache instead of computing live,
	// avoiding hundreds of sequential external API calls per request.
	if s.priceProv != nil {
		ctx, cancel := context.WithCancel(s.baseCtx)
		s.crackCacheCancel = cancel
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.crackCacheWorker(ctx)
		}()
	}

	return s
}

// Close shuts down background workers. Safe to call multiple times.
func (s *service) Close() {
	if s.certEnrichCancel != nil {
		s.certEnrichCancel()
	}
	if s.crackCacheCancel != nil {
		s.crackCacheCancel()
	}
	s.wg.Wait()
}

var _ Service = (*service)(nil)
