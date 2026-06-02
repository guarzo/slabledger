package scheduler

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/adapters/clients/marketmovers"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/mathutil"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// resolveCollectibleID searches Market Movers for the card and returns its collectible ID,
// master ID (grade-agnostic variant identifier, 0 if unknown), and the canonical SearchTitle.
// When the search yields nothing usable, collectibleID is 0 and reason is set to one of
// the MMReason* constants so the caller can persist it for the admin UI.
//
// Strategy:
//  1. Search by cert number — we embed the cert in the MM export Notes column, and MM
//     indexes PSA cert numbers so this is the most precise lookup available.
//  2. Fall back to a "{CardName} {Grader} {Grade}" text query if the cert search yields
//     no result that matches the card name.
//
// Any candidate returned by either path is validated via tokenized title matching (see
// tokenMatchesTitle) before the ID is cached.
//
// The client is passed in so that all MM API calls in a single resolve cycle hit the
// same underlying transport even if SetClient() is invoked concurrently.
func (s *MarketMoversRefreshScheduler) resolveCollectibleID(ctx context.Context, client *marketmovers.Client, p *inventory.Purchase) (collectibleID, masterID int64, searchTitle, reason string, err error) {
	if p.CardName == "" {
		return 0, 0, "", MMReasonNoCardName, nil
	}

	// 1. Try cert number first. Cert-search miss is NOT terminal — fall through
	// to name search so we still get a chance to map.
	certReason := ""
	if p.CertNumber != "" {
		cid, mid, title, r, cerr := s.searchByCert(ctx, client, p)
		if cerr != nil {
			s.logger.Warn(ctx, "MM: cert-based search failed, falling back to name search",
				observability.String("cert", p.CertNumber),
				observability.Err(cerr))
		} else if cid != 0 {
			s.logger.Info(ctx, "MM: resolved collectible via cert search",
				observability.String("cert", p.CertNumber),
				observability.Int64("collectibleId", cid))
			return cid, mid, title, "", nil
		} else {
			certReason = r
		}
	}

	// 2. Fall back to name + grade search with relevance validation.
	cid, mid, title, nameReason, err := s.searchByNameGrade(ctx, client, p)
	if err != nil || cid != 0 {
		return cid, mid, title, "", err
	}

	// Both paths failed. Prefer the more specific token-mismatch reason
	// (which tells us MM DID have candidates) over the no-results reason.
	combined := nameReason
	if certReason == MMReasonCertTokenMismatch || nameReason == "" {
		combined = certReason
	}
	return 0, 0, "", combined, nil
}

// normalizedCardName converts the raw PSA listing title stored on
// Purchase.CardName into a clean search term for MM's catalog. Falls back to
// the raw CardName only when normalization would strip everything.
func normalizedCardName(p *inventory.Purchase) string {
	if n := cardutil.SimplifyForSearch(cardutil.NormalizePurchaseName(p.CardName)); n != "" {
		return n
	}
	return p.CardName
}

// searchByCert searches MM using the PSA cert number as the query. Returns a
// reason code when no usable result is found.
//
// Token validation runs against the normalized card name: raw PSA titles like
// "PIKACHU-HOLO LEGEND MAKER" tokenize in ways that never match MM's catalog,
// so skipping normalization spuriously rejects valid hits.
func (s *MarketMoversRefreshScheduler) searchByCert(ctx context.Context, client *marketmovers.Client, p *inventory.Purchase) (collectibleID, masterID int64, searchTitle, reason string, err error) {
	results, err := client.SearchCollectibles(ctx, p.CertNumber, 0, 3)
	if err != nil {
		return 0, 0, "", "", fmt.Errorf("search by cert: %w", err)
	}
	if len(results.Items) == 0 {
		return 0, 0, "", MMReasonNoCertResults, nil
	}
	normalizedName := normalizedCardName(p)
	for _, r := range results.Items {
		if !tokenMatchesTitle(normalizedName, r.Item.SearchTitle) {
			continue
		}
		if !gradeMatchesTitle(p.Grader, p.GradeValue, r.Item.SearchTitle) {
			s.logger.Info(ctx, "MM: cert search candidate rejected by grade mismatch",
				observability.String("cert", p.CertNumber),
				observability.String("grader", p.Grader),
				observability.Float64("gradeValue", p.GradeValue),
				observability.String("resultTitle", r.Item.SearchTitle))
			continue
		}
		return r.Item.ID, r.Item.MasterID, r.Item.SearchTitle, "", nil
	}
	s.logger.Info(ctx, "MM: cert search all results rejected by token match",
		observability.String("cert", p.CertNumber),
		observability.String("cardName", p.CardName),
		observability.String("normalized", normalizedName),
		observability.String("sampleResultTitle", results.Items[0].Item.SearchTitle),
		observability.Int("resultCount", len(results.Items)))
	return 0, 0, "", MMReasonCertTokenMismatch, nil
}

