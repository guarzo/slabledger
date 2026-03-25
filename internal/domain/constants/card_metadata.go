package constants

import (
	"strconv"
	"strings"
)

// VariantKeywords lists variant markers used in PSA listing titles, ordered
// longest-first so "REVERSE HOLO" is matched before "HOLO".
// Shared across import parsing and pricing adapter normalization.
var VariantKeywords = []string{"REVERSE HOLO", "REVERSE FOIL", "1ST EDITION", "HOLO", "1ST", "SHADOWLESS"}

// IsInvalidCardNumber returns true if the value looks like a variant keyword
// (e.g. "1ST" from "1ST EDITION") or a 4-digit year rather than a real
// collector number.
func IsInvalidCardNumber(num string) bool {
	num = strings.TrimSpace(num)
	if num == "" {
		return false
	}
	upper := strings.ToUpper(num)
	if upper == "1ST" || upper == "2ND" || upper == "3RD" || upper == "4TH" {
		return true
	}
	if len(num) == 4 {
		if y, err := strconv.Atoi(num); err == nil && y >= 1990 && y <= 2039 {
			return true
		}
	}
	return false
}
