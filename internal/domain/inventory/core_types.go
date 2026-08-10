package inventory

import (
	"encoding/json"
	"time"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

// Phase represents the lifecycle state of a campaign.
type Phase string

const (
	PhasePending Phase = "pending"
	PhaseActive  Phase = "active"
	PhaseClosed  Phase = "closed"
)

// DHStatus represents the DoubleHolo inventory status.
type DHStatus = string

const (
	DHStatusInStock DHStatus = "in_stock"
	DHStatusListed  DHStatus = "listed"
	DHStatusSold    DHStatus = "sold"
)

// NormalizeDHStatus guards the dh_status column against undocumented values DH
// has returned and we used to persist verbatim (e.g. "skipped"). It returns the
// status unchanged when it is one of DH's known inventory states, and ""
// otherwise. Persisting "" keeps the row out of the price-sync PATCH path
// (which only accepts in_stock/listed) instead of letting a bogus status 422 on
// every tick. Callers that need a concrete default (e.g. a fresh push) handle
// the "" case themselves.
func NormalizeDHStatus(status string) string {
	switch status {
	case DHStatusInStock, DHStatusListed, DHStatusSold:
		return status
	default:
		return ""
	}
}

// DHStatusForPush maps a raw status from a DH push/import response to the value
// to persist in dh_status. It encodes the single rule every DH push site needs:
//   - a recognized inventory status (in_stock/listed/sold) passes through;
//   - an empty status (DH said nothing — a brand-new push) defaults to in_stock;
//   - a non-empty but unrecognized value (e.g. DH's psa_import "skipped" =
//     "cert already on DH, existing inventory left untouched") drops to "".
//     Guessing in_stock for that case would let the price-sync PATCH overwrite
//     — and delist — a real "listed" item; the price-sync guard keeps "" rows
//     out of the PATCH path entirely. The "" is later reconciled to DH's
//     authoritative status by the DH reconciler's full-snapshot sweep (the
//     checkpoint-gated inventory poll can't, since it never re-fetches an
//     unchanged item).
func DHStatusForPush(raw string) DHStatus {
	if s := NormalizeDHStatus(raw); s != "" {
		return s
	}
	if raw == "" {
		return DHStatusInStock
	}
	return ""
}

// DHPushStatus represents the DH inventory push pipeline status.
type DHPushStatus = string

const (
	DHPushStatusPending   DHPushStatus = "pending"
	DHPushStatusMatched   DHPushStatus = "matched"
	DHPushStatusUnmatched DHPushStatus = "unmatched"
	DHPushStatusManual    DHPushStatus = "manual"
	DHPushStatusHeld      DHPushStatus = "held"
	DHPushStatusDismissed DHPushStatus = "dismissed"
)

// SaleChannel represents where a card was sold.
// Defined in constants package; aliased here for backward compatibility.
type SaleChannel = constants.SaleChannel

const (
	SaleChannelEbay     = constants.SaleChannelEbay
	SaleChannelWebsite  = constants.SaleChannelWebsite
	SaleChannelInPerson = constants.SaleChannelInPerson
)

// Legacy channel values — kept for backward-compatible DB reads.
const (
	SaleChannelTCGPlayer  = constants.SaleChannelTCGPlayer
	SaleChannelLocal      = constants.SaleChannelLocal
	SaleChannelOther      = constants.SaleChannelOther
	SaleChannelGameStop   = constants.SaleChannelGameStop
	SaleChannelCardShow   = constants.SaleChannelCardShow
	SaleChannelDoubleHolo = constants.SaleChannelDoubleHolo
)

const (
	ExternalCampaignID   = "external"
	ExternalCampaignName = "External"
)

// OverrideSource identifies how a price override was set.
type OverrideSource string

const (
	OverrideSourceNone       OverrideSource = ""
	OverrideSourceManual     OverrideSource = "manual"
	OverrideSourceCostMarkup OverrideSource = "cost_markup"
	OverrideSourceAIAccepted OverrideSource = "ai_accepted"
)

// MarketSnapshotData holds market snapshot fields shared by Purchase and Sale.
// Core fields are stored as individual columns for SQL queries.
// The full MarketSnapshot (including SourcePrices, velocity, etc.) is stored
// as JSON in SnapshotJSON for the frontend without adding many DB columns.
type MarketSnapshotData struct {
	LastSoldCents     int     `json:"lastSoldCents,omitempty"`
	LowestListCents   int     `json:"lowestListCents,omitempty"`
	ConservativeCents int     `json:"conservativeCents,omitempty"`
	MedianCents       int     `json:"medianCents,omitempty"`
	MidPriceCents     int     `json:"midPriceCents,omitempty"`
	LastSoldDate      string  `json:"lastSoldDate,omitempty"`
	ActiveListings    int     `json:"activeListings,omitempty"`
	SalesLast30d      int     `json:"salesLast30d,omitempty"`
	Trend30d          float64 `json:"trend30d,omitempty"`
	SnapshotDate      string  `json:"snapshotDate,omitempty"`
	SnapshotJSON      string  `json:"-"` // Full MarketSnapshot serialized as JSON (DB column, not in API)

	// Decision-time provenance: read in-process by the freeze paths and written to
	// the *_at_purchase columns. Not part of the API wire format (json:"-") — the
	// frozen snapshots live in dedicated Purchase/Sale fields, not on this embed.
	Confidence         float64 `json:"-"` // DH pricing confidence
	SourceCountRaw     int     `json:"-"` // external platform count, pre-CL-correction
	MarketDataObserved bool    `json:"-"` // true when CardLookup market data was present
}

func (d *MarketSnapshotData) applySnapshot(snapshot *MarketSnapshot, date string) {
	d.LastSoldCents = snapshot.LastSoldCents
	d.LowestListCents = snapshot.LowestListCents
	d.ConservativeCents = snapshot.ConservativeCents
	d.MedianCents = snapshot.MedianCents
	d.MidPriceCents = snapshot.MidPriceCents
	d.LastSoldDate = snapshot.LastSoldDate
	d.ActiveListings = snapshot.ActiveListings
	d.SalesLast30d = snapshot.SalesLast30d
	d.Trend30d = snapshot.Trend30d
	d.SnapshotDate = date
	d.Confidence = snapshot.Confidence
	d.SourceCountRaw = snapshot.SourceCountRaw
	d.MarketDataObserved = snapshot.MarketDataObserved

	// Persist the full snapshot as JSON for frontend consumption
	if b, err := json.Marshal(snapshot); err == nil {
		d.SnapshotJSON = string(b)
	} else {
		d.SnapshotJSON = ""
	}
}

// TargetSubject is one portal-sourced targeting entity: a character subject or
// a card-level spec. ID is copied verbatim from the portal and is never
// re-resolved from Name — live IDs span multiple generations (4xxx, 8xxx,
// 22xxx) while getSubjects returns only 22xxx.
type TargetSubject struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SubjectFilterMode values. Target buys only the listed subjects; Exclude
// buys everything except them.
const (
	SubjectFilterTarget  = "Target"
	SubjectFilterExclude = "Exclude"
)

// LegacyUnreconciledSubjectID marks a subject that migration 000024 backfilled
// from the legacy inclusion_list string: a name with no portal id behind it
// and no reconciliation against live portal state yet.
//
// It exists because id 0 is already taken. An operator who types a new
// subject name in the UI creates it with id 0 (SubjectListEditor.tsx), and
// TranslateToCreate/TranslateToDiff deliberately resolve those by name. If
// backfilled legacy subjects also carried 0, a push issued between deploy and
// the baseline pull would re-resolve them by name and swap the live 4xxx/8xxx
// portal ids on six money-spending campaigns for current-generation 22xxx
// ids. -1 cannot collide with either case: portal-issued ids are positive.
const LegacyUnreconciledSubjectID = -1

// Campaign represents a PSA Direct Buy campaign with buy parameters and fee configuration.
type Campaign struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Sport              string  `json:"sport"`
	YearRange          string  `json:"yearRange"`          // e.g. "1999-2003"
	GradeRange         string  `json:"gradeRange"`         // e.g. "9-10"
	PriceRange         string  `json:"priceRange"`         // e.g. "50-500"
	CLConfidence       string  `json:"clConfidence"`       // CL confidence range, e.g. "2.5-4"
	BuyTermsCLPct      float64 `json:"buyTermsCLPct"`      // Buy at X% of CL value (0-1)
	DailySpendCapCents int     `json:"dailySpendCapCents"` // Max daily spend in cents

	// InclusionList and ExclusionMode are a legacy mirror kept for one release
	// so a rollback to the previous binary sees a database that still matches
	// its own model. campaign_store.go derives both from
	// Subjects/SubjectFilterMode on every write, discarding whatever a caller
	// sets on these two fields directly. Nothing in the codebase reads them
	// anymore — matching.go, portfolio.go, suggestion_rules.go,
	// portfolio/analysis.go, demand/campaign_signals.go, and
	// campaign_coverage.go were all switched to the new axes in Task 4. They
	// are write-only at this point, kept solely for the rollback guarantee
	// above.
	InclusionList string `json:"inclusionList"`
	ExclusionMode bool   `json:"exclusionMode"`

	// TargetLanguages is the set of PSA curated spec lists the campaign buys
	// from, held as stable internal tokens rather than portal UUIDs (which PSA
	// can re-issue). It is an unordered set; ValidateAndNormalizeCampaign
	// (validation.go) sorts it so persistence and diffs stay deterministic.
	//
	// Empty means an open net: the campaign buys any language. Every live
	// campaign carries BOTH "english" and "japanese" — the single-token model
	// this replaced could not represent them.
	//
	// The closed set is "english" | "japanese" only. cardutil.SetLanguage
	// classifies chinese and korean sets too, but the portal offers no curated
	// spec list for either, so those tokens are rejected rather than stored
	// unmatchable.
	TargetLanguages []string `json:"targetLanguages"`

	// SubjectFilterMode is the polarity of Subjects: Target buys only the
	// listed characters, Exclude buys everything except them. Empty is
	// normalized to SubjectFilterTarget on read.
	SubjectFilterMode string `json:"subjectFilterMode"`

	// Subjects are the characters this campaign targets or excludes. ID is the
	// PSA subject id and is authoritative — it is never re-derived from Name.
	Subjects []TargetSubject `json:"subjects"`

	// DeniedSpecs are individual cards excluded regardless of Subjects.
	DeniedSpecs []TargetSubject `json:"deniedSpecs"`

	Phase                Phase     `json:"phase"`
	PSASourcingFeeCents  int       `json:"psaSourcingFeeCents"`            // Default 300 ($3)
	EbayFeePct           float64   `json:"ebayFeePct"`                     // Default 0.1235 (12.35%)
	ExpectedFillRate     float64   `json:"expectedFillRate"`               // Target fill rate as percentage (0-100)
	PSACampaignRequestID string    `json:"psaCampaignRequestId,omitempty"` // 1:1 link to PSA portal campaign
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
	Kind                 string    `json:"kind"` // Derived at HTTP layer: "external" or "standard" (not persisted)
}

