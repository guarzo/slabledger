package liquidation

import (
	"context"
	"fmt"
	"math"
	"strings"
)

type service struct {
	purchases PurchaseLister
	comps     CompReader
	prices    PriceWriter
}

// NewService constructs a liquidation Service.
func NewService(purchases PurchaseLister, comps CompReader, prices PriceWriter) Service {
	return &service{purchases: purchases, comps: comps, prices: prices}
}

func (s *service) Preview(ctx context.Context, req PreviewRequest) (PreviewResponse, error) {
	unsold, err := s.purchases.ListUnsoldForLiquidation(ctx)
	if err != nil {
		return PreviewResponse{}, err
	}

	var items []PreviewItem
	var summary PreviewSummary

	for _, p := range unsold {
		item := PreviewItem{
			PurchaseID:                p.ID,
			CertNumber:                p.CertNumber,
			CardName:                  p.CardName,
			Grade:                     p.GradeValue,
			CampaignName:              p.CampaignName,
			BuyCostCents:              p.BuyCostCents,
			CLValueCents:              p.CLValueCents,
			CurrentReviewedPriceCents: p.ReviewedPriceCents,
		}

		if p.GemRateID != "" {
			condition := gradeToCondition(p.GradeValue)
			saleComps, compErr := s.comps.GetSaleCompsForCard(ctx, p.GemRateID, condition)
			if compErr == nil && len(saleComps) > 0 {
				result := ComputeCompPrice(saleComps, p.CLValueCents)
				item.CompPriceCents = result.CompPriceCents
				item.CompCount = result.CompCount
				item.MostRecentCompDate = result.MostRecentCompDate
				item.ConfidenceLevel = result.ConfidenceLevel
				item.GapPct = result.GapPct
				item.SuggestedPriceCents = applyDiscount(p.CLValueCents, discountWithComps)
				if item.SuggestedPriceCents < p.BuyCostCents {
					item.BelowCost = true
					summary.BelowCostCount++
				}
				summary.WithComps++
				summary.TotalSuggestedValueCents += item.SuggestedPriceCents
			} else {
				if p.CLValueCents > 0 {
					item.SuggestedPriceCents = applyDiscount(p.CLValueCents, discountNoComps)
					if item.SuggestedPriceCents < p.BuyCostCents {
						item.BelowCost = true
						summary.BelowCostCount++
					}
					summary.WithoutComps++
					summary.TotalSuggestedValueCents += item.SuggestedPriceCents
				} else {
					summary.NoData++
				}
			}
		} else if p.CLValueCents > 0 {
			item.SuggestedPriceCents = applyDiscount(p.CLValueCents, discountNoComps)
			if item.SuggestedPriceCents < p.BuyCostCents {
				item.BelowCost = true
				summary.BelowCostCount++
			}
			summary.WithoutComps++
			summary.TotalSuggestedValueCents += item.SuggestedPriceCents
		} else {
			summary.NoData++
		}

		summary.TotalCards++
		summary.TotalCurrentValueCents += p.ReviewedPriceCents
		items = append(items, item)
	}

	return PreviewResponse{Items: items, Summary: summary}, nil
}

func (s *service) Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error) {
	var result ApplyResult
	for _, item := range req.Items {
		if err := s.prices.SetReviewedPrice(ctx, item.PurchaseID, item.NewPriceCents, "liquidation"); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("purchase %s: %v", item.PurchaseID, err))
		} else {
			result.Applied++
		}
	}
	return result, nil
}

func applyDiscount(priceCents int, discountPct float64) int {
	return int(float64(priceCents) * (1 - discountPct/100))
}

const (
	discountWithComps = 2.5
	discountNoComps   = 10.0
)

// gradeToCondition converts a numeric grade to the CL sales comp condition
// format stored in cl_sales_comps (e.g. 10 → "g10", 8.5 → "g8_5").
func gradeToCondition(grade float64) string {
	if grade == math.Trunc(grade) {
		return fmt.Sprintf("g%d", int(grade))
	}
	s := fmt.Sprintf("g%g", grade)
	return strings.ReplaceAll(s, ".", "_")
}
