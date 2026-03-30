package pricecharting

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
	"github.com/guarzo/slabledger/internal/domain/pricing/analysis"
)

// saleRecordsFromRecentSales converts adapter SaleData to domain SaleRecord.
func saleRecordsFromRecentSales(sales []SaleData) []pricing.SaleRecord {
	records := make([]pricing.SaleRecord, len(sales))
	for i, sale := range sales {
		records[i] = pricing.SaleRecord{
			PriceCents: sale.PriceCents,
			Date:       sale.Date,
			Grade:      sale.Grade,
		}
	}
	return records
}

// applyConservativeExits uses domain pricing statistics to compute conservative exit prices.
func (p *PriceCharting) applyConservativeExits(ctx context.Context, match *PCMatch) {
	if match == nil || len(match.RecentSales) == 0 {
		return
	}

	records := saleRecordsFromRecentSales(match.RecentSales)

	exits := analysis.CalculateConservativeExits(records, analysis.MinSalesThreshold, pricing.SourcePriceCharting)
	if exits == nil {
		return
	}

	match.ConservativePSA10USD = exits.ConservativePSA10USD
	match.ConservativePSA9USD = exits.ConservativePSA9USD
	match.OptimisticRawUSD = exits.OptimisticRawUSD
	match.PSA10Distribution = exits.PSA10Distribution
	match.PSA9Distribution = exits.PSA9Distribution
	match.RawDistribution = exits.RawDistribution

	if p.logger != nil && exits.PSA10Distribution != nil {
		p.logger.Debug(ctx, "calculated conservative exit prices",
			observability.String("product", match.ProductName),
			observability.Float64("psa10_p25_usd", exits.ConservativePSA10USD),
		)
	}
}

// applyLastSoldByGrade uses domain pricing statistics to extract last sold data per grade.
func (p *PriceCharting) applyLastSoldByGrade(_ context.Context, match *PCMatch) {
	if match == nil || len(match.RecentSales) == 0 {
		return
	}

	match.LastSoldByGrade = analysis.CalculateLastSoldByGrade(saleRecordsFromRecentSales(match.RecentSales))
}

// enrichMatch applies conservative exit prices and last-sold-by-grade
// to a matched result. Shared by tryAPI and tryFuzzy.
func (p *PriceCharting) enrichMatch(ctx context.Context, match *PCMatch) {
	p.applyConservativeExits(ctx, match)
	p.applyLastSoldByGrade(ctx, match)
}
