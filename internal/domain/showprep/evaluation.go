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
			if sale.Date > e.LatestSaleDate {
				e.LatestSaleDate = sale.Date
			}
		}
	}
	slices.Sort(prices)
	e.CompCount = len(prices)
	var twice int64
	if len(prices) > 0 {
		twice = 2 * int64(prices[len(prices)/2])
		if len(prices)%2 == 0 {
			twice = int64(prices[len(prices)/2-1]) + int64(prices[len(prices)/2])
		}
		e.MedianCents = int((twice + 1) / 2)
	}
	e.Status = NeedsReview
	switch {
	case p.ListedPriceCents <= 0:
		e.Status = NoListedPrice
		e.Reason = "No positive DH listed price"
	case p.PriceAssociationUnclear:
		e.Reason = "DH price association unclear"
	case !p.Identity().Valid():
		e.Reason = "Comparable identity unresolved"
	case s == nil:
		e.Reason = "No verified CardLadder evidence"
	case s.Source != "cardladder" && s.AttemptState == "complete":
		e.Reason = "No verified CardLadder source provenance"
	case s.Identity != p.Identity():
		e.Reason = "Comparable identity mismatch"
	case s.AttemptState != "complete":
		e.Reason = s.AttemptError
		if e.Reason == "" {
			e.Reason = "Refresh incomplete"
		}
	case invalid:
		e.Reason = "Invalid comparable records"
	case !s.Complete:
		e.Reason = "Incomplete source window"
	case s.WindowStart != start || s.WindowEnd != end:
		e.Reason = "Evidence does not cover current 30-day window"
	case s.RefreshedAt.IsZero() || s.RefreshedAt.After(now) || now.Sub(s.RefreshedAt) > 24*time.Hour:
		e.Reason = "Evidence is stale"
	case len(prices) == 0:
		e.Status = NoRecentComps
		e.Reason = "Complete lookup found no recent matching sales"
	case 5*twice < 9*int64(p.ListedPriceCents):
		e.Status = BelowTarget
		e.Reason = "Median below 90% of DH listed price"
	case len(prices) == 1:
		e.Status = ThinEvidence
		e.Reason = "Only one matching sale supports the listed price"
	default:
		e.Status = Supported
		e.Reason = "Recent matching median supports the listed price"
	}
	// Include derived status/window (date rollover and freshness matter), but not the read instant.
	e.Version = Fingerprint(struct {
		Purchase   Purchase
		Evaluation Evaluation
	}{p, e})
	return e
}
