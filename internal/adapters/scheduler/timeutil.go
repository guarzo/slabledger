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
