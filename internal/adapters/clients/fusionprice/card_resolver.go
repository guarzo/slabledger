package fusionprice

import (
	"context"
	"fmt"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
	domainCards "github.com/guarzo/slabledger/internal/domain/cards"
	"github.com/guarzo/slabledger/internal/domain/constants"
	apperrors "github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/pricing"
)

// LookupCard implements pricing.PriceProvider interface.
// Uses TCGdex as primary resolver when possible, then falls back to
// PriceCharting for fuzzy matching. Runs the full fusion pipeline for pricing.
//
// On fusion errors, LookupCard logs the error and returns the PriceCharting-only
// result with a nil error (degraded-mode behavior). Callers receive a valid
// *pricing.Price from PriceCharting without secondary source data rather than
// an error, so they should check the Sources field to determine data completeness.
func (f *FusionPriceProvider) LookupCard(ctx context.Context, setName string, card domainCards.Card) (*pricing.Price, error) {
	if f.priceCharting == nil {
		return nil, apperrors.ProviderUnavailable(pricing.SourcePriceCharting, fmt.Errorf("provider not configured"))
	}

	// Try TCGdex first for canonical card identity resolution.
	// This avoids PriceCharting fuzzy matching issues (wrong card variants, embedded numbers).
	var canonicalCard *domainCards.Card
	if f.cardProvider != nil {
		canonicalCard = ResolveCardIdentity(ctx, f.cardProvider, card.Name, card.Number, setName)
		if canonicalCard != nil && f.logger != nil {
			f.logger.Debug(ctx, "resolved card identity via TCGdex",
				observability.String("card", card.Name),
				observability.String("canonical", canonicalCard.Name),
				observability.String("canonical_number", canonicalCard.Number),
				observability.String("set", setName))
		}
	}

	// Guard against generic set names that cause wrong-card matches in PriceCharting.
	// If we couldn't resolve via TCGdex and the set is generic, bail early —
	// UNLESS the card has a collector number, which provides enough disambiguation
	// for PriceCharting's card-number verification to reject wrong-set matches.
	if canonicalCard == nil && constants.IsGenericSetName(setName) && card.Number == "" {
		if f.logger != nil {
			f.logger.Warn(ctx, "skipping pricing: generic set name with no card number",
				observability.String("card", card.Name), observability.String("set", setName))
		}
		return nil, nil
	}

	// Use PriceCharting for card identity resolution
	pcLookupCard := card
	if canonicalCard != nil {
		pcLookupCard = *canonicalCard
	}
	pcPrice, err := f.priceCharting.LookupCard(ctx, setName, pcLookupCard)
	if err != nil {
		return nil, err
	}
	if pcPrice == nil {
		if f.logger != nil {
			f.logger.Debug(ctx, "PriceCharting LookupCard returned nil — card not found for pricing",
				observability.String("card", card.Name),
				observability.String("set", setName),
				observability.String("number", card.Number))
		}
		return nil, nil
	}

	// Build resolved identity: prefer canonical TCGdex data, fall back
	// to PriceCharting + validation.
	resolvedName := pcPrice.ProductName
	resolvedNumber := card.Number

	useNoStale := canonicalCard != nil

	if canonicalCard != nil {
		// TCGdex gave us a canonical card — use it directly
		resolvedName = canonicalCard.Name
		if canonicalCard.Number != "" {
			resolvedNumber = canonicalCard.Number
		}
	} else if f.cardProvider != nil {
		// No canonical card from TCGdex — cross-validate PriceCharting result.
		// ValidateCardResolution currently always returns Valid:true (it can't
		// disprove card existence due to incomplete DB coverage), so the
		// canonical-name path below is the only meaningful branch.
		validation := ValidateCardResolution(ctx, f.cardProvider, pcPrice.ProductName, card.Number, setName)
		if validation.CanonicalCard != nil && validation.CanonicalCard.Name != "" {
			if f.logger != nil && pcPrice.ProductName != validation.CanonicalCard.Name {
				f.logger.Debug(ctx, "using canonical card name from database",
					observability.String("original", pcPrice.ProductName),
					observability.String("canonical", validation.CanonicalCard.Name))
			}
			resolvedName = validation.CanonicalCard.Name
			if validation.CanonicalCard.Number != "" {
				resolvedNumber = validation.CanonicalCard.Number
			}
			useNoStale = true
		}
	}

	// When we still have no collector number, try to extract one from the
	// PriceCharting product name (e.g., "Charizard ex #161" → "161") before
	// NormalizeCardName strips it. Runs regardless of whether cardProvider is set.
	if resolvedNumber == "" {
		if extracted := cardutil.ExtractCollectorNumber(pcPrice.ProductName); extracted != "" {
			resolvedNumber = extracted
		}
	}

	// Strip embedded card number from the resolved name before passing to the
	// fusion engine. PriceCharting product names include collector numbers
	// (e.g., "Charizard ex #161") that may refer to different physical cards in
	// other providers' databases — especially for promo sets where numbering
	// diverges between providers. The explicit
	// resolvedNumber (from the purchase or canonical card DB) is passed
	// separately and is the only number downstream providers should use.
	cleanResolvedName := cardutil.NormalizeCardName(resolvedName)

	// Get fused pricing using the resolved card identity.
	// Mark as on-demand so PriceCharting is excluded (already queried above).
	fusedCard := pricing.Card{
		Name:            cleanResolvedName,
		Number:          resolvedNumber,
		Set:             setName,
		PSAListingTitle: card.PSAListingTitle,
	}

	getCtx := withOnDemand(ctx)
	if useNoStale {
		getCtx = withNoStale(getCtx)
	}
	result, err := f.GetPrice(getCtx, fusedCard)
	if err != nil {
		// Fall back to PriceCharting-only result if fusion fails
		if f.logger != nil {
			hasPCPrice := pcPrice != nil && pcPrice.Grades.PSA10Cents > 0
			f.logger.Info(ctx, "fusion GetPrice failed, using PriceCharting fallback",
				observability.String("card", fusedCard.Name),
				observability.String("set", fusedCard.Set),
				observability.String("number", fusedCard.Number),
				observability.Bool("has_pc_price", hasPCPrice),
				observability.Err(err))
		}
		return pcPrice, nil
	}

	// Add PriceCharting data from the correctly resolved product.
	// GetPrice only fetched secondary sources (PP) — PC data comes from
	// LookupCard's own PriceCharting query which resolved the correct variant.
	applyPCData(result, pcPrice)

	// Clean up stale DB entries when the card name was normalized.
	f.cleanupStaleName(ctx, card.Name, fusedCard.Name, setName, resolvedNumber)
	f.cleanupStaleName(ctx, resolvedName, fusedCard.Name, setName, resolvedNumber)

	// Supplement from DB under original name when names differ — batch data
	// (CardHedger) may be stored under the original purchase name.
	if card.Name != fusedCard.Name && f.priceRepo != nil {
		originalCard := pricing.Card{Name: card.Name, Number: card.Number, Set: setName}
		fd := f.freshnessDuration
		if fd <= 0 {
			fd = DefaultFreshnessDuration
		}
		f.supplementCardHedgerFromDB(ctx, result, originalCard, fd)
	}

	return result, nil
}

