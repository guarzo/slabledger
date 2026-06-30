package scheduler

import "time"

// timeUntilHour returns the duration from now until the next occurrence
// of the given hour (0-23) in UTC. If the hour already passed today,
// returns 0 so the scheduler runs immediately on startup (catching up
// after a machine restart that missed the window).
func timeUntilHour(now time.Time, hour int) time.Duration {
	nowUTC := now.UTC()
	target := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), hour, 0, 0, 0, time.UTC)
	if target.After(nowUTC) {
		return target.Sub(nowUTC)
	}
	return 0
}

// sameUTCDate reports whether a and b fall on the same calendar day in UTC.
// Used to gate daily schedulers so a redeploy doesn't replay a sweep that
// already completed earlier the same day.
func sameUTCDate(a, b time.Time) bool {
	au, bu := a.UTC(), b.UTC()
	return au.Year() == bu.Year() && au.Month() == bu.Month() && au.Day() == bu.Day()
}
