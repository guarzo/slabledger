// Package showprep owns price-support evidence and non-financial packing lists.
package showprep

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type Status string

const (
	Supported     Status = "supported"
	ThinEvidence  Status = "thin_evidence"
	BelowTarget   Status = "below_target"
	MixedEvidence Status = "mixed_evidence"
	NoRecentComps Status = "no_recent_comps"
	NeedsReview   Status = "needs_review"
	NoListedPrice Status = "no_listed_price"
)

type Identity struct {
	ProfileID string
	Grader    string
	Grade     float64
}

func (i Identity) Key() string { return Fingerprint(i) }
func (i Identity) Condition() string {
	return strings.ToUpper(i.Grader) + " " + strconv.FormatFloat(i.Grade, 'f', -1, 64)
}
func (i Identity) Valid() bool { return i.ProfileID != "" && i.Grader != "" && i.Grade > 0 }

type Purchase struct {
	ID, CampaignID, CardName, CertNumber, Grader, ProfileID string
	Grade                                                   float64
	Known, Exists, CampaignExists, Sold, Refunded, Received bool
	Phase                                                   string
	ListedPriceCents, LocalPriceCents                       int
	ListingSyncedAt                                         string
	PriceAssociationUnclear                                 bool
}

func (p Purchase) Identity() Identity {
	return Identity{strings.TrimSpace(p.ProfileID), strings.ToUpper(strings.TrimSpace(p.Grader)), p.Grade}
}

type Sale struct {
	ID          string `json:"id"`
	Date        string `json:"date"`
	PriceCents  int    `json:"priceCents"`
	Platform    string `json:"platform"`
	URL         string `json:"url"`
	ListingType string `json:"listingType"`
}

// Snapshot keeps the last readable payload separately from the latest attempt.
// Generation/Attempt are allocated by storage, never by wall-clock ordering.
type Snapshot struct {
	Identity                   Identity
	Source                     string
	Generation                 int64
	WindowStart, WindowEnd     string
	RefreshedAt                time.Time
	Complete                   bool
	Sales                      []Sale
	Attempt                    int64
	AttemptState, AttemptError string
	AttemptStartedAt           time.Time
}

type Evaluation struct {
	PurchaseID              string              `json:"purchaseId"`
	CardName                string              `json:"cardName"`
	CertNumber              string              `json:"certNumber"`
	Grader                  string              `json:"grader"`
	Grade                   float64             `json:"grade"`
	Status                  Status              `json:"status"`
	Reason                  string              `json:"reason"`
	EvidenceNeedsReview     bool                `json:"evidenceNeedsReview"`
	EvidenceReason          string              `json:"evidenceReason"`
	Availability            Availability        `json:"availability"`
	CanAdd                  bool                `json:"canAdd"`
	CanPack                 bool                `json:"canPack"`
	ListedPriceCents        int                 `json:"listedPriceCents"`
	LocalPriceCents         int                 `json:"localPriceCents"`
	PriceMismatch           bool                `json:"priceMismatch"`
	PriceAssociationUnclear bool                `json:"priceAssociationUnclear"`
	ListingSyncedAt         string              `json:"listingSyncedAt"`
	MedianCents             int                 `json:"medianCents"`
	CompCount               int                 `json:"compCount"`
	LatestSaleDate          string              `json:"latestSaleDate"`
	WindowStart             string              `json:"windowStart"`
	WindowEnd               string              `json:"windowEnd"`
	RefreshedAt             string              `json:"refreshedAt"`
	EvidenceVersion         string              `json:"evidenceVersion"`
	Version                 string              `json:"version"`
	PolicyVersion           string              `json:"policyVersion"`
	Recent                  RecentPriceEvidence `json:"recent"`
	Readiness               *Readiness          `json:"readiness,omitempty"`
}

type Evidence struct {
	Evaluation Evaluation `json:"evaluation"`
	Sales      []Sale     `json:"sales"`
}

func Fingerprint(v any) string {
	b, _ := json.Marshal(v) // Internal, finite value types only.
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func Timestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
func Window(now time.Time) (string, string) {
	d := now.UTC()
	return d.AddDate(0, 0, -29).Format(time.DateOnly), d.Format(time.DateOnly)
}
