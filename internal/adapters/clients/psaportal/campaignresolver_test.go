package psaportal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/psacampaign"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func newTestResolver(t *testing.T, portal []psacampaign.PortalCampaign, internal []inventory.Campaign, fetchedAt time.Time) *CampaignResolver {
	t.Helper()
	snap := &mocks.SnapshotStoreMock{
		GetSnapshotFn: func(_ context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
			return portal, fetchedAt, nil
		},
	}
	campaigns := &mocks.CampaignRepositoryMock{
		ListCampaignsFn: func(_ context.Context, _ bool) ([]inventory.Campaign, error) {
			return internal, nil
		},
	}
	return NewCampaignResolver(snap, campaigns, func() time.Time { return time.Now() })
}

func TestCampaignResolver_ResolveCampaignID(t *testing.T) {
	portal := []psacampaign.PortalCampaign{
		{CampaignRequestID: "req-modern", Name: "Modern"},
		{CampaignRequestID: "req-crystal", Name: "Crystal"},
	}
	internal := []inventory.Campaign{
		{ID: "camp-1", Name: "Modern", PSACampaignRequestID: "req-modern"},
		{ID: "camp-2", Name: "Crystal Pokemon", PSACampaignRequestID: "req-crystal"},
	}

	tests := []struct {
		name    string
		psaName string
		wantID  string
		wantOK  bool
	}{
		{"exact match", "Modern", "camp-1", true},
		{"case drift", "modern", "camp-1", true},
		{"whitespace drift", "  Modern  ", "camp-1", true},
		{"name drift via request id", "Crystal", "camp-2", true},
		{"dead campaign name", "Brady modern", "", false},
		{"empty name", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver(t, portal, internal, time.Now())
			gotID, gotOK, err := r.ResolveCampaignID(context.Background(), tt.psaName)
			if err != nil {
				t.Fatalf("ResolveCampaignID: %v", err)
			}
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Errorf("= (%q, %v), want (%q, %v)", gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestCampaignResolver_RefusesStaleSnapshot(t *testing.T) {
	stale := time.Now().Add(-27 * time.Hour)
	r := newTestResolver(t, []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}},
		[]inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}}, stale)
	_, _, err := r.ResolveCampaignID(context.Background(), "Modern")
	if !errors.Is(err, ErrStaleCampaignSnapshot) {
		t.Fatalf("err = %v, want ErrStaleCampaignSnapshot", err)
	}
}

func TestCampaignResolver_UnlinkedCampaignNeverMatches(t *testing.T) {
	portal := []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}}
	internal := []inventory.Campaign{{ID: "camp-1", Name: "Other", PSACampaignRequestID: ""}}
	r := newTestResolver(t, portal, internal, time.Now())
	gotID, gotOK, err := r.ResolveCampaignID(context.Background(), "Modern")
	if err != nil {
		t.Fatalf("ResolveCampaignID: %v", err)
	}
	if gotOK || gotID != "" {
		t.Fatalf("= (%q, %v), want (\"\", false) — unlinked campaign must never match", gotID, gotOK)
	}
}

// countingStores builds a resolver whose backing reads are counted, so the tests
// can assert both memoization (one read per resolver) and freshness (a new
// resolver re-reads and sees new data).
type countingStores struct {
	snapshotCalls int
	listCalls     int
	portal        []psacampaign.PortalCampaign
	internal      []inventory.Campaign
	fetchedAt     time.Time
}

func (c *countingStores) resolver(now func() time.Time) *CampaignResolver {
	snap := &mocks.SnapshotStoreMock{
		GetSnapshotFn: func(_ context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
			c.snapshotCalls++
			return c.portal, c.fetchedAt, nil
		},
	}
	campaigns := &mocks.CampaignRepositoryMock{
		ListCampaignsFn: func(_ context.Context, _ bool) ([]inventory.Campaign, error) {
			c.listCalls++
			return c.internal, nil
		},
	}
	return NewCampaignResolver(snap, campaigns, now)
}

func TestCampaignResolver_MemoizesReadsWithinOneResolver(t *testing.T) {
	c := &countingStores{
		portal:    []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}},
		internal:  []inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}},
		fetchedAt: time.Now(),
	}
	r := c.resolver(time.Now)

	for i := range 5 {
		gotID, gotOK, err := r.ResolveCampaignID(context.Background(), "Modern")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if gotID != "camp-1" || !gotOK {
			t.Fatalf("call %d = (%q, %v), want (camp-1, true)", i, gotID, gotOK)
		}
	}
	if c.snapshotCalls != 1 {
		t.Errorf("GetSnapshot called %d times across 5 resolves, want 1", c.snapshotCalls)
	}
	if c.listCalls != 1 {
		t.Errorf("ListCampaigns called %d times across 5 resolves, want 1", c.listCalls)
	}
}

