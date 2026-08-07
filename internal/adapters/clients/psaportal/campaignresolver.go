package psaportal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// ErrStaleCampaignSnapshot means the portal campaign list is too old to resolve
// names against. Resolving fresh purchases through a stale campaign list would
// silently fail to resolve names, or resolve them to superseded campaigns.
var ErrStaleCampaignSnapshot = errors.New("psa campaign snapshot is stale")

// CampaignLister reads internal campaigns and their portal links. The signature
// matches CampaignStore.ListCampaigns (internal/adapters/storage/postgres/campaign_store.go:69)
// so the existing store satisfies this interface without a shim.
type CampaignLister interface {
	ListCampaigns(ctx context.Context, activeOnly bool) ([]inventory.Campaign, error)
}

// CampaignResolver implements inventory.PSACampaignResolver over the portal
// campaign snapshot and the internal campaign list.
type CampaignResolver struct {
	snap      psacampaign.SnapshotStore
	campaigns CampaignLister
	now       func() time.Time // test seam
}

func NewCampaignResolver(snap psacampaign.SnapshotStore, campaigns CampaignLister, now func() time.Time) *CampaignResolver {
	if now == nil {
		now = time.Now
	}
	return &CampaignResolver{snap: snap, campaigns: campaigns, now: now}
}

// ResolveCampaignID implements inventory.PSACampaignResolver.
func (r *CampaignResolver) ResolveCampaignID(ctx context.Context, psaName string) (string, bool, error) {
	name := strings.TrimSpace(psaName)
	if name == "" {
		return "", false, nil
	}

	portal, fetchedAt, err := r.snap.GetSnapshot(ctx)
	if err != nil {
		return "", false, fmt.Errorf("read campaign snapshot: %w", err)
	}
	if fetchedAt.IsZero() {
		return "", false, fmt.Errorf("%w: never fetched", ErrStaleCampaignSnapshot)
	}
	if age := r.now().Sub(fetchedAt); age > maxSnapshotAge {
		return "", false, fmt.Errorf("%w (fetched %s ago)", ErrStaleCampaignSnapshot, age.Round(time.Minute))
	}

	var requestID string
	for _, pc := range portal {
		if strings.EqualFold(strings.TrimSpace(pc.Name), name) {
			requestID = pc.CampaignRequestID
			break
		}
	}
	if requestID == "" {
		return "", false, nil // dead campaign name — expected, not an error
	}

	// activeOnly=false: a purchase can legitimately belong to a paused campaign,
	// and reconciliation runs over historical rows.
	all, err := r.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return "", false, fmt.Errorf("list campaigns: %w", err)
	}
	for _, c := range all {
		if c.PSACampaignRequestID != "" && c.PSACampaignRequestID == requestID {
			return c.ID, true, nil
		}
	}
	return "", false, nil
}
