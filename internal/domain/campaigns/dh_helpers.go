package campaigns

import "strings"

// CleanCardNameForDH strips holo suffixes from PSA-style card names and
// returns the cleaned name (title-cased) plus a variant hint for DH cert resolution.
func CleanCardNameForDH(raw string) (name, variant string) {
	if raw == "" {
		return "", ""
	}

	s := raw
	if idx := strings.Index(s, "-REVERSE HOLO"); idx != -1 {
		rest := strings.TrimSpace(s[idx+len("-REVERSE HOLO"):])
		s = s[:idx]
		if rest != "" {
			s += " " + rest
		}
		variant = "Reverse Holo"
	} else if idx := strings.Index(s, "-HOLO"); idx != -1 {
		rest := strings.TrimSpace(s[idx+len("-HOLO"):])
		s = s[:idx]
		if rest != "" {
			s += " " + rest
		}
		variant = "Holo"
	}

	name = strings.Join(strings.Fields(toTitleCase(s)), " ")
	return name, variant
}

// toTitleCase converts "DRAGONITE 1ST EDITION" → "Dragonite 1st Edition".
func toTitleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
