package inventory

// FillRatePct returns a day's spend as a fraction of the daily spend cap.
// A non-positive cap yields 0: the rule is undefined without a cap, and callers
// render the result directly.
//
// This is the only expression computing the DailySpend fill-rate rule. The
// weekly utilization in internal/domain/portfolio is a separate rule on a
// separate type; it is not a duplicate of this one.
func FillRatePct(spendCents, capCents int) float64 {
	if capCents <= 0 {
		return 0
	}
	return float64(spendCents) / float64(capCents)
}

// EnrichDailySpendFillRate populates CapCents and FillRatePct on each day from
// the campaign's daily spend cap.
//
// The cap is the campaign's *current* cap: no per-day historical cap is
// persisted, so editing a campaign's cap retroactively changes these values.
// That is pre-existing behavior, preserved deliberately.
func EnrichDailySpendFillRate(daily []DailySpend, capCents int) {
	for i := range daily {
		daily[i].CapCents = capCents
		daily[i].FillRatePct = FillRatePct(daily[i].SpendCents, capCents)
	}
}
