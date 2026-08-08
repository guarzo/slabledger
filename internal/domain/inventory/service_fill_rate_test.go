package inventory

import (
	"context"
	"errors"
	"testing"
)

func TestServiceGetDailySpend_Enriches(t *testing.T) {
	repo := newMockRepo()
	repo.campaigns["c1"] = &Campaign{ID: "c1", DailySpendCapCents: 1000}
	// Exactly what the postgres store returns: no cap, no fill rate.
	repo.dailySpend = []DailySpend{
		{Date: "2025-01-01", SpendCents: 500, PurchaseCount: 1},
		{Date: "2025-01-02", SpendCents: 250, PurchaseCount: 2},
	}
	svc := NewService(repo, repo, repo, repo, repo, repo, repo, WithIDGenerator(func() string { return "id" }))

	got, err := svc.GetDailySpend(context.Background(), "c1", 30)
	if err != nil {
		t.Fatalf("GetDailySpend: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	want := []DailySpend{
		{Date: "2025-01-01", SpendCents: 500, CapCents: 1000, FillRatePct: 0.5, PurchaseCount: 1},
		{Date: "2025-01-02", SpendCents: 250, CapCents: 1000, FillRatePct: 0.25, PurchaseCount: 2},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// A campaign lookup that does not yield a usable campaign must never fail the
// call: the endpoint returns 200 with zero caps today, and this path exists to
// preserve that. The log level is asserted because the handler being replaced
// in Task 3 distinguishes a deleted campaign (Warn) from a broken lookup
// (Error), and collapsing the two would be a silent observability regression.
func TestServiceGetDailySpend_CampaignUnavailable(t *testing.T) {
	errBroken := errors.New("connection refused")

	tests := []struct {
		name      string
		setup     func(*mockRepo)
		wantLevel string // "" means nothing should be logged
	}{
		{
			name:      "missing campaign logs at warn",
			setup:     func(_ *mockRepo) {}, // no campaign registered
			wantLevel: "warn",
		},
		{
			name:      "broken lookup logs at error",
			setup:     func(m *mockRepo) { m.campaignErr = errBroken },
			wantLevel: "error",
		},
		{
			name:      "nil campaign with no error is not dereferenced and is not logged",
			setup:     func(m *mockRepo) { m.campaigns["c1"] = nil },
			wantLevel: "",
		},
	}

	for _, tt := range tests {
		for _, withLogger := range []bool{true, false} {
			name := tt.name
			if !withLogger {
				name += " (nil logger)"
			}
			t.Run(name, func(t *testing.T) {
				repo := newMockRepo()
				repo.dailySpend = []DailySpend{{Date: "2025-01-01", SpendCents: 500, PurchaseCount: 1}}
				tt.setup(repo)

				lg := newTestLogger()
				opts := []ServiceOption{WithIDGenerator(func() string { return "id" })}
				if withLogger {
					opts = append(opts, WithLogger(lg))
				}
				svc := NewService(repo, repo, repo, repo, repo, repo, repo, opts...)

				got, err := svc.GetDailySpend(context.Background(), "c1", 30)
				if err != nil {
					t.Fatalf("GetDailySpend returned an error: %v", err)
				}
				if len(got) != 1 {
					t.Fatalf("len = %d, want 1", len(got))
				}
				want := DailySpend{Date: "2025-01-01", SpendCents: 500, CapCents: 0, FillRatePct: 0, PurchaseCount: 1}
				if got[0] != want {
					t.Errorf("row = %+v, want %+v", got[0], want)
				}
				if !withLogger {
					return // the point of this variant is that it does not panic
				}
				if gotLevels := lg.levels(); tt.wantLevel == "" {
					if len(gotLevels) != 0 {
						t.Errorf("logged %v, want nothing", gotLevels)
					}
				} else if len(gotLevels) != 1 || gotLevels[0] != tt.wantLevel {
					t.Errorf("logged %v, want exactly [%s]", gotLevels, tt.wantLevel)
				}
			})
		}
	}
}
