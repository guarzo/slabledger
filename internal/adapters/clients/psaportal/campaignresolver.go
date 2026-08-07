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

// memoTTL bounds how long the resolver serves a memoized read. It must be long
// enough to cover a single import/reconcile pass (~1500 rows against a bounded
// deadline) so a pass does one read of each store rather than one per row, and
// short enough that a long-lived resolver self-heals: the app builds one
// resolver at startup (cmd/slabledger/init_inventory_services.go) and reuses it
// for the process's whole uptime, so a latched snapshot would age past
// maxSnapshotAge and disable PSA attribution until restart.
const memoTTL = 2 * time.Minute

// CampaignResolver implements inventory.PSACampaignResolver over the portal
// campaign snapshot and the internal campaign list.
//
// Both reads are memoized for memoTTL. Neither the stored snapshot nor the
// campaign list changes under a running reconcile, so re-reading both on every
// row only multiplied round trips against a bounded deadline; bounding the
// memo by time keeps that win while letting a long-lived resolver pick up
// later snapshot refreshes and newly created campaigns. Not concurrency-safe
// by design: the resolver is called from a single reconcile/import loop.
type CampaignResolver struct {
	snap      psacampaign.SnapshotStore
	campaigns CampaignLister
	now       func() time.Time // test seam

	portal         []psacampaign.PortalCampaign
	portalFetched  time.Time
	portalLoadedAt time.Time

	campaignList     []inventory.Campaign
	campaignLoadedAt time.Time
}

// memoValid reports whether a value loaded at loadedAt is still fresh enough to
// serve. A zero loadedAt means nothing has been loaded yet.
func (r *CampaignResolver) memoValid(loadedAt time.Time) bool {
	return !loadedAt.IsZero() && r.now().Sub(loadedAt) <= memoTTL
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

// loadSnapshot reads the portal campaign snapshot at most once per memoTTL. A
// failed read is not memoized, so a transient DB error does not poison the run.
func (r *CampaignResolver) loadSnapshot(ctx context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
	if r.memoValid(r.portalLoadedAt) {
		return r.portal, r.portalFetched, nil
	}
	portal, fetchedAt, err := r.snap.GetSnapshot(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	r.portal, r.portalFetched, r.portalLoadedAt = portal, fetchedAt, r.now()
	return r.portal, r.portalFetched, nil
}

// loadCampaigns lists internal campaigns at most once per memoTTL.
//
// activeOnly=false: a purchase can legitimately belong to a paused campaign,
// and reconciliation runs over historical rows.
func (r *CampaignResolver) loadCampaigns(ctx context.Context) ([]inventory.Campaign, error) {
	if r.memoValid(r.campaignLoadedAt) {
		return r.campaignList, nil
	}
	all, err := r.campaigns.ListCampaigns(ctx, false)
	if err != nil {
		return nil, err
	}
	r.campaignList, r.campaignLoadedAt = all, r.now()
	return r.campaignList, nil
}
