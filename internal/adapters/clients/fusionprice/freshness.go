package fusionprice

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// fusionProbeGrades derives the DB probe grades from the domain CoreGrades.
var fusionProbeGrades = func() []string {
	labels := make([]string, len(pricing.CoreGrades))
	for i, g := range pricing.CoreGrades {
		labels[i] = g.DisplayLabel()
	}
	return labels
}()

// probeFusionEntry returns the first fusion DB entry found across probe grades.
// When freshness is non-zero, entries older than the threshold are skipped so that
// a stale higher-grade row doesn't block a fresher lower-grade row.
func (f *FusionPriceProvider) probeFusionEntry(ctx context.Context, card pricing.Card, freshness time.Duration) *pricing.PriceEntry {
	if f.priceRepo == nil {
		return nil
	}
	for _, grade := range fusionProbeGrades {
		entry, err := f.priceRepo.GetLatestPrice(ctx, card, grade, "fusion")
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if f.logger != nil {
				f.logger.Warn(ctx, "probeFusionEntry: repo error",
					observability.String("card", card.Name),
					observability.String("grade", grade),
					observability.Err(err))
			}
			continue
		}
		if entry == nil {
			continue
		}
		if freshness > 0 && time.Since(entry.UpdatedAt) > freshness {
			continue
		}
		return entry
	}
	return nil
}

// getFromDatabase retrieves fresh prices from database (within freshness threshold).
// Returns the price and the age of the entry. Prices older than the freshness duration
// are considered stale and will return nil.
func (f *FusionPriceProvider) getFromDatabase(ctx context.Context, card pricing.Card) (*pricing.Price, time.Duration) {
	// Defensive guard: use default freshness duration if not configured
	fd := f.freshnessDuration
	if fd <= 0 {
		fd = DefaultFreshnessDuration
	}

	probeEntry := f.probeFusionEntry(ctx, card, fd)
	if probeEntry == nil {
		return nil, 0
	}

	age := time.Since(probeEntry.UpdatedAt)

	// Build price with all available fusion grades from DB
	price := f.convertEntryToPrice(probeEntry)
	f.loadAllFusionGrades(ctx, price, card, fd)
	price.Amount = price.Grades.PSA10Cents // Primary price (matches price_merger live path)
	f.supplementWithCachedDetails(ctx, price, card)

	return price, age
}

// getStalePrice retrieves any price from database (even if very old)
// Returns the price and the age of the entry
func (f *FusionPriceProvider) getStalePrice(ctx context.Context, card pricing.Card) (*pricing.Price, time.Duration) {
	probeEntry := f.probeFusionEntry(ctx, card, 0)
	if probeEntry == nil {
		return nil, 0
	}
	age := time.Since(probeEntry.UpdatedAt)
	price := f.convertEntryToPrice(probeEntry)
	f.loadAllFusionGrades(ctx, price, card, 0)
	price.Amount = price.Grades.PSA10Cents // Primary price (matches price_merger live path)
	f.supplementWithCachedDetails(ctx, price, card)

	return price, age
}

// supplementWithCachedDetails applies cached grade details to a price retrieved from DB.
// The details cache has a longer TTL than the main cache, so details survive between
// main cache expiry and the DB freshness window.
// Falls back to querying the DB for CardHedger and PriceCharting prices when the
// in-memory cache has expired.
func (f *FusionPriceProvider) supplementWithCachedDetails(ctx context.Context, price *pricing.Price, card pricing.Card) {
	if price == nil {
		return
	}

	fd := f.freshnessDuration
	if fd <= 0 {
		fd = DefaultFreshnessDuration
	}

	detailsKey := detailsCacheKey(card)
	if cached, err := f.getCached(ctx, detailsKey); err == nil && cached != nil {
		price.GradeDetails = cached.GradeDetails
		price.Velocity = cached.Velocity
		price.PCGrades = cached.PCGrades
		// RawNMCents is stored separately by the JustTCG scheduler and is not
		// part of the live-fetch cache; reconstruct from DB.
		f.supplementJustTCGFromDB(ctx, price, card, fd)
		price.Sources = buildSourcesFromData(price)
		return
	}

	// In-memory cache miss — reconstruct from DB entries.
	if f.priceRepo == nil {
		return
	}

	// Reconstruct PriceCharting raw grades from DB
	f.supplementPCGradesFromDB(ctx, price, card, fd)

	// Reconstruct CardHedger EstimateGradeDetail from DB-stored batch data
	f.supplementCardHedgerFromDB(ctx, price, card, fd)

	// Reconstruct JustTCG NM price from DB
	f.supplementJustTCGFromDB(ctx, price, card, fd)

	// Rebuild Sources from the data we actually found
	price.Sources = buildSourcesFromData(price)
}

