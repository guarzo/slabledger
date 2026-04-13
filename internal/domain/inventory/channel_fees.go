package inventory

import (
	"math"

	"github.com/guarzo/slabledger/internal/domain/constants"
)

// DefaultWebsiteFeePct is the fee percentage for website/online store sales (3% credit card processing).
const DefaultWebsiteFeePct = 0.03

// CalculateSaleFee computes marketplace fees for a given channel and sale price.
func CalculateSaleFee(channel SaleChannel, salePriceCents int, campaign *Campaign) int {
	switch NormalizeChannel(channel) {
	case SaleChannelEbay:
		feePct := campaign.EbayFeePct
		if feePct == 0 {
			feePct = constants.DefaultMarketplaceFeePct
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