// SetKind sets the Kind field based on the campaign ID.
// Kind is derived at the HTTP layer and not persisted.
func (c *Campaign) SetKind() {
	if c.ID == ExternalCampaignID {
		c.Kind = "external"
	} else {
		c.Kind = "standard"
	}
}

// SnapshotStatus represents the state of background market snapshot enrichment.
type SnapshotStatus string

// Snapshot status constants for Purchase.SnapshotStatus.
const (
	SnapshotStatusNone      SnapshotStatus = ""          // snapshot captured or not needed
	SnapshotStatusPending   SnapshotStatus = "pending"   // awaiting background enrichment
	SnapshotStatusFailed    SnapshotStatus = "failed"    // enrichment attempt failed, will retry
	SnapshotStatusExhausted SnapshotStatus = "exhausted" // max retries reached, requires manual fix
)

// CL provenance source discriminators for *AtPurchase / *AtSale value freezes.
// See migration 000041 and docs/superpowers/specs/2026-08-10-buy-decision-provenance-design.md
// (D5): a frozen CL value can come from two different writers -- the
// create-time copy of whatever value the intake carried, or CardLadder's own
// refresh sweep -- and only the latter is a genuine "CardLadder answered"
// observation. "" (the zero value) means unknown provenance, for every row
// written before this column existed.
const (
	CLProvenanceSourceIntake     = "intake"
	CLProvenanceSourceCardLadder = "cardladder"
)

