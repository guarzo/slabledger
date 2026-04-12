package inventory

import (
	"strconv"
	"strings"
)

// ParseRange parses a "min-max" range string into its integer bounds.
// Returns (0, 0, false) if the string is empty or malformed.
func ParseRange(s string) (lo, hi int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	// Normalize common Unicode dashes to ASCII hyphen
	s = strings.ReplaceAll(s, "\u2013", "-") // en dash
	s = strings.ReplaceAll(s, "\u2014", "-") // em dash
	s = strings.ReplaceAll(s, "\u2012", "-") // figure dash
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	var err error
	lo, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	hi, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	if lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

// PurchaseMatchesCampaign checks whether a purchase's attributes satisfy all
// of a campaign's defined filter criteria. Unset criteria are treated as
// wildcards (match anything). If any set criterion is violated, returns false.
// cardYear is the card's release year (e.g., 2000 for a Base Set card); 0 means unknown.
func PurchaseMatchesCampaign(grade float64, buyCostCents int, cardName string, setName string, cardYear int, c *Campaign) bool {
	// Year range check — matches the card's release year against the campaign's year range.
	// This is the primary disambiguator: a 1999 card goes to a vintage campaign, not modern.
	if c.YearRange != "" && cardYear > 0 {
		lo, hi, ok := ParseRange(c.YearRange)
		if !ok {
			return false
		}
		if cardYear < lo || cardYear > hi {
			return false
		}
	}

	// Grade range check (supports half-grades: 9.5 matches range "9-10")
	if c.GradeRange != "" {
		lo, hi, ok := ParseRange(c.GradeRange)
		if !ok {
			return false
		}
		if grade < float64(lo) || grade > float64(hi) {
			return false
		}
	}

	// Price range check — campaign range is the card's market value in dollars,
	// but buyCostCents is what was actually paid (market value * buyTermsPct).
	// Scale the range by BuyTermsCLPct so we compare apples to apples.
	if c.PriceRange != "" {
		lo, hi, ok := ParseRange(c.PriceRange)
		if !ok {
			return false
		}
		buyPct := c.BuyTermsCLPct
		if buyPct <= 0 || buyPct > 1 {
			buyPct = 1 // no scaling if unset or invalid
		}
		loCents := int(float64(lo*100) * buyPct)
		hiCents := int(float64(hi*100) * buyPct)
		if buyCostCents < loCents || buyCostCents > hiCents {
			return false
		}
	}

	// Inclusion/exclusion list check
	if c.InclusionList != "" {
		entries := SplitInclusionList(c.InclusionList)
		matched := inclusionListMatches(cardName, setName, entries)
		if c.ExclusionMode {
			// Exclusion mode: if any entry matches, this campaign rejects the purchase
			if matched {
				return false
			}
		} else {
			// Inclusion mode: at least one entry must match
			if !matched {
				return false
			}
		}
	}

	return true
}

// SplitInclusionList splits an inclusion/exclusion list string into individual
// entries. It supports both comma-separated ("charizard,pikachu") and
// space-separated ("charizard pikachu") formats, as well as mixed usage.
func SplitInclusionList(s string) []string {
	// First split by commas
	parts := strings.Split(s, ",")
	var entries []string
	for _, part := range parts {
		// Then split each part by whitespace
		for _, word := range strings.Fields(part) {
			if word != "" {
				entries = append(entries, word)
			}
		}
	}
	return entries
}

// inclusionListMatches returns true if any entry in the list is a case-insensitive
// substring of either the card name or set name.
func inclusionListMatches(cardName string, setName string, entries []string) bool {
	lowerCard := strings.ToLower(cardName)
	lowerSet := strings.ToLower(setName)
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		lowerEntry := strings.ToLower(entry)
		if strings.Contains(lowerCard, lowerEntry) || strings.Contains(lowerSet, lowerEntry) {
			return true
		}
	}
	return false
}

// MatchResult describes the outcome of matching a purchase against all inventory.
type MatchResult struct {
	CampaignID string   // Set when exactly one campaign matches
	Candidates []string // Set when multiple campaigns match (ambiguous)
	Status     string   // "matched", "unmatched", "ambiguous"
}

// FindMatchingCampaign evaluates a purchase against all provided campaigns and
// returns the matching campaign. If exactly one campaign matches, it is returned.
// If zero match, status is "unmatched". If multiple match, status is "ambiguous"
// with candidate IDs listed. cardYear is the card's release year (0 if unknown).
func FindMatchingCampaign(grade float64, buyCostCents int, cardName string, setName string, cardYear int, allCampaigns []Campaign) MatchResult {
	var matches []string
	for i := range allCampaigns {
		if PurchaseMatchesCampaign(grade, buyCostCents, cardName, setName, cardYear, &allCampaigns[i]) {
			matches = append(matches, allCampaigns[i].ID)
		}
	}

	switch len(matches) {
	case 0:
		return MatchResult{Status: "unmatched"}
	case 1:
		return MatchResult{CampaignID: matches[0], Status: "matched"}
	default:
		return MatchResult{Candidates: matches, Status: "ambiguous"}
	}
}
