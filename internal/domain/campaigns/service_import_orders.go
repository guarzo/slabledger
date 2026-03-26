package campaigns

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) ImportOrdersSales(ctx context.Context, rows []OrdersExportRow) (*OrdersImportResult, error) {
	// Collect all cert numbers for batch lookup
	certs := make([]string, 0, len(rows))
	for _, r := range rows {
		certs = append(certs, r.CertNumber)
	}

	purchaseMap, err := s.repo.GetPurchasesByCertNumbers(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("batch cert lookup failed: %w", err)
	}

	result := &OrdersImportResult{}

	for _, r := range rows {
		purchase, found := purchaseMap[r.CertNumber]
		if !found {
			result.NotFound = append(result.NotFound, OrdersImportSkip{
				CertNumber:   r.CertNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "not_found",
			})
			continue
		}

		// Check if already sold
		existingSale, _ := s.repo.GetSaleByPurchaseID(ctx, purchase.ID)
		if existingSale != nil {
			result.AlreadySold = append(result.AlreadySold, OrdersImportSkip{
				CertNumber:   r.CertNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "already_sold",
			})
			continue
		}

		// Compute fee and net profit preview
		salePriceCents := DollarsToCents(r.UnitPrice)

		campaign, err := s.repo.GetCampaign(ctx, purchase.CampaignID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "campaign lookup failed for import preview",
					observability.String("campaignID", purchase.CampaignID),
					observability.Err(err))
			}
			// Use a zero-fee campaign as fallback
			campaign = &Campaign{}
		}

		saleFeeCents := CalculateSaleFee(r.SalesChannel, salePriceCents, campaign)
		netProfit := CalculateNetProfit(salePriceCents, purchase.BuyCostCents, purchase.PSASourcingFeeCents, saleFeeCents)

		result.Matched = append(result.Matched, OrdersImportMatch{
			CertNumber:     r.CertNumber,
			ProductTitle:   r.ProductTitle,
			SaleChannel:    r.SalesChannel,
			SaleDate:       r.Date,
			SalePriceCents: salePriceCents,
			SaleFeeCents:   saleFeeCents,
			PurchaseID:     purchase.ID,
			CampaignID:     purchase.CampaignID,
			CardName:       purchase.CardName,
			BuyCostCents:   purchase.BuyCostCents,
			NetProfitCents: netProfit,
		})
	}

	return result, nil
}

func (s *service) ConfirmOrdersSales(ctx context.Context, items []OrdersConfirmItem) (*BulkSaleResult, error) {
	result := &BulkSaleResult{}

	for _, item := range items {
		purchase, err := s.repo.GetPurchase(ctx, item.PurchaseID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: "purchase not found"})
			continue
		}

		campaign, err := s.repo.GetCampaign(ctx, purchase.CampaignID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: "campaign not found"})
			continue
		}

		sa := &Sale{
			PurchaseID:     item.PurchaseID,
			SaleChannel:    item.SaleChannel,
			SalePriceCents: item.SalePriceCents,
			SaleDate:       item.SaleDate,
		}

		if err := s.CreateSale(ctx, sa, campaign, purchase); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: err.Error()})
			continue
		}
		result.Created++
	}

	return result, nil
}
