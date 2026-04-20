package postgres

import (
	"time"
)

// parseTimestamp parses a timestamp string into time.Time, tolerating both
// RFC3339 and the bare "2006-01-02 15:04:05" format that SQLite historically
// produced. Kept for rows that hold timestamps in TEXT columns (e.g. purchase
// dates) and for migration-era data that originated in SQLite. Returns the
// zero time if neither layout matches.
//
// Named parseTimestamp rather than parseSQLiteTime because the Postgres
// adapter doesn't care which DB produced the string.
func parseTimestamp(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t
	}
	return time.Time{}
}

// nullIfEmpty returns nil (SQL NULL) when s is empty, otherwise s.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// zeroAsNull returns nil (SQL NULL) when i is zero, otherwise i.
func zeroAsNull(i int) any {
	if i == 0 {
		return nil
	}
	return i
}