// searchByNameGrade searches MM by card name (normalized from the PSA listing
// title on Purchase.CardName) and validates the top result via tokenized
// matching. Returns a reason code when no usable result is found.
//
// If the primary "{name} {grader} {grade}" query returns 0 hits, retries with
// "{name}" alone — MM indexes grade as structured metadata on some cards, so
// dropping the grade token can surface rows the first query misses.
func (s *MarketMoversRefreshScheduler) searchByNameGrade(ctx context.Context, client *marketmovers.Client, p *inventory.Purchase) (collectibleID, masterID int64, searchTitle, reason string, err error) {
	grader := p.Grader
	if grader == "" {
		grader = "PSA"
	}
	normalizedName := normalizedCardName(p)

	var primaryQuery string
	if p.GradeValue == 0 {
		primaryQuery = fmt.Sprintf("%s %s", normalizedName, grader)
	} else {
		primaryQuery = fmt.Sprintf("%s %s %s", normalizedName, grader, mathutil.FormatGrade(p.GradeValue))
	}

	cid, mid, title, reason, err := s.runMMNameQuery(ctx, client, p, primaryQuery)
	if reason != MMReasonNoNameResults {
		return cid, mid, title, reason, err
	}
	return s.runMMNameQuery(ctx, client, p, normalizedName)
}

// runMMNameQuery executes one MM SearchCollectibles call and validates the top
// result via tokenized matching against the normalized card name.
func (s *MarketMoversRefreshScheduler) runMMNameQuery(ctx context.Context, client *marketmovers.Client, p *inventory.Purchase, query string) (collectibleID, masterID int64, searchTitle, reason string, err error) {
	results, err := client.SearchCollectibles(ctx, query, 0, 5)
	if err != nil {
		return 0, 0, "", "", fmt.Errorf("search by name: %w", err)
	}
	if len(results.Items) == 0 {
		return 0, 0, "", MMReasonNoNameResults, nil
	}

	normalizedName := normalizedCardName(p)
	for _, r := range results.Items {
		if !tokenMatchesTitle(normalizedName, r.Item.SearchTitle) {
			continue
		}
		if !gradeMatchesTitle(p.Grader, p.GradeValue, r.Item.SearchTitle) {
			s.logger.Info(ctx, "MM: name search candidate rejected by grade mismatch",
				observability.String("cert", p.CertNumber),
				observability.String("grader", p.Grader),
				observability.Float64("gradeValue", p.GradeValue),
				observability.String("query", query),
				observability.String("resultTitle", r.Item.SearchTitle))
			continue
		}
		s.logger.Info(ctx, "MM: resolved collectible via name search",
			observability.String("cert", p.CertNumber),
			observability.String("query", query),
			observability.String("resultTitle", r.Item.SearchTitle),
			observability.Int64("collectibleId", r.Item.ID))
		return r.Item.ID, r.Item.MasterID, r.Item.SearchTitle, "", nil
	}

	s.logger.Info(ctx, "MM: name search all results rejected",
		observability.String("cert", p.CertNumber),
		observability.String("cardName", p.CardName),
		observability.String("normalized", normalizedName),
		observability.String("query", query),
		observability.String("sampleResultTitle", results.Items[0].Item.SearchTitle),
		observability.Int("resultCount", len(results.Items)))
	return 0, 0, "", MMReasonNameTokenMismatch, nil
}
