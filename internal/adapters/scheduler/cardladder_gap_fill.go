package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

const (
	compRefreshBatchLimit = 200
	compRefreshPause      = 2 * time.Second
)

// refreshSalesCompsDecoupled fetches recent sales comps for unsold purchases
// that have a gem_rate_id, bypassing cl_card_mappings entirely. After fetching,
// it backfills last_sold_date and last_sold_cents on campaign_purchases.
func (s *CardLadderRefreshScheduler) refreshSalesCompsDecoupled(ctx context.Context, client *cardladder.Client) {
	if s.compRefreshStore == nil {
		s.logger.Debug(ctx, "CL sales: compRefreshStore not configured, skipping decoupled refresh")
		return
	}

	cards, err := s.compRefreshStore.ListUnsoldCardsNeedingComps(ctx, 1)
	if err != nil {
		s.logger.Warn(ctx, "CL sales: failed to list cards needing comps", observability.Err(err))
		return
	}

	if len(cards) > compRefreshBatchLimit {
		cards = cards[:compRefreshBatchLimit]
	}

	fetched := 0
	compsUpserted := 0
	upsertFailed := 0
	for _, card := range cards {
		select {
		case <-ctx.Done():
			return
		default:
		}

		apiCondition := cardutil.ConditionToAPIFormat(card.Condition)
		if apiCondition == "" {
			continue
		}
		resp, err := client.FetchSalesComps(ctx, card.GemRateID, apiCondition, "psa", 0, 100)
		if err != nil {
			s.logger.Warn(ctx, "CL sales: fetch failed",
				observability.String("gemRateId", card.GemRateID),
				observability.String("condition", card.Condition),
				observability.Err(err))
			continue
		}

		for _, comp := range resp.Hits {
			priceCents := mathutil.ToCentsInt(comp.Price)
			saleDate := comp.Date
			if len(saleDate) > 10 {
				saleDate = saleDate[:10]
			}
			if err := s.salesStore.UpsertSaleComp(ctx, postgres.CLSaleCompRecord{
				GemRateID:   comp.GemRateID,
				Condition:   card.Condition,
				ItemID:      comp.ItemID,
				SaleDate:    saleDate,
				PriceCents:  priceCents,
				Platform:    comp.Platform,
				ListingType: comp.ListingType,
				Seller:      comp.Seller,
				ItemURL:     comp.URL,
				SlabSerial:  comp.SlabSerial,
			}); err != nil {
				s.logger.Warn(ctx, "CL sales: upsert failed",
					observability.String("itemId", comp.ItemID),
					observability.Err(err))
				upsertFailed++
			} else {
				compsUpserted++
			}
		}
		fetched++

		time.Sleep(compRefreshPause)
	}

	// Backfill last_sold_date and last_sold_cents from comps.
	backfilled, err := s.compRefreshStore.BackfillLastSoldFromComps(ctx)
	if err != nil {
		s.logger.Warn(ctx, "CL sales: backfill last sold failed", observability.Err(err))
	}

	s.logger.Info(ctx, "CL sales: decoupled refresh complete",
		observability.Int("cardsProcessed", fetched),
		observability.Int("compsUpserted", compsUpserted),
		observability.Int("upsertFailed", upsertFailed),
		observability.Int("purchasesBackfilled", backfilled))
}
