package inventory

import "time"

// ActivationCheck represents a single pre-activation checklist item.
type ActivationCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// ActivationChecklist contains the full pre-activation advisory.
type ActivationChecklist struct {
	CampaignID   string            `json:"campaignId"`
	CampaignName string            `json:"campaignName"`
	AllPassed    bool              `json:"allPassed"`
	Checks       []ActivationCheck `json:"checks"`
	Warnings     []string          `json:"warnings"`
}

// RevocationFlag represents a segment flagged for PSA revocation.
type RevocationFlag struct {
	ID               string     `json:"id"`
	SegmentLabel     string     `json:"segmentLabel"`
	SegmentDimension string     `json:"segmentDimension"`
	Reason           string     `json:"reason"`
	Status           string     `json:"status"` // "pending", "sent"
	EmailText        string     `json:"emailText"`
	CreatedAt        time.Time  `json:"createdAt"`
	SentAt           *time.Time `json:"sentAt,omitempty"`
}
