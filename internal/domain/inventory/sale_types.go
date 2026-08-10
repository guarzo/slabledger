package inventory

import "time"

// Sale represents the sale of a purchased card.
type Sale struct {
	ID             string      `json:"id"`
	PurchaseID     string      `json:"purchaseId"` // FK to Purchase (unique — one sale per purchase)
	SaleChannel    SaleChannel `json:"saleChannel"`
	SalePriceCents int         `json:"salePriceCents"`
	SaleFeeCents   int         `json:"saleFeeCents"`   // Marketplace fees in cents
	SaleDate       string      `json:"saleDate"`       // YYYY-MM-DD
	DaysToSell     int         `json:"daysToSell"`     // Computed: saleDate - purchaseDate
	NetProfitCents int         `json:"netProfitCents"` // Computed: salePrice - buyCost - sourcingFee - saleFee
	CreatedAt      time.Time   `json:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
	// DoubleHolo v2 order tracking
	OrderID string `json:"orderId,omitempty"` // DH order_id for idempotency

	// Sale outcome enrichment — captures HOW the card sold
	OriginalListPriceCents int  `json:"originalListPriceCents,omitempty"` // Initial listing price
	PriceReductions        int  `json:"priceReductions,omitempty"`        // Number of price drops before sale
	DaysListed             int  `json:"daysListed,omitempty"`             // Days on the listing platform
	SoldAtAskingPrice      bool `json:"soldAtAskingPrice,omitempty"`      // Sold at the listed price

	// Crack slab tracking — indicates the card was cracked from its slab and sold raw
	WasCracked bool `json:"wasCracked,omitempty"`

	// ForcedLiquidation indicates this sale was driven by invoice timing pressure.
	// App-maintained (not a generated/computed column): kept in sync with SaleReason
	// by freezeSaleProvenance whenever a sale is created.
	ForcedLiquidation bool `json:"forcedLiquidation"`

	// SaleReason records why this sale happened (discretionary, invoice_pressure,
	// aging_policy, bulk_lot, show_clearout). Client-supplied when valid; defaulted
	// via heuristic when empty. Frozen at sale-creation time.
	SaleReason string `json:"saleReason,omitempty"`

	// TheirCompCents is the counterparty's own per-card comp (e.g. a dealer's
	// sticker/offer price), captured independently of any CL/DH market signal.
	// PriceSource records how the sale price was arrived at: '' (unknown),
	// 'itemized', 'estimated', or 'manual'. See migration 000040.
	TheirCompCents int    `json:"theirCompCents,omitempty"`
	PriceSource    string `json:"priceSource,omitempty"`

	// CLValueAtSaleCents and ChannelFeePctAtSale freeze the purchase's CL value and
	// the channel fee rate in effect at sale time.
	//
	// The two use different "unknown" encodings, mirroring the DB: cl_value_at_sale_cents
	// is NOT NULL DEFAULT 0, so 0 is ambiguous for sales predating migration 000022
	// (genuinely-zero vs never-recorded are indistinguishable); channel_fee_pct_at_sale
	// is nullable, so nil is unambiguous. This is why the sale side does not use a
	// pointer the way the purchase-side *AtPurchase fields do.
	CLValueAtSaleCents  int      `json:"clValueAtSaleCents,omitempty"`
	ChannelFeePctAtSale *float64 `json:"channelFeePctAtSale,omitempty"`

	// CLValueAtSaleObservedAt/CLValueAtSaleSource are the sale-side counterpart
	// of Purchase.CLValueAtPurchaseObservedAt/Source: they record whether
	// CLValueAtSaleCents is a genuine CardLadder observation at sale time or
	// merely the purchase's carried-over value. See migration 000041 and
	// CLProvenanceSourceIntake/CLProvenanceSourceCardLadder in core_types.go.
	// "" for both means this sale predates the column (provenance unknown).
	CLValueAtSaleObservedAt string `json:"clValueAtSaleObservedAt,omitempty"`
	CLValueAtSaleSource     string `json:"clValueAtSaleSource,omitempty"`

	// Market snapshot at time of sale (best-effort, may be zero)
	MarketSnapshotData
}

// BulkSaleInput represents a single item in a bulk sale request.
type BulkSaleInput struct {
	PurchaseID             string `json:"purchaseId"`
	SalePriceCents         int    `json:"salePriceCents"`
	OriginalListPriceCents int    `json:"originalListPriceCents,omitempty"`
	PriceReductions        int    `json:"priceReductions,omitempty"`
	DaysListed             int    `json:"daysListed,omitempty"`
	SaleReason             string `json:"saleReason,omitempty"`
	PriceSource            string `json:"priceSource,omitempty"`
}

// BulkSaleResult summarizes the outcome of a bulk sale operation.
type BulkSaleResult struct {
	Created int             `json:"created"`
	Failed  int             `json:"failed"`
	Errors  []BulkSaleError `json:"errors,omitempty"`
}

// BulkSaleError describes a failure for a single item in a bulk sale.
type BulkSaleError struct {
	PurchaseID string `json:"purchaseId"`
	Error      string `json:"error"`
}
