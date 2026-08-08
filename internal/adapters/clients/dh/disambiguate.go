package dh

import (
	"encoding/json"
	"fmt"

	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// Disambiguate attempts to select a single candidate from an ambiguous cert
// resolution using the submitted card_number hint. Returns the matching
// candidate's DHCardID if exactly one candidate's card_number matches
// (after normalizing away any denominator and leading zeros), or 0 if
// disambiguation fails.
func Disambiguate(candidates []CertResolutionCandidate, cardNumber string) int {
	normalized := normalizeCardNumber(cardNumber)
	if normalized == "" || len(candidates) == 0 {
		return 0
	}

	var matchID int
	matches := 0
	for _, c := range candidates {
		if normalizeCardNumber(c.CardNumber) == normalized {
			matchID = c.DHCardID
			matches++
		}
	}

	if matches == 1 {
		return matchID
	}
	return 0
}

// normalizeCardNumber reduces a collector number to its comparable form:
// the denominator is dropped and leading zeros are stripped, preserving a
// single "0" for all-zero inputs (e.g. "199/165" → "199", "000" → "0").
// Purchase card numbers parsed from PSA listing titles carry the
// "199/165" shape, while DH candidates report bare integers, so both sides
// must go through the same normalizer for disambiguation to match.
func normalizeCardNumber(s string) string {
	return cardutil.NormalizeCardNumber(s)
}

// ResolveAmbiguous tries card-number disambiguation on ambiguous candidates.
// Returns the matched DHCardID (>0) on success. On failure, marshals
// candidates to JSON and passes them to saveFn (if non-nil), then returns 0.
func ResolveAmbiguous(candidates []CertResolutionCandidate, cardNumber string, saveFn func(candidatesJSON string) error) (int, error) {
	if id := Disambiguate(candidates, cardNumber); id > 0 {
		return id, nil
	}
	if saveFn != nil {
		b, err := json.Marshal(candidates)
		if err != nil {
			return 0, fmt.Errorf("marshal candidates: %w", err)
		}
		if err := saveFn(string(b)); err != nil {
			return 0, err
		}
	}
	return 0, nil
}
