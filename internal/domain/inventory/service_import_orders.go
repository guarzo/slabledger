package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) ImportOrdersSales(ctx context.Context, rows []OrdersExportRow) (*OrdersImportResult, error) {
	// Collect all cert numbers for batch lookup
	certs := make([]string, 0, len(rows))
	for _, r := range rows {
		certs = append(certs, r.CertNumber)
	}

	purchaseMap, err := s.purchases.GetPurchasesByCertNumbers(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("batch cert lookup failed: %w", err)
	}

	result := &OrdersImportResult{
		Matched:     []OrdersImportMatch{},
		AlreadySold: []OrdersImportSkip{},
		NotFound:    []OrdersImportSkip{},
		Skipped:     []OrdersImportSkip{},
	}

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
		existingSale, saleErr := s.sales.GetSaleByPurchaseID(ctx, purchase.ID)
		if saleErr != nil && !errors.HasErrorCode(saleErr, ErrCodeSaleNotFound) {
			// Unexpected DB error — skip to avoid potential duplicate sales
			result.NotFound = append(result.NotFound, OrdersImportSkip{
				CertNumber:   r.CertNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "lookup_error",
			})
			continue
		}
		if existingSale != nil {
			result.AlreadySold = append(result.AlreadySold, OrdersImportSkip{
				CertNumber:   r.CertNumber,
				ProductTitle: r.ProductTitle,
				Reason:       "already_sold",
			})
			continue
		}

		// Compute fee and net profit preview
		salePriceCents := mathutil.ToCentsInt(r.UnitPrice)

		var campaignLookupFailed bool
		campaign, err := s.campaigns.GetCampaign(ctx, purchase.CampaignID)
		if err != nil {
			if s.logger != nil {
				s.logger.Error(ctx, "campaign lookup failed for import preview, fees are estimated",
					observability.String("campaignID", purchase.CampaignID),
					observability.Err(err))
			}
			// Use a zero-fee campaign as fallback
			campaign = &Campaign{}
			campaignLookupFailed = true
		}

		saleFeeCents := CalculateSaleFee(r.SalesChannel, salePriceCents, campaign)
		netProfit := CalculateNetProfit(salePriceCents, purchase.BuyCostCents, purchase.PSASourcingFeeCents, saleFeeCents)

		result.Matched = append(result.Matched, OrdersImportMatch{
			CertNumber:           r.CertNumber,
			ProductTitle:         r.ProductTitle,
			SaleChannel:          r.SalesChannel,
			SaleDate:             r.Date,
			SalePriceCents:       salePriceCents,
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

func (s *service) ConfirmOrdersSales(ctx context.Context, items []OrdersConfirmItem) (*BulkSaleResult, error) {
	result := &BulkSaleResult{}

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.PurchaseID)
	}
	purchaseMap, err := s.purchases.GetPurchasesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load purchases: %w", err)
	}
	saleMap, err := s.sales.GetSalesByPurchaseIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load existing sales: %w", err)
	}
	allCampaigns, err := s.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	campaignMap := make(map[string]*Campaign, len(allCampaigns))
	for i := range allCampaigns {
		campaignMap[allCampaigns[i].ID] = &allCampaigns[i]
	}

	for _, item := range items {
		purchase, ok := purchaseMap[item.PurchaseID]
		if !ok {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: "purchase not found"})
			continue
		}

		// Check for existing sale to prevent duplicates (e.g. double-submit)
		if _, alreadySold := saleMap[item.PurchaseID]; alreadySold {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: "already sold"})
			continue
		}

		campaign, ok := campaignMap[purchase.CampaignID]
		if !ok {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: "campaign not found"})
			continue
		}

		// Create sale inline without captureMarketSnapshot to avoid hitting
		// external pricing APIs for every card (which causes timeouts on bulk imports).
		// The purchase already has a market snapshot from when it was created.
		sa := &Sale{
			PurchaseID:     item.PurchaseID,
			SaleChannel:    item.SaleChannel,
			SalePriceCents: item.SalePriceCents,
			SaleDate:       item.SaleDate,
			OrderID:        item.OrderID,
		}

		if err := ValidateSale(sa); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: err.Error()})
			continue
		}

		sa.ID = s.idGen()
		sa.SaleFeeCents = CalculateSaleFee(sa.SaleChannel, sa.SalePriceCents, campaign)

		purchaseDate, parseErr := time.Parse("2006-01-02", purchase.PurchaseDate)
		if parseErr == nil {
			saleDate, parseErr2 := time.Parse("2006-01-02", sa.SaleDate)
			if parseErr2 == nil {
				if saleDate.Before(purchaseDate) {
					result.Failed++
					result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: ErrSaleDateBeforePurchase.Error()})
					continue
				}
				sa.DaysToSell = int(saleDate.Sub(purchaseDate).Hours() / 24)
			}
		}

		sa.NetProfitCents = CalculateNetProfit(sa.SalePriceCents, purchase.BuyCostCents, purchase.PSASourcingFeeCents, sa.SaleFeeCents)

		now := time.Now()
		sa.CreatedAt = now
		sa.UpdatedAt = now

		if err := s.sales.CreateSale(ctx, sa); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: err.Error()})
			continue
		}
		// Mark this purchase as sold within this batch so a duplicate item.PurchaseID
		// later in the loop is rejected by the saleMap guard rather than racing into
		// CreateSale and tripping the DB UNIQUE constraint.
		saleMap[item.PurchaseID] = sa

		// Flip local dh_status to 'sold' so the inventory UI reflects reality.
		// Best-effort: a failure logs Error but does not roll back the sale.
		// Only update for items that had DH state (DHInventoryID != 0) — items
		// with no DH history are usually eBay-CSV imports and shouldn't be flagged
		// as DH-sold.
		if purchase.DHInventoryID != 0 {
			if err := s.purchases.UpdatePurchaseDHStatus(ctx, purchase.ID, string(DHStatusSold)); err != nil {
				if s.logger != nil {
					s.logger.Error(ctx, "confirm sales: failed to update dh_status to sold",
						observability.String("purchaseID", purchase.ID),
						observability.Err(err))
				}
			}
		}

		// Notify DH that this item has sold so it is retired on their platform.
		// This is best-effort: a failure does not roll back the local sale record.
		if s.dhSoldNotifier != nil && purchase.DHInventoryID != 0 {
			if err := s.dhSoldNotifier.MarkInventorySold(ctx, purchase.DHInventoryID); err != nil {
				if s.logger != nil {
					s.logger.Warn(ctx, "confirm sales: failed to mark DH inventory as sold",
						observability.String("purchaseID", purchase.ID),
						observability.Int("dhInventoryID", purchase.DHInventoryID),
						observability.Err(err))
				}
			}
		}

		result.Created++
	}

	return result, nil
}