// gradeDBKey maps a detail key (e.g. "psa10") to a DB grade label (e.g. "PSA 10").
// gradeDBKey pairs a domain Grade (used as detail/fusion key) with its DB display label.
type gradeDBKey struct {
	grade   pricing.Grade
	dbGrade string
}

// gradeDBKeys derives from CoreGrades to avoid duplicating grade strings.
var gradeDBKeys = func() []gradeDBKey {
	keys := make([]gradeDBKey, len(pricing.CoreGrades))
	for i, g := range pricing.CoreGrades {
		keys[i] = gradeDBKey{grade: g, dbGrade: g.DisplayLabel()}
	}
	return keys
}()

// supplementFromDB fetches all grade entries for a card/source in a single
// batch query (GetLatestPricesBySource) and calls applyFn for each grade that
// has a positive price. This replaces the previous loop of 4 individual
// GetLatestPrice calls per source, reducing 12 sequential DB queries to 3
// (one per source) during cache miss recovery.
func (f *FusionPriceProvider) supplementFromDB(ctx context.Context, card pricing.Card, source string, freshness time.Duration, applyFn func(gk gradeDBKey, entry *pricing.PriceEntry)) {
	if f.priceRepo == nil {
		return
	}

	entries, err := f.priceRepo.GetLatestPricesBySource(ctx, card.Name, card.Set, card.Number, source, freshness)
	if err != nil {
		if f.logger != nil {
			f.logger.Warn(ctx, "supplementFromDB: batch repo error",
				observability.String("card", card.Name),
				observability.String("source", source),
				observability.Err(err))
		}
		return
	}

	for _, gk := range gradeDBKeys {
		entry, ok := entries[gk.dbGrade]
		if !ok {
			continue
		}
		if entry.PriceCents <= 0 {
			continue
		}
		applyFn(gk, &entry)
	}
}

// supplementCardHedgerFromDB queries the DB for CardHedger prices stored by the
// batch/delta schedulers and adds them as EstimateGradeDetail entries.
func (f *FusionPriceProvider) supplementCardHedgerFromDB(ctx context.Context, price *pricing.Price, card pricing.Card, freshness time.Duration) {
	f.supplementFromDB(ctx, card, pricing.SourceCardHedger, freshness, func(gk gradeDBKey, entry *pricing.PriceEntry) {
		if price.GradeDetails == nil {
			price.GradeDetails = make(map[string]*pricing.GradeDetail)
		}
		detail, ok := price.GradeDetails[gk.grade.String()]
		if !ok {
			detail = &pricing.GradeDetail{}
			price.GradeDetails[gk.grade.String()] = detail
		}
		if detail.Estimate == nil {
			detail.Estimate = &pricing.EstimateGradeDetail{
				PriceCents: entry.PriceCents,
				Confidence: entry.Confidence,
			}
		}
	})
}

