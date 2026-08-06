package psacampaign

import "time"

// PortalCampaign is the parsed offer-program config for one PSA campaign.
type PortalCampaign struct {
	CampaignRequestID string         `json:"campaignRequestId"`
	Name              string         `json:"name"`
	Type              string         `json:"type"`     // e.g. "CATEGORY"
	Status            string         `json:"status"`   // e.g. "PAUSED"
	Category          string         `json:"category"` // e.g. "POKEMON"
	BuyPercentClv     int            `json:"buyPercentClv"`
	BuyBox            CampaignBuyBox `json:"buyBox"`
	DailyBudgetCents  int            `json:"dailyBudgetCents"`
	DailySpecLimit    int            `json:"dailySpecLimit"`
	SubjectFilter     CampaignFilter `json:"subjectFilter"`
	PublisherFilter   CampaignFilter `json:"publisherFilter"`
	SpecListIDs       []string       `json:"specListIds"`
	// SpecListNames is display-only: it resolves each id in SpecListIDs
	// against the curated catalog known at decode time, skipping any id the
	// catalog does not explain. It is therefore not index-aligned with
	// SpecListIDs and must not be zipped against it positionally.
	SpecListNames []string     `json:"specListNames"`
	DeniedSpecs   []SubjectRef `json:"deniedSpecs"`
	CreatedAt     time.Time    `json:"createdAt"`
	UpdatedAt     time.Time    `json:"updatedAt"`

	// TargetingComplete is false when the edit-form fetch for this campaign
	// failed, or the fetched edit-form response could not be decoded (missing
	// or malformed formData, or a decodeFormData error) — in every such case
	// the targeting fields above are zero values rather than portal truth.
	// The baseline pull refuses to write a campaign row from an incomplete
	// record.
	TargetingComplete bool `json:"targetingComplete"`
}

// CampaignBuyBox holds the offer bounds. Prices in cents.
type CampaignBuyBox struct {
	GradeMin          string `json:"gradeMin"`
	GradeMax          string `json:"gradeMax"`
	YearMin           int    `json:"yearMin"`
	YearMax           int    `json:"yearMax"`
	PriceMinCents     int    `json:"priceMinCents"`
	PriceMaxCents     int    `json:"priceMaxCents"`
	ClvConfidenceMin  int    `json:"clvConfidenceMin"`
	BuyerFlatFeeCents int    `json:"buyerFlatFeeCents"`
}

// CampaignFilter is a Target (allow) or Exclude (deny) list of subjects.
type CampaignFilter struct {
	Type     string       `json:"type"` // "Target" | "Exclude"
	Subjects []SubjectRef `json:"subjects"`
}

// SubjectRef is a PSA subject id + display name.
type SubjectRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CampaignFormData is the write shape echoed to updateCampaign (superset used
// for read-modify-write). Prices here are whole USD to match the portal wire.
type CampaignFormData struct {
	CampaignName                string       `json:"campaignName"`
	CampaignType                string       `json:"campaignType"`
	Category                    string       `json:"category"`
	PrepackagedSpecListIDs      []string     `json:"prepackagedSpecListIds"`
	IsActive                    bool         `json:"isActive"`
	BidPercentage               int          `json:"bidPercentage"`
	FlatFee                     int          `json:"flatFee"`
	DailyBudget                 int          `json:"dailyBudget"`
	DailySpecLimit              int          `json:"dailySpecLimit"`
	GradeMinimum                string       `json:"gradeMinimum"`
	GradeMaximum                string       `json:"gradeMaximum"`
	YearMinimum                 int          `json:"yearMinimum"`
	YearMaximum                 int          `json:"yearMaximum"`
	PriceMinimum                int          `json:"priceMinimum"`
	PriceMaximum                int          `json:"priceMaximum"`
	CardLadderConfidenceMinimum int          `json:"cardLadderConfidenceMinimum"`
	PublisherFilterType         string       `json:"publisherFilterType"`
	SelectedPublishers          []SubjectRef `json:"selectedPublishers"`
	SubjectFilterType           string       `json:"subjectFilterType"`
	SelectedSubjects            []SubjectRef `json:"selectedSubjects"`
	DeniedSpecs                 []SubjectRef `json:"deniedSpecs"`
}

// PushStatus is the lifecycle of a queued edit.
type PushStatus string

const (
	PushPending  PushStatus = "pending"
	PushApproved PushStatus = "approved"
	PushPushing  PushStatus = "pushing"
	PushPushed   PushStatus = "pushed"
	PushFailed   PushStatus = "failed"
)

// Operation distinguishes what a queued push row does at the portal.
type Operation string

const (
	OpUpdate Operation = "update"
	OpCreate Operation = "create"
)

// FieldChange is one proposed field mutation (old -> new), for audit + UI diff.
type FieldChange struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
	// Value carries the new value for list-valued fields, where the string
	// rendering in New is for display and audit only. Scalar fields leave
	// this nil and push.go falls back to New.
	Value any `json:"value,omitempty"`
}

// ProposedDiff is the payload of a queued push. For updates it holds the field
// changes; for creates it holds the full formData being created (the approver
// reviews every field, not a diff).
type ProposedDiff struct {
	Changes []FieldChange     `json:"changes,omitempty"`
	Create  *CampaignFormData `json:"create,omitempty"`
}
