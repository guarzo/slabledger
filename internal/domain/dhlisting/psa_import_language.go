package dhlisting

import "strings"

// InferDHLanguage maps a SlabLedger set_name (and card_name as a backup) to DH's
// language enum. Returns the empty string for English (the DH default) so callers
// can omit the field entirely rather than overriding.
//
// This is used when submitting non-English certs via DH's psa_import endpoint,
// where the override must match DH's enum keys or the row fails as
// partner_card_error. Only high-confidence matches are returned — when in doubt
// we return "" and let DH's PSA metadata drive the language.
func InferDHLanguage(setName, cardName string) string {
	hay := strings.ToLower(setName + " " + cardName)
	switch {
	case strings.Contains(hay, "japanese"):
		return "japanese"
	case strings.Contains(hay, "german"):
		return "german"
	case strings.Contains(hay, "french"):
		return "french"
	case strings.Contains(hay, "italian"):
		return "italian"
	case strings.Contains(hay, "spanish"):
		return "spanish"
	case strings.Contains(hay, "korean"):
		return "korean"
	case strings.Contains(hay, "chinese"):
		// DH distinguishes Simplified from Traditional; a bare "chinese"
		// override is rejected as partner_card_error. Only return a value when
		// the script is explicit — otherwise defer to DH's PSA metadata.
		switch {
		case strings.Contains(hay, "simplified"):
			return "chinese_simplified"
		case strings.Contains(hay, "traditional"):
			return "chinese_traditional"
		}
		return ""
	}
	return ""
}
