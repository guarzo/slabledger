package inventory

// PurchaseFilter holds optional filtering criteria for GetAllPurchasesWithSales.
type PurchaseFilter struct {
	SinceDate       string // "2025-01-01" or empty for all
	ExcludeArchived bool
	ExcludeExternal bool
}

// PurchaseFilterOpt is a functional option for configuring PurchaseFilter.
type PurchaseFilterOpt func(*PurchaseFilter)

// WithSinceDate returns an option that filters purchases to those on or after the given date (YYYY-MM-DD).
func WithSinceDate(d string) PurchaseFilterOpt {
	return func(f *PurchaseFilter) { f.SinceDate = d }
}

// WithExcludeArchived returns an option that excludes purchases from archived inventory.
func WithExcludeArchived() PurchaseFilterOpt {
	return func(f *PurchaseFilter) { f.ExcludeArchived = true }
}

// WithExcludeExternal returns an option that excludes purchases from the
// external (Shopify-imported) campaign. External purchases have no real cost
// basis, so including them skews any profit/margin calculation.
func WithExcludeExternal() PurchaseFilterOpt {
	return func(f *PurchaseFilter) { f.ExcludeExternal = true }
}