// Purchase represents a single card purchased through a campaign.
type Purchase struct {
	// --- Core identity ---
	ID         string  `json:"id"`
	CampaignID string  `json:"campaignId"`
	CardName   string  `json:"cardName"`
	CertNumber string  `json:"certNumber"`           // PSA cert number (unique)
	CardNumber string  `json:"cardNumber,omitempty"` // Card number within set (from PSA)
	SetName    string  `json:"setName,omitempty"`    // Set/category name (from PSA)
	Grader     string  `json:"grader,omitempty"`     // e.g. "PSA", "CGC", "BGS", "SGC"
	GradeValue float64 `json:"gradeValue"`           // Numeric grade (1-10, supports half-grades like 9.5)

	// --- Card Ladder data ---
	CLValueCents           int    `json:"clValueCents"`                     // Current CL market value (scheduler-refreshed; frozen snapshot lives in CLValueAtPurchaseCents)
	CLValueUpdatedAt       string `json:"clValueUpdatedAt,omitempty"`       // When CL value was last refreshed (RFC3339)
	CLValueAtPurchaseCents int    `json:"clValueAtPurchaseCents,omitempty"` // CL value at purchase/first-enrichment; set once, never overwritten (0 = no snapshot)

	// CLValueAtPurchaseObservedAt/CLValueAtPurchaseSource record WHEN and HOW
	// CLValueAtPurchaseCents was captured; see CLProvenanceSourceIntake/
	// CLProvenanceSourceCardLadder and migration 000041. "" for both means
	// this purchase predates the column (provenance unknown). CLCardConfidenceAtPurchase
	// is CardLadder's own per-card comp confidence (resp.Confidence) -- distinct
	// from CLConfidenceAtPurchase below, which is the campaign's configured
	// policy minimum, not an observation about the card.
	CLValueAtPurchaseObservedAt string `json:"clValueAtPurchaseObservedAt,omitempty"`
	CLValueAtPurchaseSource     string `json:"clValueAtPurchaseSource,omitempty"`
	CLCardConfidenceAtPurchase  *int   `json:"clCardConfidenceAtPurchase,omitempty"`

	// --- Decision-time provenance (frozen once at CreatePurchase; server-derived only) ---
	CLConfidenceAtPurchase          *int     `json:"clConfidenceAtPurchase,omitempty"`          // MISNAMED: this is the campaign's policy floor (ParseCLConfidenceMin), not card confidence. Superseded by CLPolicyConfidenceMinAtPurchase; kept only for the deploy-N/N+1 API compatibility window (see migration 000042). The Go field goes away in Task 11 (deploy N+1); the DB column outlives it by one deploy and is dropped in Task 12.
	CLPolicyConfidenceMinAtPurchase *int     `json:"clPolicyConfidenceMinAtPurchase,omitempty"` // The campaign's configured CL-confidence policy minimum at purchase time (ParseCLConfidenceMin(campaign.CLConfidence)). NOT the card's real CardLadder confidence -- see CLCardConfidenceAtPurchase for that.
	PopulationAtPurchase            *int     `json:"populationAtPurchase,omitempty"`
	DHConfidenceAtPurchase          *float64 `json:"dhConfidenceAtPurchase,omitempty"`
	SourceCountAtPurchase           *int     `json:"sourceCountAtPurchase,omitempty"`
	ActiveListingsAtPurchase        *int     `json:"activeListingsAtPurchase,omitempty"`
	SalesLast30dAtPurchase          *int     `json:"salesLast30dAtPurchase,omitempty"`

	// --- Campaign attribution provenance ---
	PSACampaignName   string `json:"psaCampaignName,omitempty"`   // raw campaign name PSA reported, verbatim
	AttributionSource string `json:"attributionSource,omitempty"` // psa | inferred | manual

	// --- Purchase cost & logistics ---
	BuyCostCents        int     `json:"buyCostCents"`         // Actual cost paid
	PSASourcingFeeCents int     `json:"psaSourcingFeeCents"`  // Fee charged per card
	Population          int     `json:"population,omitempty"` // PSA population count
	PurchaseDate        string  `json:"purchaseDate"`         // YYYY-MM-DD
	ReceivedAt          *string `json:"receivedAt,omitempty"`
	PSAShipDate         string  `json:"psaShipDate,omitempty"`
	InvoiceDate         string  `json:"invoiceDate,omitempty"`
	WasRefunded         bool    `json:"wasRefunded,omitempty"`

	// --- Media ---
	FrontImageURL string `json:"frontImageUrl,omitempty"`
	BackImageURL  string `json:"backImageUrl,omitempty"`

	// --- Metadata ---
	PurchaseSource  string `json:"purchaseSource,omitempty"`
	PSAListingTitle string `json:"psaListingTitle,omitempty"` // Raw PSA listing title for pricing fallback

	// --- Market snapshot enrichment ---
	SnapshotStatus     SnapshotStatus `json:"snapshotStatus,omitempty"` // see SnapshotStatus* constants
	SnapshotRetryCount int            `json:"snapshotRetryCount,omitempty"`

	// --- Price override ---
	OverridePriceCents int            `json:"overridePriceCents,omitempty"`
	OverrideSource     OverrideSource `json:"overrideSource,omitempty"`
	OverrideSetAt      string         `json:"overrideSetAt,omitempty"`

	// --- AI suggestion ---
	AISuggestedPriceCents int    `json:"aiSuggestedPriceCents,omitempty"`
	AISuggestedAt         string `json:"aiSuggestedAt,omitempty"`

	// --- Review ---
	CardYear            string       `json:"cardYear,omitempty"`
	EbayExportFlaggedAt *time.Time   `json:"ebayExportFlaggedAt,omitempty"`
	ReviewedPriceCents  int          `json:"reviewedPriceCents,omitempty"`
	ReviewedAt          string       `json:"reviewedAt,omitempty"`
	ReviewSource        ReviewSource `json:"reviewSource,omitempty"`

	// --- DoubleHolo integration ---
	DHCardID            int          `json:"dhCardId,omitempty"`            // DH card identity (from cert resolution)
	DHInventoryID       int          `json:"dhInventoryId,omitempty"`       // DH inventory item ID (from inventory push)
	DHCertStatus        string       `json:"dhCertStatus,omitempty"`        // Resolution state: matched, ambiguous, not_found, unresolved, resolving
	DHListingPriceCents int          `json:"dhListingPriceCents,omitempty"` // Current DH listing price
	DHChannelsJSON      string       `json:"dhChannelsJson,omitempty"`      // Per-channel sync status JSON blob
	DHStatus            DHStatus     `json:"dhStatus,omitempty"`            // DH inventory status
	DHPushStatus        DHPushStatus `json:"dhPushStatus,omitempty"`        // Pipeline status: "", "pending", "matched", "unmatched", "manual", "held"
	DHPushAttempts      int          `json:"dhPushAttempts,omitempty"`      // Consecutive DH push skip count; reset when row re-enters pending or matched
	DHHoldReason        string       `json:"dhHoldReason,omitempty"`        // Why a re-push was held
	DHCandidatesJSON    string       `json:"dhCandidatesJson,omitempty"`    // Ambiguous cert resolution candidates JSON
	DHLastSyncedAt      string       `json:"dhLastSyncedAt,omitempty"`      // When DH inventory was last polled for this purchase (RFC3339)
	// DHUnlistedDetectedAt is set by the DH reconciler when this purchase's
	// dh_inventory_id was missing from the DH inventory snapshot (i.e. deleted
	// on DH). Cleared when the purchase is successfully re-listed. NULL otherwise.
	DHUnlistedDetectedAt *time.Time `json:"dhUnlistedDetectedAt,omitempty"`

	// --- Card Ladder enrichment ---
	GemRateID     string `json:"gemRateId,omitempty"`     // CL gemRateID (grade-agnostic card variant identifier)
	PSASpecID     int    `json:"psaSpecId,omitempty"`     // PSA spec ID from CL cards index
	CardPlayer    string `json:"cardPlayer,omitempty"`    // Player/subject name (e.g. "Charizard", "LeBron James")
	CardVariation string `json:"cardVariation,omitempty"` // Card variation (e.g. "Holo Rare", "1st Edition")
	CardCategory  string `json:"cardCategory,omitempty"`  // Sport/category (e.g. "Pokemon", "Basketball")
	CLSyncedAt    string `json:"clSyncedAt,omitempty"`    // When card was last synced to CL collection (RFC3339)
	CLLastError   string `json:"clLastError,omitempty"`   // Last CL mapping/pricing failure reason tag (e.g. "no_value", "catalog_fallback")

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Market snapshot at time of purchase (best-effort, may be zero)
	MarketSnapshotData
}

