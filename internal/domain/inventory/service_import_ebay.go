package inventory

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) ImportEbayOrdersSales(ctx context.Context, rows []EbayOrderRow) (*OrdersImportResult, error) {
	// Split rows by lookup type
	var dhIDs []int
	var certNumbers []string
	for _, r := range rows {
		if r.DHInventoryID != 0 {
			dhIDs = append(dhIDs, r.DHInventoryID)
		}
		if r.CertNumber != "" {
			certNumbers = append(certNumbers, r.CertNumber)
		}
	}

	dhMap := make(map[int]*Purchase)
	certMap := make(map[string]*Purchase)

	if len(dhIDs) > 0 {
		var err error
		dhMap, err = s.purchases.GetPurchasesByDHInventoryIDs(ctx, dhIDs)
		if err != nil {
			return nil, fmt.Errorf("batch DH inventory lookup failed: %w", err)
		}
	}
	if len(certNumbers) > 0 {
		var err error
		certMap, err = s.purchases.GetPurchasesByCertNumbers(ctx, certNumbers)
		if err != nil {
			return nil, fmt.Errorf("batch cert lookup failed: %w", err)
		}
	}

	result := &OrdersImportResult{
		Matched:     []OrdersImportMatch{},
		AlreadySold: []OrdersImportSkip{},
		NotFound:    []OrdersImportSkip{},
		Skipped:     []OrdersImportSkip{},
	}

	seen := make(map[string]bool)

	for _, r := range rows {
		var purchase *Purchase
		var certNumber string

		if r.DHInventoryID != 0 {
			purchase = dhMap[r.DHInventoryID]
			if purchase != nil {
				certNumber = purchase.CertNumber
			}
		} else if r.CertNumber != "" {
			purchase = certMap[r.CertNumber]
			certNumber = r.CertNumber
		}

		if purchase == nil {
			result.NotFound = append(result.NotFound, OrdersImportSkip{
				CertNumber:   certNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "not_found",
			})
			continue
		}

		if seen[purchase.ID] {
			result.Skipped = append(result.Skipped, OrdersImportSkip{
				CertNumber:   certNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "duplicate",
			})
			continue
		}
		seen[purchase.ID] = true

		existingSale, saleErr := s.sales.GetSaleByPurchaseID(ctx, purchase.ID)
		if saleErr != nil && !errors.HasErrorCode(saleErr, ErrCodeSaleNotFound) {
			result.NotFound = append(result.NotFound, OrdersImportSkip{
				CertNumber:   certNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "lookup_error",
			})
			continue
		}
		if existingSale != nil {
			result.AlreadySold = append(result.AlreadySold, OrdersImportSkip{
				CertNumber:   certNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "already_sold",
			})
			continue
		}

		var campaignLookupFailed bool
		campaign, err := s.campaigns.GetCampaign(ctx, purchase.CampaignID)
		if err != nil {
			if s.logger != nil {
				s.logger.Error(ctx, "campaign lookup failed for eBay import preview",
					observability.String("campaignID", purchase.CampaignID),
					observability.Err(err))
			}
			campaign = &Campaign{}
			campaignLookupFailed = true
		}

		saleFeeCents := CalculateSaleFee(SaleChannelEbay, r.SalePriceCents, campaign)
		netProfit := CalculateNetProfit(r.SalePriceCents, purchase.BuyCostCents, purchase.PSASourcingFeeCents, saleFeeCents)

		result.Matched = append(result.Matched, OrdersImportMatch{
			CertNumber:           purchase.CertNumber,
			ProductTitle:         r.ProductTitle,
			SaleChannel:          SaleChannelEbay,
			SaleDate:             r.Date,
			SalePriceCents:       r.SalePriceCents,
			SaleFeeCents:         saleFeeCents,
			PurchaseID:           purchase.ID,
			CampaignID:           purchase.CampaignID,
			CardName:             purchase.CardName,
			BuyCostCents:         purchase.BuyCostCents,
			NetProfitCents:       netProfit,
			CampaignLookupFailed: campaignLookupFailed,
		})
	}

	return result, nil
}
