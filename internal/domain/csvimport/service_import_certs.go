package csvimport

import (
	"context"
	"fmt"
	"math"

	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ImportCertSales previews a card-show batch: a dealer's own per-card comp
// plus one percentage negotiated across the whole deal. It never persists —
// the server computes SalePriceCents = round(TheirCompCents * NegotiatedPct)
// per item so the exact price the skill never computed is derived here, once,
// under one negotiated rate.
func (s *service) ImportCertSales(ctx context.Context, req CertSaleImportRequest) (*CertSaleImportResult, error) {
	if req.NegotiatedPct <= 0 || req.NegotiatedPct > 1 {
		return nil, errors.NewAppError(errors.ErrCodeValidation,
			fmt.Sprintf("negotiatedPct must be in (0, 1], got %v", req.NegotiatedPct))
	}
	if req.SaleDate == "" {
		return nil, errors.NewAppError(errors.ErrCodeValidation, "saleDate is required")
	}
	if len(req.Items) == 0 {
		return nil, errors.NewAppError(errors.ErrCodeValidation, "items must not be empty")
	}

	certs := make([]string, 0, len(req.Items))
	for _, item := range req.Items {
		certs = append(certs, item.CertNumber)
	}
	purchaseMap, err := s.purchases.GetPurchasesByCertNumbers(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("batch cert lookup failed: %w", err)
	}

	result := &CertSaleImportResult{
		Matched:            []CertSaleMatch{},
		AlreadySold:        []OrdersImportSkip{},
		NotFound:           []OrdersImportSkip{},
		NearDuplicateCerts: nearDuplicateCerts(certs),
	}

	for _, item := range req.Items {
		if item.TheirCompCents <= 0 {
			result.NotFound = append(result.NotFound, OrdersImportSkip{
				CertNumber: item.CertNumber,
				Reason:     "invalid_comp",
			})
			continue
		}

		purchase, found := purchaseMap[item.CertNumber]
		if !found {
			result.NotFound = append(result.NotFound, OrdersImportSkip{
				CertNumber: item.CertNumber,
				Reason:     "not_found",
			})
			continue
		}

		existingSale, saleErr := s.sales.GetSaleByPurchaseID(ctx, purchase.ID)
		if saleErr != nil && !errors.HasErrorCode(saleErr, inventory.ErrCodeSaleNotFound) {
			result.NotFound = append(result.NotFound, OrdersImportSkip{
				CertNumber: item.CertNumber,
				Reason:     "lookup_error",
			})
			continue
		}
		if existingSale != nil {
			result.AlreadySold = append(result.AlreadySold, OrdersImportSkip{
				CertNumber: item.CertNumber,
				Reason:     "already_sold",
			})
			continue
		}

		salePriceCents := int(math.Round(float64(item.TheirCompCents) * req.NegotiatedPct))

		campaign, err := s.campaigns.GetCampaign(ctx, purchase.CampaignID)
		if err != nil {
			if s.logger != nil {
				s.logger.Error(ctx, "campaign lookup failed for cert-sale preview, fees are estimated",
					observability.String("campaignID", purchase.CampaignID),
					observability.Err(err))
			}
			campaign = &inventory.Campaign{}
		}

		saleFeeCents := inventory.CalculateSaleFee(req.SaleChannel, salePriceCents, campaign)
		netProfit := inventory.CalculateNetProfit(salePriceCents, purchase.BuyCostCents, purchase.PSASourcingFeeCents, saleFeeCents)

		result.Matched = append(result.Matched, CertSaleMatch{
			CertNumber:     item.CertNumber,
			PurchaseID:     purchase.ID,
			CampaignID:     purchase.CampaignID,
			CardName:       purchase.CardName,
			TheirCompCents: item.TheirCompCents,
			SalePriceCents: salePriceCents,
			SaleFeeCents:   saleFeeCents,
			CLValueCents:   purchase.CLValueCents,
			BuyCostCents:   purchase.BuyCostCents,
			NetProfitCents: netProfit,
		})
		result.ComputedTotalCents += salePriceCents
	}

	if req.TotalReceivedCents > 0 {
		result.ReconcileDeltaCents = result.ComputedTotalCents - req.TotalReceivedCents
	}

	return result, nil
}

// nearDuplicateCerts returns every cert in certs that is within edit distance
// 1 of another cert in the same batch: OCR misreading one digit either fails
// to match anything (self-flagging, harmless) or silently matches a
// DIFFERENT owned card (silent corruption — the real risk this guards
// against). O(n^2) over the batch; a card-show batch is ~200 items, so this
// does not need a trie.
func nearDuplicateCerts(certs []string) []string {
	flagged := make(map[string]bool)
	for i := 0; i < len(certs); i++ {
		for j := i + 1; j < len(certs); j++ {
			if certs[i] == certs[j] {
				continue // exact duplicates are a separate concern, not a misread
			}
			if withinOneEdit(certs[i], certs[j]) {
				flagged[certs[i]] = true
				flagged[certs[j]] = true
			}
		}
	}
	if len(flagged) == 0 {
		return nil
	}
	out := make([]string, 0, len(flagged))
	for _, c := range certs {
		if flagged[c] {
			out = append(out, c)
		}
	}
	return out
}

// withinOneEdit reports whether a and b differ by exactly one substitution
// (equal length) or exactly one deletion/insertion (lengths differ by one).
func withinOneEdit(a, b string) bool {
	if len(a) == len(b) {
		diffs := 0
		for i := 0; i < len(a); i++ {
			if a[i] != b[i] {
				diffs++
				if diffs > 1 {
					return false
				}
			}
		}
		return diffs == 1
	}

	shorter, longer := a, b
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(longer)-len(shorter) != 1 {
		return false
	}
	// Find the single deletion point in longer that reproduces shorter.
	i, j := 0, 0
	skipped := false
	for i < len(shorter) && j < len(longer) {
		if shorter[i] == longer[j] {
			i++
			j++
			continue
		}
		if skipped {
			return false
		}
		skipped = true
		j++
	}
	return true
}