// A new resolver is a new run: it must re-read both stores rather than serving
// data memoized by a previous resolver.
func TestCampaignResolver_MemoizationIsPerResolver(t *testing.T) {
	c := &countingStores{
		portal:    []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}},
		internal:  []inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}},
		fetchedAt: time.Now(),
	}
	if _, _, err := c.resolver(time.Now).ResolveCampaignID(context.Background(), "Modern"); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Portal renames the campaign and it is relinked to a different internal row.
	c.portal = []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern-v2", Name: "Modern"}}
	c.internal = []inventory.Campaign{{ID: "camp-2", PSACampaignRequestID: "req-modern-v2"}}
	c.fetchedAt = time.Now()

	gotID, gotOK, err := c.resolver(time.Now).ResolveCampaignID(context.Background(), "Modern")
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if gotID != "camp-2" || !gotOK {
		t.Fatalf("second run = (%q, %v), want (camp-2, true) — stale data carried across resolvers", gotID, gotOK)
	}
	if c.snapshotCalls != 2 || c.listCalls != 2 {
		t.Errorf("reads per run = (snapshot %d, list %d), want (2, 2)", c.snapshotCalls, c.listCalls)
	}
}

// Memoizing the reads must not memoize the freshness verdict: the age check runs
// on every call against the current clock.
func TestCampaignResolver_StalenessRecheckedAfterMemoization(t *testing.T) {
	fetchedAt := time.Now()
	c := &countingStores{
		portal:    []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}},
		internal:  []inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}},
		fetchedAt: fetchedAt,
	}
	now := fetchedAt
	r := c.resolver(func() time.Time { return now })

	if _, ok, err := r.ResolveCampaignID(context.Background(), "Modern"); err != nil || !ok {
		t.Fatalf("first call = (%v, %v), want resolved", ok, err)
	}

	now = fetchedAt.Add(27 * time.Hour)
	if _, _, err := r.ResolveCampaignID(context.Background(), "Modern"); !errors.Is(err, ErrStaleCampaignSnapshot) {
		t.Fatalf("err = %v, want ErrStaleCampaignSnapshot once the memoized snapshot ages out", err)
	}
}

// A failed read is not memoized: the resolver must retry rather than turn one
// transient error into a whole dead run.
func TestCampaignResolver_SnapshotErrorNotMemoized(t *testing.T) {
	boom := errors.New("db down")
	var calls int
	snap := &mocks.SnapshotStoreMock{
		GetSnapshotFn: func(_ context.Context) ([]psacampaign.PortalCampaign, time.Time, error) {
			calls++
			if calls == 1 {
				return nil, time.Time{}, boom
			}
			return []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}}, time.Now(), nil
		},
	}
	campaigns := &mocks.CampaignRepositoryMock{
		ListCampaignsFn: func(_ context.Context, _ bool) ([]inventory.Campaign, error) {
			return []inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}}, nil
		},
	}
	r := NewCampaignResolver(snap, campaigns, time.Now)

	// A read failure is a real error, never (ok=false, err=nil).
	gotID, gotOK, err := r.ResolveCampaignID(context.Background(), "Modern")
	if !errors.Is(err, boom) {
		t.Fatalf("first call err = %v, want %v", err, boom)
	}
	if gotOK || gotID != "" {
		t.Fatalf("first call = (%q, %v), want (\"\", false) alongside the error", gotID, gotOK)
	}

	gotID, gotOK, err = r.ResolveCampaignID(context.Background(), "Modern")
	if err != nil || !gotOK || gotID != "camp-1" {
		t.Fatalf("retry = (%q, %v, %v), want (camp-1, true, nil)", gotID, gotOK, err)
	}
}

