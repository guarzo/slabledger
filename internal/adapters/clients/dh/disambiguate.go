package dh

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Disambiguate attempts to select a single candidate from an ambiguous cert
// resolution using the submitted card_number hint. Returns the matching
// candidate's DHCardID if exactly one candidate's card_number matches
// (after stripping leading zeros), or 0 if disambiguation fails.
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

// normalizeCardNumber strips leading zeros, preserving a single "0" for
// all-zero inputs (e.g. "000" → "0").
func normalizeCardNumber(s string) string {
	n := strings.TrimLeft(s, "0")
	if n == "" && len(s) > 0 {
		return "0"
	}
	return n
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
