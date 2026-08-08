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

func (s *service) UpdateCampaignIfUnchanged(ctx context.Context, c *Campaign, expectedUpdatedAt time.Time) error {
	if err := ValidateAndNormalizeCampaign(c); err != nil {
		return err
	}
	// The new stamp and the precondition are different values: UpdatedAt is what
	// the row becomes, expectedUpdatedAt is what it must currently be.
	c.UpdatedAt = time.Now()
	return s.campaigns.UpdateCampaignIfUnchanged(ctx, c, expectedUpdatedAt)
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
	campaign, err := s.campaigns.GetCampaign(ctx, p.CampaignID)
	if err != nil {
		return fmt.Errorf("campaign lookup: %w", err)
	}

	// Server-authoritative: discard any client-supplied frozen provenance up front.
	// The HTTP handler decodes the raw request body straight into inventory.Purchase,
	// so these pointers are attacker-controllable; clearing them here ensures the
	// freeze logic below can only ever set SERVER-derived values.
	p.CLConfidenceAtPurchase = nil
	p.PopulationAtPurchase = nil
	p.DHConfidenceAtPurchase = nil
	p.SourceCountAtPurchase = nil
	p.ActiveListingsAtPurchase = nil
	p.SalesLast30dAtPurchase = nil

	// (a) creation-time facts, set-once.
	if c, ok := ParseCLConfidenceMin(campaign.CLConfidence); ok {
		p.CLConfidenceAtPurchase = &c
	}
	if p.Population > 0 {
		pop := p.Population
		p.PopulationAtPurchase = &pop
	}

	// Skip synchronous market snapshot when the caller has flagged the purchase
	// for asynchronous background enrichment (e.g. during bulk PSA import).
	if p.SnapshotStatus != SnapshotStatusPending {
		// Best-effort: capture market snapshot at time of purchase.
		if snap, ok := s.captureMarketSnapshot(ctx, p, p.ToCardIdentity(), p.GradeValue, p.CLValueCents); ok {
			// (b) market-time facts, gated on confirmed capture.
			conf := snap.Confidence
			p.DHConfidenceAtPurchase = &conf
			sc := p.SourceCountRaw // set on the embed by applyMarketSnapshot
			p.SourceCountAtPurchase = &sc
			if p.MarketDataObserved {
				al := p.ActiveListings
				sl := p.SalesLast30d
				p.ActiveListingsAtPurchase = &al
				p.SalesLast30dAtPurchase = &sl
			}
		}
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

func (s *service) QuickAddPurchase(ctx context.Context, campaignID string, req QuickAddRequest) (*Purchase, error) {
	if s.certLookup == nil {
		return nil, ErrCertLookupNotConfigured
	}

	info, err := s.certLookup.LookupCert(ctx, req.CertNumber)
	if err != nil {
		return nil, fmt.Errorf("cert lookup: %w", err)
	}

	purchaseDate := req.PurchaseDate
	if purchaseDate == "" {
		purchaseDate = time.Now().Format("2006-01-02")
	}

	setName := ResolvePSACategory(info.Category)
	if IsGenericSetName(setName) {
		setName = info.Category // keep original if resolved is still generic
	}

	p := &Purchase{
		CampaignID:   campaignID,
		CardName:     info.CardName,
		CertNumber:   req.CertNumber,
		CardNumber:   info.CardNumber,
		SetName:      setName,
		Grader:       "PSA",
		GradeValue:   info.Grade,
		BuyCostCents: req.BuyCostCents,
		CLValueCents: req.CLValueCents,
		PurchaseDate: purchaseDate,
		// campaignID is the campaign an operator opened in the UI, not a heuristic
		// match, so this is a manual attribution. The store would otherwise default
		// it to 'inferred', which ReconcilePSAAttribution is free to overwrite.
		AttributionSource: AttributionSourceManual,
	}

	campaign, err := s.campaigns.GetCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign lookup: %w", err)
	}
	p.PSASourcingFeeCents = campaign.PSASourcingFeeCents

	if err := s.CreatePurchase(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// freezeSaleProvenance validates and defaults SaleReason, then overwrites the
// derived, server-authoritative provenance fields (CLValueAtSaleCents,
// ChannelFeePctAtSale, ForcedLiquidation) at sale-creation time. SaleReason
// itself is legitimate client input and is preserved when valid.
func freezeSaleProvenance(sa *Sale, purchase *Purchase, campaign *Campaign, forced bool) error {
	if sa.SaleReason != "" && !ValidSaleReason(sa.SaleReason) {
		return ErrInvalidSaleReason
	}
	if sa.SaleReason == "" {
		if forced {
			sa.SaleReason = SaleReasonInvoicePressure
		} else {
			sa.SaleReason = SaleReasonDiscretionary
		}
	}
	// Server-authoritative: overwrite any client-supplied values.
	sa.CLValueAtSaleCents = purchase.CLValueCents
	pct := EffectiveChannelFeePct(sa.SaleChannel, campaign)
	sa.ChannelFeePctAtSale = &pct
	// Keep the plain boolean in sync with the reason (app-maintained; not generated).
	sa.ForcedLiquidation = IsForcedReason(sa.SaleReason)
	return nil
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

	invoices, invErr := s.finance.ListInvoices(ctx)
	if invErr != nil {
		invoices = nil // heuristic degrades to false; never block a sale on invoice lookup
	}
	if err := freezeSaleProvenance(sa, purchase, campaign, IsForcedLiquidation(sa.SaleChannel, sa.SaleDate, invoices)); err != nil {
		return err
	}

	// Best-effort: capture market snapshot at time of sale
	_, _ = s.captureMarketSnapshot(ctx, sa, purchase.ToCardIdentity(), purchase.GradeValue, purchase.CLValueCents)

	now := time.Now()
	sa.CreatedAt = now
	sa.UpdatedAt = now
	if err := s.sales.CreateSale(ctx, sa); err != nil {
		return err
	}

	// Best-effort: mark DH status as sold so inventory UI reflects reality
	if dhErr := s.purchases.UpdatePurchaseDHStatus(ctx, sa.PurchaseID, string(DHStatusSold)); dhErr != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "create sale: failed to update dh_status to sold",
				observability.String("purchaseID", sa.PurchaseID),
				observability.Err(dhErr))
		}
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

	bulkInvoices, invErr := s.finance.ListInvoices(ctx)
	if invErr != nil {
		bulkInvoices = nil // heuristic degrades to false; never block a sale on invoice lookup
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
			PurchaseID:             item.PurchaseID,
			SaleChannel:            channel,
			SalePriceCents:         item.SalePriceCents,
			SaleDate:               saleDate,
			OriginalListPriceCents: item.OriginalListPriceCents,
			PriceReductions:        item.PriceReductions,
			DaysListed:             item.DaysListed,
			SaleReason:             item.SaleReason,
		}

		// Inline sale creation without captureMarketSnapshot to avoid hitting
		// external pricing APIs for every card (which causes timeouts on bulk sales).
		// Bulk-created sales will not have MarketSnapshotData unless SaleRepository
		// gains an update/backfill path.
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
		if err := freezeSaleProvenance(sa, purchase, campaign, IsForcedLiquidation(sa.SaleChannel, sa.SaleDate, bulkInvoices)); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: err.Error()})
			continue
		}

		now := time.Now()
		sa.CreatedAt = now
		sa.UpdatedAt = now

		if err := s.sales.CreateSale(ctx, sa); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BulkSaleError{PurchaseID: item.PurchaseID, Error: err.Error()})
			continue
		}

		// Best-effort: mark DH status as sold so inventory UI reflects reality
		if dhErr := s.purchases.UpdatePurchaseDHStatus(ctx, sa.PurchaseID, string(DHStatusSold)); dhErr != nil {
			if s.logger != nil {
				s.logger.Warn(ctx, "bulk sale: failed to update dh_status to sold",
					observability.String("purchaseID", sa.PurchaseID),
					observability.Err(dhErr))
			}
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

// ReassignPurchase hand-moves a purchase to a different campaign, which
// UpdatePurchaseCampaign records as attribution_source='manual'. PSA's claim
// about the cert (psa_campaign_name) is left untouched: it remains true and
// worth keeping even after an operator overrides where the purchase is booked.
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

func (s *service) UpdateSaleReason(ctx context.Context, campaignID, saleID, reason string) error {
	if !ValidSaleReasonForPatch(reason) {
		return ErrInvalidSaleReason
	}
	return s.sales.UpdateSaleReason(ctx, campaignID, saleID, reason, IsForcedReason(reason))
}
