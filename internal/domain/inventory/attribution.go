package inventory

// Attribution source values for Purchase.AttributionSource. Every write path
// must set one. A NULL in the column is not a reliable detector for a write
// path that forgot: PurchaseStore.CreatePurchase defaults an empty value to
// 'inferred', so only writes that bypass that store method can leave a NULL.
const (
	// AttributionSourcePSA means PSA's own campaign name resolved to this campaign.
	AttributionSourcePSA = "psa"
	// AttributionSourceInferred means FindMatchingCampaign chose this campaign.
	AttributionSourceInferred = "inferred"
	// AttributionSourceManual means an operator assigned this campaign by hand.
	AttributionSourceManual = "manual"
)

// Reattribution carries a PSA-authoritative campaign correction for an unsold
// purchase. CLPolicyConfidenceMinAtPurchase is nil when the value cannot be
// vouched for and must be stored as NULL rather than guessed.
type Reattribution struct {
	CampaignID                      string
	PSACampaignName                 string
	PSASourcingFeeCents             int
	CLPolicyConfidenceMinAtPurchase *int
}
