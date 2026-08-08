package inventory

// LowSellThroughPct is the sell-through percentage below which a campaign is
// treated as underperforming.
//
// It lives in the hub because two *siblings* act on it: recLowSellThrough in
// internal/domain/tuning, and the campaign-suggestion rule in
// internal/domain/portfolio that may suggest closing the campaign. Siblings
// cannot import each other, so neither package can own the threshold without
// forcing the other to keep a second copy that would drift.
const LowSellThroughPct = 0.30