// supplementPCGradesFromDB queries the DB for PriceCharting prices and populates
// PCGrades so buildSourcePrices can show PriceCharting's actual grade prices.
func (f *FusionPriceProvider) supplementPCGradesFromDB(ctx context.Context, price *pricing.Price, card pricing.Card, freshness time.Duration) {
	if price.PCGrades != nil {
		return
	}
	var pcg pricing.GradedPrices
	found := false
	f.supplementFromDB(ctx, card, pricing.SourcePriceCharting, freshness, func(gk gradeDBKey, entry *pricing.PriceEntry) {
		found = true
		pricing.SetGradePrice(&pcg, gk.grade, entry.PriceCents)
	})
	if found {
		price.PCGrades = &pcg
	}
}

// supplementJustTCGFromDB queries the DB for JustTCG NM prices and populates
// price.Grades.RawNMCents. This is separate from the standard supplementFromDB
// flow because GradeRawNM is not in CoreGrades (adding it would cause CardHedger
// to request an unsupported grade).
func (f *FusionPriceProvider) supplementJustTCGFromDB(ctx context.Context, price *pricing.Price, card pricing.Card, freshness time.Duration) {
	if f.priceRepo == nil || price.Grades.RawNMCents > 0 {
		return
	}
	entries, err := f.priceRepo.GetLatestPricesBySource(ctx, card.Name, card.Set, card.Number, pricing.SourceJustTCG, freshness)
	if err != nil || len(entries) == 0 {
		return
	}
	entry, ok := entries[pricing.GradeRawNM.DisplayLabel()]
	if !ok || entry.PriceCents <= 0 {
		return
	}
	pricing.SetGradePrice(&price.Grades, pricing.GradeRawNM, entry.PriceCents)
}

// loadAllFusionGrades queries the DB for all fusion grade entries and populates price.Grades.
// If freshness is 0, no staleness check is applied (used by getStalePrice).
func (f *FusionPriceProvider) loadAllFusionGrades(ctx context.Context, price *pricing.Price, card pricing.Card, freshness time.Duration) {
	if f.priceRepo == nil {
		return
	}
	for _, gk := range gradeDBKeys {
		entry, err := f.priceRepo.GetLatestPrice(ctx, card, gk.dbGrade, "fusion")
		if err != nil || entry == nil || entry.PriceCents <= 0 {
			continue
		}
		if freshness > 0 && time.Since(entry.UpdatedAt) > freshness {
			continue
		}
		pricing.SetGradePrice(&price.Grades, gk.grade, entry.PriceCents)
	}
}

// convertEntryToPrice converts a database price entry back to pricing.Price.
// Grade-specific prices are NOT set here — loadAllFusionGrades populates them
// from DB so that each grade slot gets the correct value.
func (f *FusionPriceProvider) convertEntryToPrice(entry *pricing.PriceEntry) *pricing.Price {
	source := pricing.Source(entry.Source)
	if source == "" {
		source = pricing.SourcePriceCharting
	}

	price := &pricing.Price{
		Currency:   "USD",
		Source:     source,
		Confidence: entry.Confidence,
	}

	// Add fusion metadata
	price.FusionMetadata = &pricing.FusionMetadata{
		SourceCount:   entry.FusionSourceCount,
		OutliersFound: entry.FusionOutliersRemoved,
		Method:        entry.FusionMethod,
	}

	return price
}

// buildSourcesFromData inspects the price data and returns which sources
// contributed, based on what fields are populated rather than tracking state.
func buildSourcesFromData(price *pricing.Price) []string {
	var sources []string
	if price.PCGrades != nil {
		sources = append(sources, pricing.SourcePriceCharting)
	}
	if price.GradeDetails != nil {
		hasEstimate := false
		for _, gd := range price.GradeDetails {
			if gd == nil {
				continue
			}
			if gd.Estimate != nil {
				hasEstimate = true
			}
		}
		if hasEstimate {
			sources = append(sources, pricing.SourceCardHedger)
		}
	}
	if price.Grades.RawNMCents > 0 {
		sources = append(sources, pricing.SourceJustTCG)
	}
	return sources
}
