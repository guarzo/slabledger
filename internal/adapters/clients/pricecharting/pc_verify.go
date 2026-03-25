package pricecharting

import (
	"context"
	"strings"

	"github.com/guarzo/slabledger/internal/adapters/clients/cardutil"
)

// VerifyProductMatch checks whether expectedNumber appears somewhere in the
// PriceCharting ProductName string. This is a quick, lightweight guard used
// before the heavier CardProvider validation to reject obvious mismatches
// from fuzzy search results.
func VerifyProductMatch(_ context.Context, productName, expectedNumber string) bool {
	if expectedNumber == "" {
		return true // nothing to verify
	}
	normalized := cardutil.NormalizeCardNumber(expectedNumber)
	if normalized == "" {
		return true
	}
	// Tokenize and normalize each token to compare against the expected number.
	// This handles "#004" matching "4", "4/102" matching "4", etc.
	// Iterate over original-case tokens so NormalizeCardNumber preserves letter prefixes
	// (e.g., "GG42" vs "gg42" after lowercasing).
	//
	// Fallback within the same loop: compare numeric portions only when both sides
	// have the same prefix situation (both have a letter prefix or both are pure
	// digits). This handles format normalization differences without creating false
	// positives like "75" matching an unrelated "SWSH75".
	expectedDigits := cardutil.NumericOnly(normalized)
	expectedHasPrefix := expectedDigits != normalized
	for _, token := range strings.Fields(productName) {
		clean := strings.TrimPrefix(token, "#")
		normToken := cardutil.NormalizeCardNumber(clean)
		if normToken == normalized {
			return true
		}
		if expectedDigits != "" {
			tokenDigits := cardutil.NumericOnly(normToken)
			tokenHasPrefix := tokenDigits != normToken
			if tokenDigits == expectedDigits && expectedHasPrefix == tokenHasPrefix {
				return true
			}
		}
	}

	return false
}

// VerifySetOverlap checks whether the PriceCharting consoleName shares at
// least one significant word (>= 3 chars) with the expected set name.
func VerifySetOverlap(_ context.Context, consoleName, expectedSetName string) bool {
	return cardutil.MatchesSetOverlap(consoleName, expectedSetName)
}
