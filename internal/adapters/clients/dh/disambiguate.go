package dh

import "strings"

// Disambiguate attempts to select a single candidate from an ambiguous cert
// resolution using the submitted card_number hint. Returns the matching
// candidate's DHCardID if exactly one candidate's card_number matches
// (after stripping leading zeros), or 0 if disambiguation fails.
func Disambiguate(candidates []CertResolutionCandidate, cardNumber string) int {
	normalized := strings.TrimLeft(cardNumber, "0")
	if normalized == "" || len(candidates) == 0 {
		return 0
	}

	var matchID int
	matches := 0
	for _, c := range candidates {
		if strings.TrimLeft(c.CardNumber, "0") == normalized {
			matchID = c.DHCardID
			matches++
		}
	}

	if matches == 1 {
		return matchID
	}
	return 0
}
