package inventory

import "time"

// ResolveListingPriceCents returns the operator-committed price for DH
// listing. Both ReviewedPriceCents (price-review flow) and OverridePriceCents
// (Set Price dialog) are treated as human commitments; when both are set,
// the newest commit wins. Timestamps are compared by parsed time so RFC3339
// values with different offsets compare chronologically — string comparison
// alone would mis-order "2026-04-21T08:00:00-05:00" (13:00 UTC) against
// "2026-04-21T12:00:00Z" (12:00 UTC). When either timestamp fails to parse
// (most commonly because it's empty on legacy rows), we fall back to
// lexicographic comparison — which preserves the historical "reviewed wins
// when both empty" and "populated timestamp beats empty" behaviors.
// CL is deliberately excluded: it can be stale and we don't want to
// silently list at a wrong price. Returns 0 when neither field is set —
// callers treat that as "omit listing_price_cents and let DH's catalog
// fallback take over" (fine for in_stock, rejected at list time).
//
// This lives on the hub rather than in a sibling because both dhlisting and
// dhpricing need it and the flat-sibling rule bars them from importing each
// other. It was duplicated in both for exactly that reason (SLA-89).
func ResolveListingPriceCents(p *Purchase) int {
	if p.OverridePriceCents == 0 {
		return p.ReviewedPriceCents
	}
	if p.ReviewedPriceCents == 0 {
		return p.OverridePriceCents
	}
	if overrideNewer(p.OverrideSetAt, p.ReviewedAt) {
		return p.OverridePriceCents
	}
	return p.ReviewedPriceCents
}

// overrideNewer reports whether the override commit timestamp is strictly
// after the reviewed commit timestamp. Both are RFC3339 strings written by
// the storage layer; we parse them to compare wall-clock instants rather
// than text. On parse failure (empty or malformed timestamps) we fall back
// to string comparison so legacy rows preserve their prior resolution.
func overrideNewer(overrideSetAt, reviewedAt string) bool {
	tOverride, errO := time.Parse(time.RFC3339, overrideSetAt)
	tReviewed, errR := time.Parse(time.RFC3339, reviewedAt)
	if errO == nil && errR == nil {
		return tOverride.After(tReviewed)
	}
	return overrideSetAt > reviewedAt
}
