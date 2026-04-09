package scheduler

import (
	"context"
	"fmt"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/storage/sqlite"
	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *CardLadderRefreshScheduler) refreshSalesComps(ctx context.Context, mappings []sqlite.CLCardMapping) {
	type compKey struct{ gemRateID, condition string }
	seen := make(map[compKey]bool, len(mappings))
	fetched := 0

	for _, m := range mappings {
		if m.CLGemRateID == "" || m.CLCondition == "" {
			continue
		}

		key := compKey{m.CLGemRateID, m.CLCondition}
		if seen[key] {
			continue
		}
		seen[key] = true

		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := s.client.FetchSalesComps(ctx, m.CLGemRateID, m.CLCondition, "psa", 0, 100)
		if err != nil {
			s.logger.Warn(ctx, "CL sales: fetch failed",
				observability.String("gemRateId", m.CLGemRateID),
				observability.Err(err))
			continue
		}

		for _, comp := range resp.Hits {
			priceCents := mathutil.ToCentsInt(comp.Price)
			saleDate := comp.Date
			if len(saleDate) > 10 {
				saleDate = saleDate[:10]
			}
			if err := s.salesStore.UpsertSaleComp(ctx, sqlite.CLSaleCompRecord{
				GemRateID:   comp.GemRateID,
				Condition:   m.CLCondition,
				ItemID:      comp.ItemID,
				SaleDate:    saleDate,
				PriceCents:  priceCents,
				Platform:    comp.Platform,
				ListingType: comp.ListingType,
				Seller:      comp.Seller,
				ItemURL:     comp.URL,
				SlabSerial:  comp.SlabSerial,
			}); err != nil {
				s.logger.Debug(ctx, "CL sales: upsert failed",
					observability.String("itemId", comp.ItemID),
					observability.Err(err))
			}
		}
		fetched++
	}

	s.logger.Info(ctx, "CL sales: refresh complete",
		observability.Int("cardsProcessed", fetched))
}

// gapFillGemRateIDs queries the CL cards index for purchases that still have no gemRateID.
// Matches on player name + condition + grading company. Rate limited by the client's built-in limiter.
func (s *CardLadderRefreshScheduler) gapFillGemRateIDs(ctx context.Context, purchases []campaigns.Purchase) {
	filled := 0
	for i := range purchases {
		p := &purchases[i]
		if p.GemRateID != "" || p.CardName == "" {
			continue
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		// Build condition label from grade: "PSA 10", "PSA 9", etc.
		grader := p.Grader
		if grader == "" {
			grader = "PSA"
		}
		condition := fmt.Sprintf("%s %s", grader, mathutil.FormatGrade(p.GradeValue))

		filters := map[string]string{
			"condition":      condition,
			"gradingCompany": strings.ToLower(grader),
		}

		resp, err := s.client.FetchCardCatalog(ctx, p.CardName, filters, 0, 5)
		if err != nil {
			s.logger.Warn(ctx, "CL gap-fill: search failed",
				observability.String("card", p.CardName),
				observability.Err(err))
			continue
		}

		if len(resp.Hits) == 0 {
			continue
		}

		// Take the top result — highest score match
		hit := resp.Hits[0]
		if hit.GemRateID == "" {
			continue
		}

		if err := s.gemRateUpdater.UpdatePurchaseGemRateID(ctx, p.ID, hit.GemRateID); err != nil {
			s.logger.Warn(ctx, "CL gap-fill: failed to persist gemRateID",
				observability.String("cert", p.CertNumber),
				observability.Err(err))
			continue
		}

		if hit.PSASpecID != 0 {
			if err := s.gemRateUpdater.UpdatePurchasePSASpecID(ctx, p.ID, hit.PSASpecID); err != nil {
				s.logger.Warn(ctx, "CL gap-fill: failed to persist psaSpecId",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
			}
		}

		// Persist player/variation/category for MM export enrichment.
		if hit.Player != "" || hit.Variation != "" || hit.Category != "" {
			if err := s.gemRateUpdater.UpdatePurchaseCLCardMetadata(ctx, p.ID, hit.Player, hit.Variation, hit.Category); err != nil {
				s.logger.Warn(ctx, "CL gap-fill: failed to persist card metadata",
					observability.String("cert", p.CertNumber),
					observability.Err(err))
			}
		}

		filled++
	}

	if filled > 0 {
		s.logger.Info(ctx, "CL gap-fill: complete",
			observability.Int("filled", filled))
	}
}
