package inventory

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
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
// It is a value object — there is no identity field; equality is field-for-field.
//
// Field groups:
//   - Basic: LastSoldCents, LastSoldDate, MidPriceCents, SaleCount, GradePriceCents, LowestListCents, ActiveListings
//   - Sales volume: SalesLast30d, SalesLast90d
//   - Percentile distribution: ConservativeCents (P25), MedianCents (P50), OptimisticCents (P75), P10Cents, P90Cents
//   - Trend: Trend30d, Trend90d, Volatility
//   - Velocity: DailyVelocity, WeeklyVelocity, MonthlyVelocity
//   - Short-term signal: Avg7DayCents
//   - Source metadata: SourceCount, Sources, Confidence, SourcePrices, DistSampleSize, DistPeriodDays
//   - Estimate (secondary source): EstimatedValueCents, EstimateSource, IsEstimated
//   - Diagnostic flags: PricingGap
//   - CL reference: CLValueCents, CLDeviationPct, CLAnchorApplied
//
// Zero values mean "no data available" for that field — a zero GradePriceCents is not a free card.
type MarketSnapshot struct {
	LastSoldCents     int     `json:"lastSoldCents"`
	LastSoldDate      string  `json:"lastSoldDate,omitempty"`
	MidPriceCents     int     `json:"midPriceCents,omitempty"`
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
	// SourceCountRaw is len(Sources) at build time, BEFORE applyCLCorrection
	// appends the "cardladder" anchor. This is the external-platform count.
	SourceCountRaw int `json:"sourceCountRaw,omitempty"`
	// MarketDataObserved is true when the DH CardLookup returned a market block
	// (price.Market != nil) — distinguishing an observed zero from a missing one.
	MarketDataObserved bool `json:"marketDataObserved,omitempty"`

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
	// LookupImages fetches front/back slab image URLs for a PSA cert. Returns
	// empty strings (not an error) when PSA has no images for the cert.
	LookupImages(ctx context.Context, certNumber string) (front, back string, err error)
}

// CertEnrichEnqueuer enqueues certificate numbers for background enrichment.
type CertEnrichEnqueuer interface {
	Enqueue(certNumber string)
}

// PricingEnqueuer enqueues certificate numbers for immediate on-demand
// pricing via each configured price provider (CL). Intake paths call this
// after creating a purchase so freshly scanned inventory gets priced without
// waiting for the daily refresh cycle.
//
// Implementations must be non-blocking — intake flows should not stall on a
// full queue or slow provider.
type PricingEnqueuer interface {
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

// Service is the full campaigns API. It is the contract for consumers that
// genuinely need most of it — CampaignsHandler alone calls roughly forty of
// these methods, so narrowing it would buy nothing. Consumers that need less
// depend on one of the sub-interfaces in service_interfaces.go instead; each of
// those has at least one such consumer.
//
// Campaign/purchase/sale CRUD is declared inline here rather than as a
// sub-interface because no consumer wants it on its own.
type Service interface {
	AnalyticsService
	CertService
	PricingService
	CertLookupService
	SnapshotService
	DHService
	IntakeSupportService

	CreateCampaign(ctx context.Context, c *Campaign) error
	GetCampaign(ctx context.Context, id string) (*Campaign, error)
	ListCampaigns(ctx context.Context, activeOnly bool) ([]Campaign, error)
	UpdateCampaign(ctx context.Context, c *Campaign) error
	// UpdateCampaignIfUnchanged is UpdateCampaign for read-modify-write callers;
	// see CampaignRepository for why both exist.
	UpdateCampaignIfUnchanged(ctx context.Context, c *Campaign, expectedUpdatedAt time.Time) error
	DeleteCampaign(ctx context.Context, id string) error

	CreatePurchase(ctx context.Context, p *Purchase) error
	GetPurchase(ctx context.Context, id string) (*Purchase, error)
	DeletePurchase(ctx context.Context, id string) error
	ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Purchase, error)

	CreateSale(ctx context.Context, s *Sale, campaign *Campaign, purchase *Purchase) error
	CreateBulkSales(ctx context.Context, campaignID string, channel SaleChannel, saleDate string, items []BulkSaleInput) (*BulkSaleResult, error)
	ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Sale, error)
	DeleteSaleByPurchaseID(ctx context.Context, purchaseID string) error
	UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string) error

	ReassignPurchase(ctx context.Context, purchaseID string, newCampaignID string) error

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

	priceProv          PriceLookup
	certLookup         CertLookup
	cardIDResolver     CardIDResolver
	psaResolver        PSACampaignResolver // optional — resolves PSA campaign names to internal campaign IDs
	eventRec           dhevents.Recorder   // optional — DH state-transition event recorder
	logger             observability.Logger
	idGen              func() string // generates unique IDs; must be injected via WithIDGenerator
	maxSnapshotRetries int           // max retry attempts for failed snapshots (0 = unlimited)

	compProv        CompSummaryProvider     // optional — Card Ladder comp analytics
	intelRepo       intelligence.Repository // optional — DH market intelligence for price-sync enrichment
	pendingItemRepo PendingItemRepository   // optional — stores ambiguous/unmatched items from imports

	// certEnrichQueue enqueues cert numbers for background enrichment (optional).
	// A scheduler job processes cert numbers sequentially, respecting PSA API rate limits (100/day).
	certEnrichQueue CertEnrichEnqueuer

	// pricingQueue enqueues cert numbers for on-demand CL pricing (optional).
	// The intake flow calls this after creating a purchase so freshly scanned
	// inventory gets priced without waiting for the daily scheduler.
	pricingQueue PricingEnqueuer

	// dhSaleRecorder records (and, on un-sell, voids) sales on DH via the
	// purpose-built sale endpoint.
	dhSaleRecorder DHSaleRecorder

	// disableBackgroundWorkers is a test-only flag to prevent background workers from running.
	disableBackgroundWorkers bool

	// wg tracks background goroutines (e.g. batchResolveCardIDs, card ID resolver).
	// Note: cert enrichment worker is now managed by scheduler, not here.
	wg sync.WaitGroup
}

