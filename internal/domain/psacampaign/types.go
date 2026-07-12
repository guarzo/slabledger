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
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
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
	PushPushed   PushStatus = "pushed"
	PushFailed   PushStatus = "failed"
)

// FieldChange is one proposed field mutation (old -> new), for audit + UI diff.
type FieldChange struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// ProposedDiff is the set of changes a proposal wants to apply to a campaign.
type ProposedDiff struct {
	Changes []FieldChange `json:"changes"`
}
