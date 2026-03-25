package constants

import "strings"

// IsGenericSetName returns true if the set name is too generic for reliable price lookups.
// Generic set names are placeholder values from imports that don't correspond to real TCG sets.
func IsGenericSetName(setName string) bool {
	lower := strings.ToLower(strings.TrimSpace(setName))
	switch lower {
	case "", "pokemon cards", "tcg cards", "cards", "pokemon", "trading cards", "other":
		return true
	}
	return false
}
