package inventory

import (
	"context"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) LookupCert(ctx context.Context, certNumber string) (*CertInfo, *MarketSnapshot, error) {
	if s.certLookup == nil {
		return nil, nil, ErrCertLookupNotConfigured
	}

	info, err := s.certLookup.LookupCert(ctx, certNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("cert lookup: %w", err)
	}

	var snapshot *MarketSnapshot
	if s.priceProv != nil && info.CardName != "" && info.Grade > 0 {
		resolvedCategory := ResolvePSACategory(info.Category)
		if IsGenericSetName(resolvedCategory) {
			resolvedCategory = info.Category
		}
		snapshot, err = s.priceProv.GetMarketSnapshot(ctx, CardIdentity{CardName: info.CardName, CardNumber: info.CardNumber, SetName: resolvedCategory}, info.Grade)
		if err != nil && s.logger != nil {
			s.logger.Debug(ctx, "GetMarketSnapshot for cert lookup failed",
				observability.String("card", info.CardName),
				observability.Err(err))
		}
	}

	return info, snapshot, nil
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