// ToCardIdentity returns a CardIdentity populated from this purchase's card metadata.
func (p *Purchase) ToCardIdentity() CardIdentity {
	return CardIdentity{
		CardName:        p.CardName,
		CardNumber:      p.CardNumber,
		SetName:         p.SetName,
		PSAListingTitle: p.PSAListingTitle,
	}
}

// IsReceivedOrShipped reports whether the card is physically in hand
// (ReceivedAt != nil) or has been shipped by PSA (PSAShipDate != ""). This is
// the same predicate the DH push scheduler gates on
// (GetPurchasesByDHPushStatus / receivedOrShippedPredicate) and is the minimum
// bar for putting an item live on DH: we must never list something that hasn't
// at least left PSA.
func (p *Purchase) IsReceivedOrShipped() bool {
	return p.ReceivedAt != nil || p.PSAShipDate != ""
}

// NeedsDHPush returns true if this purchase is eligible for DH push pipeline enrollment.
// A purchase is eligible once it has been received (ReceivedAt != nil) OR shipped by PSA
// (PSAShipDate != ""). Shipping alone is sufficient because DH allows 2 days from order
// placement to ship, which covers the typical 1-2 day PSA-to-receipt window.
// Purchases already marked sold on DH (DHStatus == DHStatusSold) are excluded to keep
// the in-memory check consistent with the DB-level GetPurchasesByDHPushStatus query,
// which gates on a missing sale row.
func (p *Purchase) NeedsDHPush() bool {
	return (p.ReceivedAt != nil || p.PSAShipDate != "") &&
		p.DHStatus != DHStatusSold &&
		p.DHInventoryID == 0 &&
		p.DHPushStatus != DHPushStatusPending &&
		p.DHPushStatus != DHPushStatusUnmatched &&
		p.DHPushStatus != DHPushStatusManual &&
		p.DHPushStatus != DHPushStatusHeld &&
		p.DHPushStatus != DHPushStatusDismissed
}

