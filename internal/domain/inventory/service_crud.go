package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) CreateCampaign(ctx context.Context, c *Campaign) error {
	if err := ValidateAndNormalizeCampaign(c); err != nil {
		return err
	}
	if c.ID == "" {
		c.ID = s.idGen()
	}
	if c.Phase == "" {
		c.Phase = PhasePending
	}
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	return s.campaigns.CreateCampaign(ctx, c)
}

func (s *service) GetCampaign(ctx context.Context, id string) (*Campaign, error) {
	return s.campaigns.GetCampaign(ctx, id)
}

func (s *service) ListCampaigns(ctx context.Context, activeOnly bool) ([]Campaign, error) {
	return s.campaigns.ListCampaigns(ctx, activeOnly)
}

func (s *service) UpdateCampaign(ctx context.Context, c *Campaign) error {
	if err := ValidateAndNormalizeCampaign(c); err != nil {
		return err
	}
	c.UpdatedAt = time.Now()
	return s.campaigns.UpdateCampaign(ctx, c)
}

func (s *service) DeleteCampaign(ctx context.Context, id string) error {
	return s.campaigns.DeleteCampaign(ctx, id)
}

func (s *service) DeletePurchase(ctx context.Context, id string) error {
	return s.purchases.DeletePurchase(ctx, id)
}

func (s *service) CreatePurchase(ctx context.Context, p *Purchase) error {
	if err := ValidateAndNormalizePurchase(p); err != nil {
		return err
	}
	if p.ID == "" {
		p.ID = s.idGen()
	}

	// Verify campaign exists
	_, err := s.campaigns.GetCampaign(ctx, p.CampaignID)
	if err != nil {
		return fmt.Errorf("campaign lookup: %w", err)
	}

	// Skip synchronous market snapshot when the caller has flagged the purchase
	// for asynchronous background enrichment (e.g. during bulk PSA import).
	if p.SnapshotStatus != SnapshotStatusPending {
		// Best-effort: capture market snapshot at time of purchase.
		s.captureMarketSnapshot(ctx, p, p.ToCardIdentity(), p.GradeValue, p.CLValueCents)
	}

	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	return s.purchases.CreatePurchase(ctx, p)
}

func (s *service) GetPurchase(ctx context.Context, id string) (*Purchase, error) {
	return s.purchases.GetPurchase(ctx, id)
}

func (s *service) ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Purchase, error) {
	return s.purchases.ListPurchasesByCampaign(ctx, campaignID, limit, offset)
}

func (s *service) CreateSale(ctx context.Context, sa *Sale, campaign *Campaign, purchase *Purchase) error {
	if err := ValidateSale(sa); err != nil {
		return err
	}
	if sa.ID == "" {
		sa.ID = s.idGen()
	}

	sa.SaleFeeCents = CalculateSaleFee(sa.SaleChannel, sa.SalePriceCents, campaign)

	purchaseDate, err := time.Parse("2006-01-02", purchase.PurchaseDate)
	if err == nil {
		saleDate, err2 := time.Parse("2006-01-02", sa.SaleDate)
		if err2 == nil {
			if saleDate.Before(purchaseDate) {
				return ErrSaleDateBeforePurchase
			}
			sa.DaysToSell = int(saleDate.Sub(purchaseDate).Hours() / 24)
		}
	}

	sa.NetProfitCents = CalculateNetProfit(
		sa.SalePriceCents, purchase.BuyCostCents,
		purchase.PSASourcingFeeCents, sa.SaleFeeCents,
	)

	// Best-effort: capture market snapshot at time of sale
	s.captureMarketSnapshot(ctx, sa, purchase.ToCardIdentity(), purchase.GradeValue, purchase.CLValueCents)

	now := time.Now()
	sa.CreatedAt = now
	sa.UpdatedAt = now
	if err := s.sales.CreateSale(ctx, sa); err != nil {
		return err
	}

	// Best-effort: clear eBay export flag since the card is now sold
	if clearErr := s.purchases.ClearEbayExportFlags(ctx, []string{sa.PurchaseID}); clearErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "create sale: failed to clear ebay export flag",
				observability.String("purchaseID", sa.PurchaseID),
				observability.Err(clearErr))
		}
	}

	return nil
}

func (s *service) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Sale, error) {
	return s.sales.ListSalesByCampaign(ctx, campaignID, limit, offset)
}

func (s *service) CreateBulkSales(ctx context.Context, campaignID string, channel SaleChannel, saleDate string, items []BulkSaleInput) (*BulkSaleResult, error) {
	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign not found: %w", err)
	}

	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.PurchaseID)
	}
	purchasesByID, err := s.purchases.GetPurchasesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load purchases: %w", err)
	}

	result := &BulkSaleResult{}
	for _, item := range items {
		purchase, ok := purchasesByID[item.PurchaseID]
		if !ok {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: "purchase not found"})
			continue
		}
		if purchase.CampaignID != campaignID {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: "purchase does not belong to this campaign"})
			continue
		}
		sa := &Sale{
			PurchaseID:     item.PurchaseID,
			SaleChannel:    channel,
			SalePriceCents: item.SalePriceCents,
			SaleDate:       saleDate,
		}

		// Inline sale creation without captureMarketSnapshot to avoid hitting
		// external pricing APIs for every card (which causes timeouts on bulk sales).
		// The purchase already has a market snapshot from when it was created;
		// the scheduler will refresh it if needed.
		if err := ValidateSale(sa); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: err.Error()})
			continue
		}

		sa.ID = s.idGen()
		sa.SaleFeeCents = CalculateSaleFee(sa.SaleChannel, sa.SalePriceCents, campaign)

		purchaseDate, parseErr := time.Parse("2006-01-02", purchase.PurchaseDate)
		if parseErr == nil {
			saleDateParsed, parseErr2 := time.Parse("2006-01-02", sa.SaleDate)
			if parseErr2 == nil {
				if saleDateParsed.Before(purchaseDate) {
					result.Failed++
					result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: ErrSaleDateBeforePurchase.Error()})
					continue
				}
				sa.DaysToSell = int(saleDateParsed.Sub(purchaseDate).Hours() / 24)
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

		// Best-effort: clear eBay export flag since the card is now sold
		if clearErr := s.purchases.ClearEbayExportFlags(ctx, []string{sa.PurchaseID}); clearErr != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "bulk sale: failed to clear ebay export flag",
					observability.String("purchaseID", sa.PurchaseID),
					observability.Err(clearErr))
			}
		}

		result.Created++
	}
	return result, nil
}

func (s *service) ReassignPurchase(ctx context.Context, purchaseID string, newCampaignID string) error {
	// Verify purchase exists
	if _, err := s.purchases.GetPurchase(ctx, purchaseID); err != nil {
		return fmt.Errorf("purchase lookup: %w", err)
	}

	// Verify target campaign exists and get its sourcing fee
	campaign, err := s.campaigns.GetCampaign(ctx, newCampaignID)
	if err != nil {
		return fmt.Errorf("campaign lookup: %w", err)
	}

	// UpdatePurchaseCampaign atomically rejects the update if a linked sale exists.
	return s.purchases.UpdatePurchaseCampaign(ctx, purchaseID, newCampaignID, campaign.PSASourcingFeeCents)
}
