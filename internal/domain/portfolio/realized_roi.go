package portfolio

import "github.com/guarzo/slabledger/internal/domain/inventory"

// RealizedROISince is the first sale date (YYYY-MM-DD, inclusive) counted in
// the dashboard's realized ROI. Earlier sales are excluded so the headline
// reflects the current operating period rather than all-time history.
const RealizedROISince = "2026-08-01"

// computeRealizedROI returns net profit over cost basis for sales dated on or
// after since, limited to purchases in the given campaigns. The external
// (Shopify-imported) campaign is always excluded: it has no real cost basis.
// Cost basis is each sold card's own buy cost plus PSA sourcing fee.
func computeRealizedROI(campaigns []inventory.Campaign, data []inventory.PurchaseWithSale, since string) float64 {
	inScope := make(map[string]bool, len(campaigns))
	for _, c := range campaigns {
		if c.ID != inventory.ExternalCampaignID {
			inScope[c.ID] = true
		}
	}
	costBasis, netProfit := 0, 0
	for _, d := range data {
		// SaleDate is YYYY-MM-DD, so string comparison orders by date.
		if d.Sale == nil || d.Sale.SaleDate < since || !inScope[d.Purchase.CampaignID] {
			continue
		}
		costBasis += d.Purchase.BuyCostCents + d.Purchase.PSASourcingFeeCents
		netProfit += d.Sale.NetProfitCents
	}
	if costBasis == 0 {
		return 0
	}
	return float64(netProfit) / float64(costBasis)
}