// DHCardKey returns the pipe-delimited identity key used for DH card ID mapping lookups.
func (p *Purchase) DHCardKey() string {
	return DHCardKey(p.CardName, p.SetName, p.CardNumber)
}

// DHCardKey builds the pipe-delimited identity key used by DH card ID mapping lookups.
func DHCardKey(cardName, setName, cardNumber string) string {
	return cardName + "|" + setName + "|" + cardNumber
}

// DHPipelineHealth is a small set of counts the DH status dashboard uses to
// reconcile "how many things the system says are queued" with "how many things
// the queue endpoint actually returns". PendingReceived matches the draining
// query in ListDHPendingItems; UnenrolledReceived surfaces the previously
// invisible bucket of received rows that never got dh_push_status set.
type DHPipelineHealth struct {
	PendingReceived    int `json:"pendingReceived"`
	UnenrolledReceived int `json:"unenrolledReceived"`
}

// DHPendingItem represents a received, unsold card currently in the DH push pipeline.
// Used by GET /api/dh/pending to show the operator what's queued for DH listing.
type DHPendingItem struct {
	PurchaseID            string  `json:"purchaseId"`
	CardName              string  `json:"cardName"`
	SetName               string  `json:"setName,omitempty"`
	Grade                 float64 `json:"grade"`
	RecommendedPriceCents int     `json:"recommendedPriceCents"`
	DaysQueued            int     `json:"daysQueued"`
	DHConfidence          string  `json:"dhConfidence"` // "high" (<24h), "medium" (<7d), "low" (>7d or never synced)
}

// Invoice tracks a PSA invoice cycle for capital exposure management.
type Invoice struct {
	ID                  string    `json:"id"`
	InvoiceDate         string    `json:"invoiceDate"`
	TotalCents          int       `json:"totalCents"`
	PaidCents           int       `json:"paidCents"`
	PendingReceiptCents int       `json:"pendingReceiptCents"`
	DueDate             string    `json:"dueDate,omitempty"`
	PaidDate            string    `json:"paidDate,omitempty"`
	Status              string    `json:"status"` // "unpaid", "partial", "paid"
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// CashflowConfig holds capital allocation and cash management settings.
type CashflowConfig struct {
	CapitalBudgetCents int       `json:"capitalBudgetCents"` // User-set target for max outstanding exposure (0 = no target)
	CashBufferCents    int       `json:"cashBufferCents"`
	UpdatedAt          time.Time `json:"updatedAt"`
}
