package inventory

import (
	"strconv"
	"strings"

	"github.com/guarzo/slabledger/internal/platform/cardutil"
)

// ParseRange parses a "min-max" range string into its integer bounds.
// Returns (0, 0, false) if the string is empty or malformed.
func ParseRange(s string) (lo, hi int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	// Normalize common Unicode dashes to ASCII hyphen
	s = strings.ReplaceAll(s, "–", "-") // en dash
	s = strings.ReplaceAll(s, "—", "-") // em dash
	s = strings.ReplaceAll(s, "‒", "-") // figure dash
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

// MatchInput is the purchase-side half of a campaign match.
type MatchInput struct {
	Grade        float64
	BuyCostCents int
	CardName     string
	SetName      string
	CardNumber   string // within-set number, from PSA
	PSASpecID    int    // Card Ladder spec id; 0 when unmapped
	CardYear     int
}

// PurchaseMatchesCampaign checks whether a purchase's attributes satisfy all
// of a campaign's defined filter criteria. Unset criteria are treated as
// wildcards (match anything). Evaluation order: year, grade, price (scaled by
// BuyTermsCLPct), language axis, subject axis, then the card-level deny list.
func PurchaseMatchesCampaign(in MatchInput, c *Campaign) bool {
	// Year range check — matches the card's release year against the campaign's year range.
	// This is the primary disambiguator: a 1999 card goes to a vintage campaign, not modern.
	if c.YearRange != "" && in.CardYear > 0 {
		lo, hi, ok := ParseRange(c.YearRange)
		if !ok {
			return false
		}
		if in.CardYear < lo || in.CardYear > hi {
			return false
		}
	}

	// Grade range check (supports half-grades: 9.5 matches range "9-10")
	if c.GradeRange != "" {
		lo, hi, ok := ParseRange(c.GradeRange)
		if !ok {
			return false
		}
		if in.Grade < float64(lo) || in.Grade > float64(hi) {
			return false
		}
	}

	// Price range check — campaign range is the card's market value in dollars,
	// but BuyCostCents is what was actually paid (market value * buyTermsPct).
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
		if in.BuyCostCents < loCents || in.BuyCostCents > hiCents {
			return false
		}
	}

	if !LanguageAxisMatches(in.SetName, c.TargetLanguage) {
		return false
	}

	if !SubjectAxisMatches(in.CardName, c.Subjects, c.SubjectFilterMode) {
		return false
	}

	if SpecDenied(in, c.DeniedSpecs) {
		return false
	}

	return true
}

// LanguageAxisMatches reports whether a set name satisfies the language axis.
// An empty targetLanguage is an open net and always matches.
func LanguageAxisMatches(setName, targetLanguage string) bool {
	if targetLanguage == "" {
		return true
	}
	return cardutil.SetLanguage(setName) == targetLanguage
}

// SubjectAxisMatches reports whether a card name satisfies the subject axis.
// An empty subjects list is an open net and always matches, regardless of
// mode. Matching is a case-insensitive substring test of each subject's name
// against the card name.
func SubjectAxisMatches(cardName string, subjects []TargetSubject, mode string) bool {
	if len(subjects) == 0 {
		return true
	}
	lowerCard := strings.ToLower(cardName)
	matched := false
	for _, s := range subjects {
		if s.Name == "" {
			continue
		}
		if strings.Contains(lowerCard, strings.ToLower(s.Name)) {
			matched = true
			break
		}
	}
	if mode == SubjectFilterExclude {
		return !matched
	}
	return matched
}

// SpecDenied reports whether the card is on the campaign's card-level deny
// list. Identity is ID-first: if both in.PSASpecID and the denied entry's ID
// are non-zero, deny on equality. PSASpecID is CL-sourced and omitempty, so it
// is frequently 0 — the ID path is the exception, not the common case.
//
// The common-case fallback compares a composite key built from the
// purchase's normalized set name (cardutil.NormalizeSetNameSimple) and card
// number against the denied entry's Name. TargetSubject carries only
// {ID, Name} — there is no separate set/number field on a denied entry to
// compare against — so this fallback treats a name-only denied entry's Name
// as recorded in that same "<normalized set name> <card number>" form and
// requires an exact, case-insensitive match on the whole composite key. That
// is the most conservative reading available: this plan is defining the
// convention (there is no existing denied-entry data to reverse-engineer it
// from), and an exact composite match can only under-deny relative to any
// substring or fuzzy comparison, never over-deny. If either half of the
// composite key is missing (empty set name or card number), no key is built
// and that entry cannot deny by name.
//
// When neither identity — PSASpecID/ID, or the set+number composite — is
// available, the entry does not deny the purchase. That fail-open direction
// is deliberate: this predicate does not gate buying (the portal enforces
// denials at buy time); it only attributes an already-purchased card to a
// campaign after the fact. A false deny would silently misattribute a
// legitimate purchase and distort every downstream analytic, while a missed
// deny merely attributes a card that shouldn't have been bought — visible
// and correctable. Do not "fix" this into failing closed.
func SpecDenied(in MatchInput, denied []TargetSubject) bool {
	key := specDenyKey(in.SetName, in.CardNumber)
	for _, d := range denied {
		if in.PSASpecID != 0 && d.ID != 0 {
			if in.PSASpecID == d.ID {
				return true
			}
			continue
		}
		if key != "" && strings.EqualFold(strings.TrimSpace(d.Name), key) {
			return true
		}
	}
	return false
}

// specDenyKey builds the composite fallback identity used by SpecDenied when
// PSASpecID is unavailable: the set name normalized via
// cardutil.NormalizeSetNameSimple, plus the card number, space-joined.
// Returns "" if either half is missing, since a partial key is not a
// reliable identity and must not be allowed to match anything.
func specDenyKey(setName, cardNumber string) string {
	normSet := strings.TrimSpace(cardutil.NormalizeSetNameSimple(setName))
	cardNumber = strings.TrimSpace(cardNumber)
	if normSet == "" || cardNumber == "" {
		return ""
	}
	return normSet + " " + cardNumber
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

// MatchResult describes the outcome of matching a purchase against all inventory.
type MatchResult struct {
	CampaignID string   // Set when exactly one campaign matches
	Candidates []string // Set when multiple campaigns match (ambiguous)
	Status     string   // "matched", "unmatched", "ambiguous"
}

// FindMatchingCampaign evaluates a purchase against all provided campaigns and
// returns the matching campaign. If exactly one campaign matches, it is returned.
// If zero match, status is "unmatched". If multiple match, status is "ambiguous"
// with candidate IDs listed.
func FindMatchingCampaign(in MatchInput, allCampaigns []Campaign) MatchResult {
	var matches []string
	for i := range allCampaigns {
		if PurchaseMatchesCampaign(in, &allCampaigns[i]) {
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
