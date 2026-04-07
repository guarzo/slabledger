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

// Service is the full campaigns API — a composition of all sub-interfaces.
// Consumers that only need a subset should depend on the narrower interface
// (e.g. AnalyticsService, ImportService) to follow the Interface Segregation Principle.
// Sub-interfaces are defined in service_interfaces.go.
type Service interface {
	CRUDService
	AnalyticsService
	ImportService
	FinanceService
	PricingService
	CertLookupService
	SnapshotService
	DHService

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

	compProv  CompSummaryProvider     // optional — Card Ladder comp analytics
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

// WithCompSummaryProvider enables Card Ladder comp analytics on inventory aging.
func WithCompSummaryProvider(p CompSummaryProvider) ServiceOption {
	return func(s *service) { s.compProv = p }
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
)
