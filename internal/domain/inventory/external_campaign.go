package inventory

import (
	"context"
	"fmt"
	"time"
)

// EnsureExternalCampaign returns the catch-all campaign that holds purchases
// which arrived without a campaign of their own, creating it if absent.
//
// It takes the repository rather than hanging off a service because both intake
// paths need it: cert entry (this package) and CSV import (domain/csvimport),
// which reaches it through the same CampaignRepository it already holds.
func EnsureExternalCampaign(ctx context.Context, campaigns CampaignRepository) (*Campaign, error) {
	c, err := campaigns.GetCampaign(ctx, ExternalCampaignID)
	if err == nil {
		return c, nil
	}
	if !IsCampaignNotFound(err) {
		return nil, fmt.Errorf("lookup external campaign: %w", err)
	}
	now := time.Now()
	c = &Campaign{
		ID:                  ExternalCampaignID,
		Name:                ExternalCampaignName,
		Phase:               PhaseActive,
		PSASourcingFeeCents: 0,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := campaigns.CreateCampaign(ctx, c); err != nil {
		// Race condition: another goroutine may have created it concurrently
		if existing, getErr := campaigns.GetCampaign(ctx, ExternalCampaignID); getErr == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("create external campaign: %w", err)
	}
	return c, nil
}
