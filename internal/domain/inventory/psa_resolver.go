package inventory

import "context"

// PSACampaignResolver maps PSA's own campaign name to an internal campaign ID.
//
// The two-hop lookup (portal snapshot -> psa_campaign_request_id -> campaign)
// is implemented in the adapter layer: the portal snapshot types live in
// internal/domain/psacampaign, which already imports this package, so resolving
// here would create an import cycle.
type PSACampaignResolver interface {
	// ResolveCampaignID returns the internal campaign ID for a PSA campaign name.
	// ok is false when the name is empty or names a campaign no longer in the
	// portal snapshot (e.g. deleted in the 2026-07-27/28 band restructure).
	// A non-nil error means the lookup could not be performed at all — a stale
	// snapshot, for instance — and must not be treated as "unresolved".
	ResolveCampaignID(ctx context.Context, psaName string) (campaignID string, ok bool, err error)
}
