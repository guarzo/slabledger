// Package insights composes read-only portfolio-wide signals, actions, and
// per-campaign tuning recommendations for the Insights page.
package insights

// Overview is the full payload returned to the Insights page.
type Overview struct {
	Actions     []Action    `json:"actions"`
	Signals     Signals     `json:"signals"`
	Campaigns   []TuningRow `json:"campaigns"`
	GeneratedAt string      `json:"generatedAt"` // RFC3339
}

// Severity ranks items by how urgent action is.
type Severity string

const (
	SeverityAct  Severity = "act"
	SeverityTune Severity = "tune"
	SeverityOK   Severity = "ok"
)

// Action is one row in the "Do now" section.
type Action struct {
	ID          string     `json:"id"`
	Severity    Severity   `json:"severity"` // "act" or "tune" only in v1
	Title       string     `json:"title"`
	Detail      string     `json:"detail"`
	Link        ActionLink `json:"link"`
	ImpactCents int        `json:"impactCents,omitempty"`
}

// ActionLink points the frontend at a target page + optional filter query.
type ActionLink struct {
	Path  string            `json:"path"`
	Query map[string]string `json:"query,omitempty"`
}

// Signals are the four health-signal tiles.
type Signals struct {
	AIAcceptRate                AIAcceptRate `json:"aiAcceptRate"`
	LiquidationRecoverableCents int          `json:"liquidationRecoverableCents"`
	SpikeProfitCents            int          `json:"spikeProfitCents"`
	SpikeCertCount              int          `json:"spikeCertCount"`
	StuckInPipelineCount        int          `json:"stuckInPipelineCount"`
}

// AIAcceptRate is the 7-day AI suggestion acceptance rate.
type AIAcceptRate struct {
	Pct      float64 `json:"pct"` // 0.0 — 100.0
	Accepted int     `json:"accepted"`
	Resolved int     `json:"resolved"` // accepted + dismissed
}

// TuningRow is one campaign in the tuning table.
type TuningRow struct {
	CampaignID   string                `json:"campaignId"`
	CampaignName string                `json:"campaignName"`
	Cells        map[string]TuningCell `json:"cells"` // keys: buyPct, characters, years, spendCap
	Status       Status                `json:"status"`
}

// Status is the pill in the rightmost column.
type Status string

const (
	StatusAct  Status = "Act"
	StatusTune Status = "Tune"
	StatusOK   Status = "OK"
	StatusKill Status = "Kill"
)

// TuningCell is one cell in a campaign tuning row.
type TuningCell struct {
	Recommendation string   `json:"recommendation"` // e.g. "Raise 55 → 60%"
	Severity       Severity `json:"severity"`
}
