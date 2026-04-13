package constants

// SaleChannel represents where a card was sold.
type SaleChannel string

const (
	SaleChannelEbay     SaleChannel = "ebay"
	SaleChannelWebsite  SaleChannel = "website"
	SaleChannelInPerson SaleChannel = "inperson"
)

// Legacy channel values — kept for backward-compatible DB reads.
const (
	SaleChannelTCGPlayer  SaleChannel = "tcgplayer"
	SaleChannelLocal      SaleChannel = "local"
	SaleChannelOther      SaleChannel = "other"
	SaleChannelGameStop   SaleChannel = "gamestop"
	SaleChannelCardShow   SaleChannel = "cardshow"
	SaleChannelDoubleHolo SaleChannel = "doubleholo"
)

// DefaultMarketplaceFeePct is the default eBay/TCGPlayer fee percentage (12.35%).
const DefaultMarketplaceFeePct = 0.1235
