package inventory

// Repository is the combined persistence interface for all campaign/inventory data.
// It embeds all focused sub-interfaces. The SQLite adapter implements all of them.
//
// Deprecated: prefer the focused interfaces (CampaignRepository, PurchaseRepository, etc.)
type Repository interface {
	CampaignRepository
	PurchaseRepository
	SaleRepository
	AnalyticsRepository
	FinanceRepository
	PricingRepository
	DHRepository
}
