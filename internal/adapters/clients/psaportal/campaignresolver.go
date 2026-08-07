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
//
// Both reads are memoized for the resolver's lifetime. One run constructs one
// resolver (cmd/psa-harvest builds it per harvest; the app builds it at
// startup and its imports resolve against the same snapshot the scheduler just
// wrote), and neither the stored snapshot nor the campaign list changes under
// a running reconcile — so re-reading both on every row only multiplied round
// trips against a bounded deadline. Not concurrency-safe by design: the
// resolver is called from a single reconcile/import loop.
type CampaignResolver struct {
	snap      psacampaign.SnapshotStore
	campaigns CampaignLister
	now       func() time.Time // test seam

	portal        []psacampaign.PortalCampaign
	portalFetched time.Time
	portalLoaded  bool

	campaignList   []inventory.Campaign
	campaignLoaded bool
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

	portal, fetchedAt, err := r.loadSnapshot(ctx)
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

	all, err := r.loadCampaigns(ctx)
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

// loadSnapshot reads the portal campaign snapshot once per resolver. A failed
// read is not memoized, so a transient DB error does not poison the whole run.
func (r *CampaignResolver) loadSnapshot(ctx context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
	if r.portalLoaded {
		return r.portal, r.portalFetched, nil
	}
	portal, fetchedAt, err := r.snap.GetSnapshot(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	r.portal, r.portalFetched, r.portalLoaded = portal, fetchedAt, true
	return r.portal, r.portalFetched, nil
}

// loadCampaigns lists internal campaigns once per resolver.
//
// activeOnly=false: a purchase can legitimately belong to a paused campaign,
// and reconciliation runs over historical rows.
func (r *CampaignResolver) loadCampaigns(ctx context.Context) ([]inventory.Campaign, error) {
	if r.campaignLoaded {
		return r.campaignList, nil
	}
	all, err := r.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return nil, err
	}
	r.campaignList, r.campaignLoaded = all, true
	return r.campaignList, nil
}
