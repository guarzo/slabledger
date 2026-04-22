package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// refreshSalesComps fetches recent completed sales for every unique
// MM collectible ID in the provided mappings and upserts them into the
// MM sales comps store.
func (s *MarketMoversRefreshScheduler) refreshSalesComps(ctx context.Context, client *marketmovers.Client, mappings map[string]postgres.MMCardMapping) {
	seen := make(map[int64]bool, len(mappings))
	fetched := 0

	for _, m := range mappings {
		if m.MMCollectibleID == 0 {
			continue
		}
		if seen[m.MMCollectibleID] {
			continue
		}
		seen[m.MMCollectibleID] = true

		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := client.FetchCompletedSales(ctx, m.MMCollectibleID, 0, 100)
		if err != nil {
			s.logger.Warn(ctx, "MM sales: fetch failed",
				observability.Int64("collectibleId", m.MMCollectibleID),
				observability.Err(err))
			continue
		}

		for _, sale := range resp.Items {
			priceCents := mathutil.ToCentsInt(sale.FinalPrice)
			saleDate := parseSaleDate(sale.SaleDate)
			if saleDate == "" {
				s.logger.Debug(ctx, "MM sales: unparseable date, skipping",
					observability.Int64("saleId", sale.SaleID),
					observability.String("raw", sale.SaleDate))
				continue
			}
			if err := s.salesStore.UpsertSaleComp(ctx, postgres.MMSaleCompRecord{
				MMCollectibleID: m.MMCollectibleID,
				SaleID:          sale.SaleID,
				SaleDate:        saleDate,
				PriceCents:      priceCents,
				Platform:        sale.SalePlatform,
				ListingType:     sale.ListingType,
				Seller:          sale.SellerName,
				SaleURL:         sale.SaleURL,
			}); err != nil {
				s.logger.Debug(ctx, "MM sales: upsert failed",
					observability.Int64("saleId", sale.SaleID),
					observability.Err(err))
			}
		}
		fetched++
	}

	s.logger.Info(ctx, "MM sales: refresh complete",
		observability.Int("cardsProcessed", fetched))
}

func parseSaleDate(raw string) string {
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}
