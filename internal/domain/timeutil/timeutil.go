package timeutil

import "time"

// DaysSince parses an ISO-8601 date ("2006-01-02") and returns the number
// of whole days elapsed since that date. Returns 0 if the date is invalid.
func DaysSince(dateStr string) int {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0
	}
	return int(time.Since(t).Hours() / 24)
}
