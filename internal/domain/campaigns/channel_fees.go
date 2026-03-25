package campaigns

import "math"

// DefaultMarketplaceFeePct is the default fee percentage for eBay and TCGPlayer (12.35%).
const DefaultMarketplaceFeePct = 0.1235

// grossModeFee signals enrichSellSheetItem to skip fee deduction, returning gross prices.
const grossModeFee = -1.0

// GameStop payout range as a percentage of CL value (70-90%).
const (
	GameStopPayoutMinPct = 0.70
	GameStopPayoutMaxPct = 0.90
)

// CalculateSaleFee computes marketplace fees for a given channel and sale price.
func CalculateSaleFee(channel SaleChannel, salePriceCents int, campaign *Campaign) int {
	switch channel {
	case SaleChannelEbay, SaleChannelTCGPlayer:
		feePct := campaign.EbayFeePct
		if feePct == 0 {
			feePct = DefaultMarketplaceFeePct
		}
		return int(math.Round(float64(salePriceCents) * feePct))
	case SaleChannelLocal, SaleChannelOther, SaleChannelGameStop, SaleChannelCardShow, SaleChannelWebsite:
		return 0
	default:
		return 0
	}
}

// CalculateNetProfit computes net profit for a sale.
// netProfit = salePrice - buyCost - sourcingFee - saleFee
func CalculateNetProfit(salePriceCents, buyCostCents, sourcingFeeCents, saleFeeCents int) int {
	return salePriceCents - buyCostCents - sourcingFeeCents - saleFeeCents
}
