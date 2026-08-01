package inventory

import (
	"math"
	"strconv"
	"strings"
)

// ParseCLConfidenceMin returns the truncated integer minimum of a CL confidence
// range like "2.5-4" (→ 2). ok is false when the input is empty or unparseable.
func ParseCLConfidenceMin(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	lo := strings.TrimSpace(strings.SplitN(s, "-", 2)[0])
	f, err := strconv.ParseFloat(lo, 64)
	if err != nil {
		return 0, false
	}
	return int(math.Trunc(f)), true
}
