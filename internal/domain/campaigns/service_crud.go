package campaigns

import (
	"context"
	"fmt"
	"time"
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
	return s.repo.CreateCampaign(ctx, c)
}

func (s *service) GetCampaign(ctx context.Context, id string) (*Campaign, error) {
	return s.repo.GetCampaign(ctx, id)
}

func (s *service) ListCampaigns(ctx context.Context, activeOnly bool) ([]Campaign, error) {
	return s.repo.ListCampaigns(ctx, activeOnly)
}

func (s *service) UpdateCampaign(ctx context.Context, c *Campaign) error {
	if err := ValidateAndNormalizeCampaign(c); err != nil {
		return err
	}
	c.UpdatedAt = time.Now()
	return s.repo.UpdateCampaign(ctx, c)
}

func (s *service) DeleteCampaign(ctx context.Context, id string) error {
	return s.repo.DeleteCampaign(ctx, id)
}

func (s *service) DeletePurchase(ctx context.Context, id string) error {
	return s.repo.DeletePurchase(ctx, id)
}

func (s *service) CreatePurchase(ctx context.Context, p *Purchase) error {
	if err := ValidateAndNormalizePurchase(p); err != nil {
		return err
	}
	if p.ID == "" {
		p.ID = s.idGen()
	}

	// Verify campaign exists
	_, err := s.repo.GetCampaign(ctx, p.CampaignID)
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
	return s.repo.CreatePurchase(ctx, p)
}

func (s *service) GetPurchase(ctx context.Context, id string) (*Purchase, error) {
	return s.repo.GetPurchase(ctx, id)
}

func (s *service) ListPurchasesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Purchase, error) {
	return s.repo.ListPurchasesByCampaign(ctx, campaignID, limit, offset)
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
	return s.repo.CreateSale(ctx, sa)
}

func (s *service) ListSalesByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]Sale, error) {
	return s.repo.ListSalesByCampaign(ctx, campaignID, limit, offset)
}

func (s *service) CreateBulkSales(ctx context.Context, campaignID string, channel SaleChannel, saleDate string, items []BulkSaleInput) (*BulkSaleResult, error) {
	campaign, err := s.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign not found: %w", err)
	}

	result := &BulkSaleResult{}
	for _, item := range items {
		purchase, err := s.repo.GetPurchase(ctx, item.PurchaseID)
		if err != nil {
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
		if err := s.CreateSale(ctx, sa, campaign, purchase); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: err.Error()})
			continue
		}
		result.Created++
	}
	return result, nil
}
