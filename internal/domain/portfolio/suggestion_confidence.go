package portfolio

import (
	"strconv"

	"github.com/guarzo/slabledger/internal/domain/mathutil"
)

// confidenceLabelWithAge returns a confidence string that decays based on data age.
// If the latest data is older than 6 months, confidence is reduced by one level.
func confidenceLabelWithAge(n int, latestSaleDate string, now string) string {
	base := mathutil.ConfidenceLabel(n)
	if latestSaleDate == "" || now == "" {
		return base
	}

	nowMonths := mustParseYearMonth(now)
	saleMonths := mustParseYearMonth(latestSaleDate)
	if nowMonths == 0 || saleMonths == 0 {
		return base
	}
	if nowMonths-saleMonths > 6 {
		switch base {
		case "high":
			return "medium"
		case "medium":
			return "low"
		}
	}
	return base
}

// mustParseYearMonth returns year*12+month from a YYYY-MM-DD string, or 0 on failure.
func mustParseYearMonth(date string) int {
	if len(date) < 7 {
		return 0
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	month, err := strconv.Atoi(date[5:7])
	if err != nil {
		return 0
	}
	return year*12 + month
}
