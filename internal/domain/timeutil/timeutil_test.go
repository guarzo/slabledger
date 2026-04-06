package timeutil_test

import (
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/timeutil"
)

func TestDaysSince(t *testing.T) {
	tests := []struct {
		name    string
		date    string
		wantMin int
		wantMax int
	}{
		{
			name:    "today returns 0",
			date:    time.Now().Format("2006-01-02"),
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "10 days ago",
			date:    time.Now().AddDate(0, 0, -10).Format("2006-01-02"),
			wantMin: 10,
			wantMax: 10,
		},
		{
			name:    "invalid date returns 0",
			date:    "not-a-date",
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "empty string returns 0",
			date:    "",
			wantMin: 0,
			wantMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeutil.DaysSince(tt.date)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("DaysSince(%q) = %d, want [%d, %d]", tt.date, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}