// applyPCData merges PriceCharting-specific fields from pcPrice into result.
// Used by LookupCard to add PC data from the correctly resolved product.
func applyPCData(result, pcPrice *pricing.Price) {
	if pcPrice == nil || result == nil {
		return
	}
	result.ProductName = pcPrice.ProductName
	result.ID = pcPrice.ID
	result.PCGrades = &pcPrice.Grades
	result.LastSoldByGrade = pcPrice.LastSoldByGrade
	if pcPrice.Conservative != nil {
		result.Conservative = pcPrice.Conservative
	}
	if pcPrice.Market != nil {
		if result.Market == nil {
			result.Market = &pricing.MarketData{}
		}
		result.Market.SalesLast30d = pcPrice.Market.SalesLast30d
		result.Market.SalesLast90d = pcPrice.Market.SalesLast90d
		result.Market.ActiveListings = pcPrice.Market.ActiveListings
		result.Market.LowestListing = pcPrice.Market.LowestListing
		result.Market.ListingVelocity = pcPrice.Market.ListingVelocity
		result.Market.Volatility = pcPrice.Market.Volatility
	}
	// Ensure "pricecharting" is in Sources (GetPrice may not have added it)
	for _, s := range result.Sources {
		if s == pricing.SourcePriceCharting {
			return
		}
	}
	result.Sources = append(result.Sources, pricing.SourcePriceCharting)
}

// cleanupStaleName deletes price history stored under oldName when it differs
// from newName. Card ID mappings are NOT deleted — they're managed by the
// CardHedger batch scheduler and needed by the delta poll for card resolution.
func (f *FusionPriceProvider) cleanupStaleName(ctx context.Context, oldName, newName, setName, cardNumber string) {
	if oldName == newName {
		return
	}
	if f.priceRepo != nil {
		deleted, err := f.priceRepo.DeletePricesByCard(ctx, oldName, setName, cardNumber)
		if err != nil {
			if f.logger != nil {
				f.logger.Error(ctx, "failed to delete stale price entries",
					observability.Err(err),
					observability.String("old_name", oldName),
					observability.String("new_name", newName))
			}
		} else if deleted > 0 {
			if f.logger != nil {
				f.logger.Info(ctx, "deleted stale price entries",
					observability.String("old_name", oldName),
					observability.String("new_name", newName),
					observability.Int("deleted", int(deleted)))
			}
		}
	}
}
