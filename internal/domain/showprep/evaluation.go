package showprep

import (
	"slices"
	"time"
)

// Evaluate is shared by inventory reads, list reads and locked mutations.
// Price quality never overrides physical availability.
func Evaluate(p Purchase, s *Snapshot, now time.Time) Evaluation {
	start, end := Window(now)
	a := AvailabilityOf(p)
	e := Evaluation{PurchaseID: p.ID, CardName: p.CardName, CertNumber: p.CertNumber, Grader: p.Grader, Grade: p.Grade,
		Availability: a, CanAdd: a.CanAdd(), CanPack: a.CanPack(), ListedPriceCents: p.ListedPriceCents,
		LocalPriceCents: p.LocalPriceCents, PriceMismatch: p.LocalPriceCents > 0 && p.LocalPriceCents != p.ListedPriceCents,
		PriceAssociationUnclear: p.PriceAssociationUnclear, WindowStart: start, WindowEnd: end}
	if synced, err := time.Parse(time.RFC3339Nano, p.ListingSyncedAt); err == nil {
		e.ListingSyncedAt = Timestamp(synced)
	}
	var prices []int
	var eligibleSales []Sale
	invalid := false
	if s != nil {
		e.RefreshedAt = Timestamp(s.RefreshedAt)
		e.EvidenceVersion = Fingerprint(s)
		seen := map[string]bool{}
		for _, sale := range s.Sales {
			date, err := time.Parse(time.DateOnly, sale.Date)
			if err != nil || date.Format(time.DateOnly) > end || sale.PriceCents <= 0 || sale.ID == "" {
				invalid = true
				continue
			}
			if sale.Date < start || seen[sale.ID] {
				continue
			}
			seen[sale.ID] = true
			prices = append(prices, sale.PriceCents)
			eligibleSales = append(eligibleSales, sale)
			if sale.Date > e.LatestSaleDate {
				e.LatestSaleDate = sale.Date
			}
		}
	}
	slices.Sort(prices)
	e.CompCount = len(prices)
	// Twice a positive int fits uint64, including half-cent display medians.
	var twice uint64
	if len(prices) > 0 {
		twice = 2 * uint64(prices[len(prices)/2])
		if len(prices)%2 == 0 {
			twice = uint64(prices[len(prices)/2-1]) + uint64(prices[len(prices)/2])
		}
		e.MedianCents = int((twice + 1) / 2)
	}
	// Evidence health is independent of asking price. Validate once so price
	// precedence cannot hide failed attempts or mislabel healthy retained sales.
	switch {
	case !p.Identity().Valid():
		e.EvidenceReason = "Comparable identity unresolved"
	case s == nil:
		e.EvidenceReason = "No verified CardLadder evidence"
	case s.Source != "cardladder" && s.AttemptState == "complete":
		e.EvidenceReason = "No verified CardLadder source provenance"
	case s.Identity != p.Identity():
		e.EvidenceReason = "Comparable identity mismatch"
	case s.AttemptState != "complete":
		e.EvidenceReason = s.AttemptError
		if e.EvidenceReason == "" {
			e.EvidenceReason = "Refresh incomplete"
		}
	case invalid:
		e.EvidenceReason = "Invalid comparable records"
	case !s.Complete:
		e.EvidenceReason = "Incomplete source window"
	case s.WindowStart != start || s.WindowEnd != end:
		e.EvidenceReason = "Evidence does not cover current 30-day window"
	case s.RefreshedAt.IsZero() || s.RefreshedAt.After(now) || now.Sub(s.RefreshedAt) > 24*time.Hour:
		e.EvidenceReason = "Evidence is stale"
	}
	e.EvidenceNeedsReview = e.EvidenceReason != ""
	e.Status, e.Reason, e.Recent = assessRecentPrice(p.LocalPriceCents, eligibleSales, now)
	e.PolicyVersion = PriceAssessmentPolicy
	if e.EvidenceNeedsReview {
		// Retain readable facts, but never certify or show a gap from unhealthy evidence.
		e.Recent.GapPct = nil
		if p.LocalPriceCents > 0 {
			e.Status, e.Reason = NeedsReview, e.EvidenceReason
		}
	}
	// Include derived status/window (date rollover and freshness matter), but not the read instant.
	e.Version = Fingerprint(struct {
		Purchase   Purchase
		Evaluation Evaluation
	}{p, e})
	// Hash the business projection first. The absent optional field
	// is omitted from JSON, so scheduling transitions cannot invalidate selection.
	e.Readiness = deriveReadiness(p.Identity(), s, now, invalid)
	return e
}