// The app builds one resolver at startup and reuses it for the process's whole
// uptime. Memoizing for the resolver's lifetime would freeze the snapshot's
// fetchedAt, so every import past maxSnapshotAge would fall back to inference
// forever. Once memoTTL elapses the resolver must re-read and see the refreshed
// snapshot.
func TestCampaignResolver_RereadsSnapshotAfterTTL(t *testing.T) {
	start := time.Now()
	c := &countingStores{
		portal:    []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}},
		internal:  []inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}},
		fetchedAt: start,
	}
	now := start
	r := c.resolver(func() time.Time { return now })

	gotID, gotOK, err := r.ResolveCampaignID(context.Background(), "Modern")
	if err != nil || !gotOK || gotID != "camp-1" {
		t.Fatalf("first call = (%q, %v, %v), want (camp-1, true, nil)", gotID, gotOK, err)
	}

	// A day later psa-harvest has refreshed the snapshot and relinked the
	// portal campaign to a different internal row.
	now = start.Add(30 * time.Hour)
	c.portal = []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern-v2", Name: "Modern"}}
	c.internal = []inventory.Campaign{{ID: "camp-2", PSACampaignRequestID: "req-modern-v2"}}
	c.fetchedAt = now.Add(-1 * time.Hour)

	gotID, gotOK, err = r.ResolveCampaignID(context.Background(), "Modern")
	if err != nil {
		t.Fatalf("second call: %v — a latched fetchedAt makes the snapshot look stale forever", err)
	}
	if gotID != "camp-2" || !gotOK {
		t.Fatalf("second call = (%q, %v), want (camp-2, true) — memoized reads never expired", gotID, gotOK)
	}
	if c.snapshotCalls != 2 {
		t.Errorf("GetSnapshot called %d times, want 2 (one per TTL window)", c.snapshotCalls)
	}
	if c.listCalls != 2 {
		t.Errorf("ListCampaigns called %d times, want 2 (one per TTL window)", c.listCalls)
	}
}

// A campaign linked after the resolver's first read must become resolvable once
// the memo expires, without restarting the process.
func TestCampaignResolver_SeesCampaignCreatedAfterFirstResolve(t *testing.T) {
	start := time.Now()
	c := &countingStores{
		portal: []psacampaign.PortalCampaign{
			{CampaignRequestID: "req-modern", Name: "Modern"},
			{CampaignRequestID: "req-crystal", Name: "Crystal"},
		},
		internal:  []inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}},
		fetchedAt: start,
	}
	now := start
	r := c.resolver(func() time.Time { return now })

	if gotID, gotOK, err := r.ResolveCampaignID(context.Background(), "Crystal"); err != nil || gotOK || gotID != "" {
		t.Fatalf("before creation = (%q, %v, %v), want (\"\", false, nil)", gotID, gotOK, err)
	}

	// The user creates and links the Crystal campaign.
	c.internal = append(c.internal, inventory.Campaign{ID: "camp-9", PSACampaignRequestID: "req-crystal"})

	// Still inside the TTL: the memoized list has not expired yet.
	now = start.Add(memoTTL / 2)
	if gotID, gotOK, _ := r.ResolveCampaignID(context.Background(), "Crystal"); gotOK || gotID != "" {
		t.Fatalf("within TTL = (%q, %v), want (\"\", false) — memo should still be serving", gotID, gotOK)
	}

	now = start.Add(memoTTL + time.Second)
	gotID, gotOK, err := r.ResolveCampaignID(context.Background(), "Crystal")
	if err != nil {
		t.Fatalf("after TTL: %v", err)
	}
	if gotID != "camp-9" || !gotOK {
		t.Fatalf("after TTL = (%q, %v), want (camp-9, true) — campaign list never re-read", gotID, gotOK)
	}
	if c.listCalls != 2 {
		t.Errorf("ListCampaigns called %d times, want 2", c.listCalls)
	}
}

// The per-pass win must survive the TTL: an import pass over many rows completes
// well inside memoTTL and must still do exactly one read of each store.
func TestCampaignResolver_OneReadPerPassWithinTTL(t *testing.T) {
	start := time.Now()
	c := &countingStores{
		portal:    []psacampaign.PortalCampaign{{CampaignRequestID: "req-modern", Name: "Modern"}},
		internal:  []inventory.Campaign{{ID: "camp-1", PSACampaignRequestID: "req-modern"}},
		fetchedAt: start,
	}
	now := start
	r := c.resolver(func() time.Time { return now })

	const rows = 1500
	for i := range rows {
		// A whole pass spread across most of the TTL.
		now = start.Add(time.Duration(i) * memoTTL / (2 * rows))
		gotID, gotOK, err := r.ResolveCampaignID(context.Background(), "Modern")
		if err != nil || !gotOK || gotID != "camp-1" {
			t.Fatalf("row %d = (%q, %v, %v), want (camp-1, true, nil)", i, gotID, gotOK, err)
		}
	}
	if c.snapshotCalls != 1 {
		t.Errorf("GetSnapshot called %d times across %d rows, want 1", c.snapshotCalls, rows)
	}
	if c.listCalls != 1 {
		t.Errorf("ListCampaigns called %d times across %d rows, want 1", c.listCalls, rows)
	}
}
