package dh

import "errors"

// ErrAnalyticsNotComputed is returned by per-card analytics endpoints
// (velocity, trend, saturation, price_distribution) when DH's nightly
// rollup job has not yet produced a row for the requested card.
//
// Callers should treat this as "skip this card and continue" rather than
// a hard failure. Use errors.Is(err, dh.ErrAnalyticsNotComputed) to detect it.
//
// DH surfaces this as HTTP 404 with a body of the form
// {"error": "analytics_not_computed"}.
var ErrAnalyticsNotComputed = errors.New("dh: analytics not computed for card")
