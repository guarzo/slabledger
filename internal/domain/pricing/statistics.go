package pricing

// SaleRecord represents a single sale with grade and price information,
// used as input to CalculateConservativeExits and CalculateLastSoldByGrade.
type SaleRecord struct {
	PriceCents int
	Date       string
	Grade      string
}

// ConservativeExits holds conservative and optimistic exit prices
// computed from recent sales data.
type ConservativeExits struct {
	ConservativePSA10USD float64
	ConservativePSA9USD  float64
	OptimisticRawUSD     float64
	PSA10Distribution    *SalesDistribution
	PSA9Distribution     *SalesDistribution
	RawDistribution      *SalesDistribution
}