// ServiceOption configures optional service dependencies.
type ServiceOption func(*service)

// WithPriceLookup enables market signal computation on inventory aging.
func WithPriceLookup(pl PriceLookup) ServiceOption {
	return func(s *service) { s.priceProv = pl }
}

// WithPSACampaignResolver enables PSA-authoritative campaign attribution.
func WithPSACampaignResolver(r PSACampaignResolver) ServiceOption {
	return func(s *service) { s.psaResolver = r }
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

// WithCertEnrichEnqueuer injects a cert enrichment queue for background enrichment.
// If not provided, no cert enrichment will occur (optional).
func WithCertEnrichEnqueuer(q CertEnrichEnqueuer) ServiceOption {
	return func(s *service) { s.certEnrichQueue = q }
}

// WithPricingEnqueuer injects a pricing enqueuer for intake-time CL pricing.
// If not provided, new certs wait for the daily refresh cycle (optional).
func WithPricingEnqueuer(q PricingEnqueuer) ServiceOption {
	return func(s *service) { s.pricingQueue = q }
}

// WithDisableBackgroundWorkers is a test-only option that prevents background workers
// from starting. This prevents races with non-thread-safe mocks.
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

// WithDHSaleRecorder injects a DH sale recorder so a local sale is also
// recorded (and, on un-sell, voided) on DH. Optional — if nil, no DH sale
// call is made and the sale still commits locally.
func WithDHSaleRecorder(r DHSaleRecorder) ServiceOption {
	return func(s *service) { s.dhSaleRecorder = r }
}

// WithEventRecorder enables dh_state_events recording for enrollment and
// card-id-resolution transitions written by this service. Optional — nil
// means no events are written.
func WithEventRecorder(r dhevents.Recorder) ServiceOption {
	return func(s *service) { s.eventRec = r }
}

// recordEvent is a nil-safe helper that writes a DH state event. Failures are
// logged at Warn and never propagated to the caller.
func (s *service) recordEvent(ctx context.Context, e dhevents.Event) {
	if s.eventRec == nil {
		return
	}
	if err := s.eventRec.Record(ctx, e); err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "inventory: record dh event failed",
				observability.String("type", string(e.Type)),
				observability.Err(err))
		}
	}
}

func NewService(
	campaigns CampaignRepository,
	purchases PurchaseRepository,
	sales SaleRepository,
	analytics AnalyticsRepository,
	finance FinanceRepository,
	pricing PricingRepository,
	dh DHRepository,
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
// The cert enrichment worker is scheduler-managed.
func (s *service) Close() {
	s.wg.Wait()
}

// Compile-time checks: service satisfies Service and each sub-interface.
var (
	_ Service              = (*service)(nil)
	_ AnalyticsService     = (*service)(nil)
	_ CertService          = (*service)(nil)
	_ PricingService       = (*service)(nil)
	_ CertLookupService    = (*service)(nil)
	_ SnapshotService      = (*service)(nil)
	_ DHService            = (*service)(nil)
	_ IntakeSupportService = (*service)(nil)
)
