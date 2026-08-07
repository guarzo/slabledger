package inventory

// Attribution source values for Purchase.AttributionSource. Every write path
// must set one; a null value after migration 000023 is a defect, not a state.
const (
	// AttributionSourcePSA means PSA's own campaign name resolved to this campaign.
	AttributionSourcePSA = "psa"
	// AttributionSourceInferred means FindMatchingCampaign chose this campaign.
	AttributionSourceInferred = "inferred"
	// AttributionSourceManual means an operator assigned this campaign by hand.
	AttributionSourceManual = "manual"
)

// Reattribution carries a PSA-authoritative campaign correction for an unsold
// purchase. CLConfidenceAtPurchase is nil when the value cannot be vouched for
// and must be stored as NULL rather than guessed.
type Reattribution struct {
	CampaignID             string
	PSACampaignName        string
	PSASourcingFeeCents    int
	CLConfidenceAtPurchase *int
}
