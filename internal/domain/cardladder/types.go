// Package cardladder holds the data types exchanged across the CardLadder
// persistence seam: the stored connection config, the cert→collection-card
// mapping, and the three admin reports the HTTP layer renders.
//
// They live here, and not in the postgres adapter that produces them, so that
// consumers can declare the seam they need without importing storage. The
// handler-side interface is CardLadderStore in
// internal/adapters/httpserver/handlers/cardladder.go (SLA-92).
//
// These are pure data — no behaviour, no dependencies. The postgres package
// keeps its historical names as aliases, so every existing caller (scheduler,
// wiring, tests) refers to the same types.
package cardladder

// Config holds the stored CL connection configuration.
type Config struct {
	Email          string
	RefreshToken   string // decrypted
	CollectionID   string
	FirebaseAPIKey string
	FirebaseUID    string
}

// CardMapping maps a purchase cert to a CL collection card.
type CardMapping struct {
	SlabSerial         string
	CLCollectionCardID string
	CLGemRateID        string
	CLCondition        string
}

// PriceStats summarizes CL value freshness across unsold inventory.
type PriceStats struct {
	UnsoldTotal  int    `json:"unsoldTotal"`  // Total unsold purchases
	WithCLValue  int    `json:"withCLValue"`  // Unsold purchases CardLadder has priced (cl_value_updated_at set)
	SyncedCount  int    `json:"syncedCount"`  // Unsold purchases pushed to the CL remote collection
	OldestUpdate string `json:"oldestUpdate"` // Oldest cl_value_updated_at among priced cards
	NewestUpdate string `json:"newestUpdate"` // Newest cl_value_updated_at
	StaleCount   int    `json:"staleCount"`   // Priced cards whose value is older than 7 days
}

// IntegrationFailureSample is a single failed-mapping row returned by the
// /failures admin endpoint.
type IntegrationFailureSample struct {
	PurchaseID string `json:"purchaseId"`
	CertNumber string `json:"certNumber"`
	CardName   string `json:"cardName"`
	Reason     string `json:"reason"`
	ErrorAt    string `json:"errorAt"`
}

// IntegrationFailuresReport groups per-purchase failure reasons for an integration.
type IntegrationFailuresReport struct {
	ByReason map[string]int             `json:"byReason"`
	Samples  []IntegrationFailureSample `json:"samples"`
}

// CoverageCohort is the per-cohort breakdown for one month.
//
// Rows is the full total and deliberately does NOT equal the Pct denominator:
// Rows = Resolved + Unresolved + Pending + Stranded + PreCL. It is reported so
// the excluded counts can be reconciled by a reader.
type CoverageCohort struct {
	Rows       int      `json:"rows"`
	Resolved   int      `json:"resolved"`
	Unresolved int      `json:"unresolved"`
	Pending    int      `json:"pending"`
	Stranded   int      `json:"stranded"`
	PreCL      int      `json:"preCL"`
	Pct        *float64 `json:"pct"`
}

// CoverageMonth is one purchase month, split by intake cohort.
//
// Reassigned counts rows whose purchase_source is set but whose campaign_id is
// 'external' -- i.e. the two possible cohort definitions disagree. It is
// reported so that drift between them is visible rather than silent.
type CoverageMonth struct {
	Month              string         `json:"month"`
	Reassigned         int            `json:"reassigned"`
	Campaign           CoverageCohort `json:"campaign"`
	External           CoverageCohort `json:"external"`
	UnresolvedByReason map[string]int `json:"unresolvedByReason"`
}

// CoverageReport is the full response. EraStart echoes the pinned era constant
// (postgres.CLCoverageEraStart) so a reader can see what PreCL was measured
// against.
type CoverageReport struct {
	EraStart string          `json:"eraStart"`
	Months   []CoverageMonth `json:"months"`
}
