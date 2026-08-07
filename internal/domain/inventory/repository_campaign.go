package inventory

import (
	"context"
	"time"
)

// CampaignRepository handles campaign persistence.
type CampaignRepository interface {
	CreateCampaign(ctx context.Context, c *Campaign) error
	GetCampaign(ctx context.Context, id string) (*Campaign, error)
	ListCampaigns(ctx context.Context, activeOnly bool) ([]Campaign, error)
	UpdateCampaign(ctx context.Context, c *Campaign) error
	// UpdateCampaignIfUnchanged writes the row only if its stored updated_at is
	// still expectedUpdatedAt, returning ErrCampaignConflict when it is not.
	//
	// This exists alongside the unconditional UpdateCampaign because the two
	// writers differ in kind. Read-modify-write callers (the campaign edit form
	// and the bulk-paste importer) hold a row they read moments ago and must not
	// clobber a concurrent write; they use this. The psa-harvest baseline pull
	// (cmd/psa-harvest/baseline.go) constructs its update without a prior read,
	// so it has no expected version to offer and keeps using UpdateCampaign.
	UpdateCampaignIfUnchanged(ctx context.Context, c *Campaign, expectedUpdatedAt time.Time) error
	DeleteCampaign(ctx context.Context, id string) error
}
