package inventory

import "math"

// DefaultMarketplaceFeePct is the default fee percentage for eBay (12.35%).
const DefaultMarketplaceFeePct = 0.1235

// DefaultWebsiteFeePct is the fee percentage for website/online store sales (3% credit card processing).
const DefaultWebsiteFeePct = 0.03

// grossModeFee is a sentinel value passed to enrichSellSheetItem to suppress fee
// deduction and return gross (pre-fee) prices. Any negative value would work but
// -1.0 is chosen to be clearly invalid as a real fee percentage.
const grossModeFee = -1.0

// CalculateSaleFee computes marketplace fees for a given channel and sale price.
func CalculateSaleFee(channel SaleChannel, salePriceCents int, campaign *Campaign) int {
	switch NormalizeChannel(channel) {
	case SaleChannelEbay:
		feePct := campaign.EbayFeePct
		if feePct == 0 {
			feePct = DefaultMarketplaceFeePct
		}
		return int(math.Round(float64(salePriceCents) * feePct))
	case SaleChannelWebsite:
		return int(math.Round(float64(salePriceCents) * DefaultWebsiteFeePct))
	default:
		return 0
	}
}

// CalculateNetProfit computes net profit for a sale.
// netProfit = salePrice - buyCost - sourcingFee - saleFee
func CalculateNetProfit(salePriceCents, buyCostCents, sourcingFeeCents, saleFeeCents int) int {
	return salePriceCents - buyCostCents - sourcingFeeCents - saleFeeCents
}

// NormalizeChannel maps legacy channel values to the three active channels.
// Used for display, analytics, and fee calculations. Old DB values are preserved.
func NormalizeChannel(ch SaleChannel) SaleChannel {
	switch ch {
	case SaleChannelEbay, SaleChannelTCGPlayer:
		return SaleChannelEbay
	case SaleChannelWebsite:
		return SaleChannelWebsite
	case SaleChannelInPerson, SaleChannelLocal, SaleChannelOther,
		SaleChannelGameStop, SaleChannelCardShow, SaleChannelDoubleHolo:
		return SaleChannelInPerson
	default:
		return SaleChannelInPerson
	}
}
