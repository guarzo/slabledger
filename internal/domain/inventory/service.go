package inventory

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

	// Pricing metadata
	SourceCount int      `json:"sourceCount,omitempty"`
	Sources     []string `json:"sources,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`

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
	Source       string  `json:"source"`                 // e.g. "eBay", "Estimate"
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

// MMMappingProvider provides Market Movers search title mappings for export enrichment.
// Implementations typically wrap the MM card mapping store.
type MMMappingProvider interface {
	// ListMMSearchTitles returns a map of cert number → MM canonical SearchTitle
	// for all cards that have been resolved by the MM scheduler.
	ListMMSearchTitles(ctx context.Context) (map[string]string, error)
}

// MMMappingFunc is a functional adapter for MMMappingProvider (like http.HandlerFunc).
type MMMappingFunc func(ctx context.Context) (map[string]string, error)

// ListMMSearchTitles implements MMMappingProvider.
func (f MMMappingFunc) ListMMSearchTitles(ctx context.Context) (map[string]string, error) {
	return f(ctx)
}

// CertLookup resolves PSA certificate numbers to card details.
type CertLookup interface {
	LookupCert(ctx context.Context, certNumber string) (*CertInfo, error)
}

// CertEnrichEnqueuer enqueues certificate numbers for background enrichment.
type CertEnrichEnqueuer interface {
	Enqueue(certNumber string)
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

// Service is the full campaigns API — a composition of all sub-interfaces.
// Consumers that only need a subset should depend on the narrower interface
// (e.g. AnalyticsService, ImportService) to follow the Interface Segregation Principle.
// Sub-interfaces are defined in service_interfaces.go.
type Service interface {
	CRUDService
	AnalyticsService
	ImportService
	PricingService
	CertLookupService
	SnapshotService
	DHService

	// Close shuts down background workers.
	Close()
}

type service struct {
	campaigns CampaignRepository
	purchases PurchaseRepository
	sales     SaleRepository
	analytics AnalyticsRepository
	finance   FinanceRepository
	pricing   PricingRepository
	dh        DHRepository
	snapshots SnapshotRepository

	priceProv          PriceLookup
	certLookup         CertLookup
	cardIDResolver     CardIDResolver
	mmMappings         MMMappingProvider // optional — MM search title enrichment for export
	logger             observability.Logger
	idGen              func() string // generates unique IDs; must be injected via WithIDGenerator
	maxSnapshotRetries int           // max retry attempts for failed snapshots (0 = unlimited)

	// History recorders (optional — best-effort logging, never block imports)
	popRecorder PopulationHistoryRecorder
	clRecorder  CLValueHistoryRecorder

	compProv        CompSummaryProvider     // optional — Card Ladder comp analytics
	intelRepo       intelligence.Repository // optional — DH market intelligence for price-sync enrichment
	pendingItemRepo PendingItemRepository   // optional — stores ambiguous/unmatched items from imports

	// certEnrichQueue enqueues cert numbers for background enrichment (optional).
	// A scheduler job processes cert numbers sequentially, respecting PSA API rate limits (100/day).
	certEnrichQueue CertEnrichEnqueuer

	// disableBackgroundWorkers is a test-only flag to prevent background workers from running.
	// When true, the crack cache worker will not start even if priceProv is set.
	disableBackgroundWorkers bool

	// wg tracks background goroutines (e.g. batchResolveCardIDs, card ID resolver).
	// Note: cert enrichment worker is now managed by scheduler, not here.
	wg sync.WaitGroup

	// Crack candidate cache — refreshed in background, read by inventory and handler endpoints.
	crackCacheMu  sync.RWMutex
	crackCacheSet map[string]bool // purchaseID→true (derived from crackCacheAll)
	crackCacheAll []CrackAnalysis // full cross-campaign results
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

// WithMMMappings enables Market Movers search title enrichment on CSV export.
func WithMMMappings(m MMMappingProvider) ServiceOption {
	return func(s *service) { s.mmMappings = m }
}

// WithLogger enables structured logging for the campaigns service.
func WithLogger(l observability.Logger) ServiceOption {
	return func(s *service) { s.logger = l }
}

// WithCertEnrichEnqueuer injects a cert enrichment queue for background enrichment.
// If not provided, no cert enrichment will occur (optional).
func WithCertEnrichEnqueuer(q CertEnrichEnqueuer) ServiceOption {
	return func(s *service) { s.certEnrichQueue = q }
}

// WithDisableBackgroundWorkers is a test-only option that prevents background workers
// (like crack cache refresh) from starting. This prevents races with non-thread-safe mocks.
func WithDisableBackgroundWorkers() ServiceOption {
	return func(s *service) { s.disableBackgroundWorkers = true }
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

// WithCompSummaryProvider enables Card Ladder comp analytics on inventory aging.
func WithCompSummaryProvider(p CompSummaryProvider) ServiceOption {
	return func(s *service) { s.compProv = p }
}

// WithIntelligenceRepo enables DH market intelligence enrichment in price-sync.
func WithIntelligenceRepo(r intelligence.Repository) ServiceOption {
	return func(s *service) { s.intelRepo = r }
}

// WithPendingItemRepository enables persistent storage of ambiguous/unmatched import items.
func WithPendingItemRepository(r PendingItemRepository) ServiceOption {
	return func(s *service) { s.pendingItemRepo = r }
}

func NewService(
	campaigns CampaignRepository,
	purchases PurchaseRepository,
	sales SaleRepository,
	analytics AnalyticsRepository,
	finance FinanceRepository,
	pricing PricingRepository,
	dh DHRepository,
	snapshots SnapshotRepository,
	opts ...ServiceOption,
) Service {
	s := &service{
		campaigns: campaigns,
		purchases: purchases,
		sales:     sales,
		analytics: analytics,
		finance:   finance,
		pricing:   pricing,
		dh:        dh,
		snapshots: snapshots,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.idGen == nil {
		panic("inventory.NewService: WithIDGenerator is required")
	}

	return s
}

// Close waits for any in-flight background goroutines (e.g. batchResolveCardIDs).
// The crack candidate cache and cert enrichment workers are now scheduler-managed.
func (s *service) Close() {
	s.wg.Wait()
}

// Compile-time checks: service satisfies Service and each sub-interface.
var (
	_ Service           = (*service)(nil)
	_ CRUDService       = (*service)(nil)
	_ AnalyticsService  = (*service)(nil)
	_ ImportService     = (*service)(nil)
	_ FinanceService    = (*service)(nil)
	_ PricingService    = (*service)(nil)
	_ CertLookupService = (*service)(nil)
	_ SnapshotService   = (*service)(nil)
	_ DHService         = (*service)(nil)
)
