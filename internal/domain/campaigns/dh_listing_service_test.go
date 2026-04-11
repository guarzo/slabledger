package campaigns

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// --- Test doubles for DHListingService dependencies ---

type stubPurchaseLookup struct {
	result map[string]*Purchase
	err    error
}

func (s *stubPurchaseLookup) GetPurchasesByCertNumbers(_ context.Context, _ []string) (map[string]*Purchase, error) {
	return s.result, s.err
}

type stubCertResolver struct {
	resp *DHCertResolution
	err  error
}

func (s *stubCertResolver) ResolveCert(_ context.Context, _ DHCertResolveRequest) (*DHCertResolution, error) {
	return s.resp, s.err
}

type stubInventoryPusher struct {
	resp *DHInventoryPushResult
	err  error
}

func (s *stubInventoryPusher) PushInventory(_ context.Context, _ []DHInventoryPushItem) (*DHInventoryPushResult, error) {
	return s.resp, s.err
}

type stubInventoryLister struct {
	updateStatusErr error
	syncErr         error
	statusCalls     []statusCall
	syncCalls       []syncCall
}

type statusCall struct {
	inventoryID int
	status      string
}

type syncCall struct {
	inventoryID int
	channels    []string
}

func (s *stubInventoryLister) UpdateInventoryStatus(_ context.Context, inventoryID int, status string) error {
	s.statusCalls = append(s.statusCalls, statusCall{inventoryID, status})
	return s.updateStatusErr
}

func (s *stubInventoryLister) SyncChannels(_ context.Context, inventoryID int, channels []string) error {
	s.syncCalls = append(s.syncCalls, syncCall{inventoryID, channels})
	return s.syncErr
}

// --- normalizeCardNum tests ---

func TestNormalizeCardNum(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"0", "0"},
		{"00", "0"},
		{"000", "0"},
		{"1", "1"},
		{"01", "1"},
		{"001", "1"},
		{"0001", "1"},
		{"42", "42"},
		{"042", "42"},
		{"0042", "42"},
		{"100", "100"},
		{"0100", "100"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeCardNum(tc.input)
			if got != tc.want {
				t.Errorf("normalizeCardNum(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- disambiguateByCardNumber tests ---

func TestDisambiguateByCardNumber(t *testing.T) {
	tests := []struct {
		name       string
		candidates []DHCertCandidate
		cardNumber string
		wantID     int
	}{
		{
			name:       "empty candidates returns 0",
			candidates: nil,
			cardNumber: "42",
			wantID:     0,
		},
		{
			name:       "empty card number returns 0",
			candidates: []DHCertCandidate{{DHCardID: 1, CardNumber: "42"}},
			cardNumber: "",
			wantID:     0,
		},
		{
			name: "exact match single candidate",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "42"},
				{DHCardID: 20, CardNumber: "99"},
			},
			cardNumber: "42",
			wantID:     10,
		},
		{
			name: "normalized match with leading zeros",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "042"},
				{DHCardID: 20, CardNumber: "99"},
			},
			cardNumber: "42",
			wantID:     10,
		},
		{
			name: "normalized match both sides have zeros",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "0042"},
				{DHCardID: 20, CardNumber: "99"},
			},
			cardNumber: "042",
			wantID:     10,
		},
		{
			name: "multiple matches returns 0",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "42"},
				{DHCardID: 20, CardNumber: "042"},
			},
			cardNumber: "42",
			wantID:     0,
		},
		{
			name: "no matches returns 0",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "1"},
				{DHCardID: 20, CardNumber: "2"},
			},
			cardNumber: "42",
			wantID:     0,
		},
		{
			name: "all-zero card number matches all-zero candidate",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "000"},
				{DHCardID: 20, CardNumber: "1"},
			},
			cardNumber: "0",
			wantID:     10,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := disambiguateByCardNumber(tc.candidates, tc.cardNumber)
			if got != tc.wantID {
				t.Errorf("disambiguateByCardNumber() = %d, want %d", got, tc.wantID)
			}
		})
	}
}

// --- NewDHListingService constructor tests ---

func TestNewDHListingService_NilPurchaseLookup(t *testing.T) {
	_, err := NewDHListingService(nil, observability.NewNoopLogger())
	if err == nil {
		t.Fatal("expected error for nil purchaseLookup, got nil")
	}
}

func TestNewDHListingService_NilLogger(t *testing.T) {
	_, err := NewDHListingService(&stubPurchaseLookup{}, nil)
	if err == nil {
		t.Fatal("expected error for nil logger, got nil")
	}
}

func TestNewDHListingService_Valid(t *testing.T) {
	svc, err := NewDHListingService(&stubPurchaseLookup{}, observability.NewNoopLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

// --- ListPurchases tests ---

func TestListPurchases_NilLister(t *testing.T) {
	svc, _ := NewDHListingService(&stubPurchaseLookup{}, observability.NewNoopLogger())
	// No lister wired → should return zero result.
	result := svc.ListPurchases(context.Background(), []string{"CERT-1"})
	if result.Listed != 0 || result.Synced != 0 || result.Total != 0 {
		t.Errorf("expected zero result with nil lister, got %+v", result)
	}
}

func TestListPurchases_EmptyCerts(t *testing.T) {
	svc, _ := NewDHListingService(
		&stubPurchaseLookup{},
		observability.NewNoopLogger(),
		WithDHListingLister(&stubInventoryLister{}),
	)
	result := svc.ListPurchases(context.Background(), nil)
	if result.Listed != 0 || result.Synced != 0 || result.Total != 0 {
		t.Errorf("expected zero result with empty certs, got %+v", result)
	}
}

func TestListPurchases_LookupError(t *testing.T) {
	svc, _ := NewDHListingService(
		&stubPurchaseLookup{err: errors.New("db error")},
		observability.NewNoopLogger(),
		WithDHListingLister(&stubInventoryLister{}),
	)
	result := svc.ListPurchases(context.Background(), []string{"CERT-1"})
	if result.Listed != 0 || result.Synced != 0 || result.Total != 0 {
		t.Errorf("expected zero result on lookup error, got %+v", result)
	}
}

func TestListPurchases_SuccessfulListAndSync(t *testing.T) {
	lister := &stubInventoryLister{}
	svc, _ := NewDHListingService(
		&stubPurchaseLookup{
			result: map[string]*Purchase{
				"CERT-1": {ID: "p1", CertNumber: "CERT-1", DHInventoryID: 100},
			},
		},
		observability.NewNoopLogger(),
		WithDHListingLister(lister),
	)
	result := svc.ListPurchases(context.Background(), []string{"CERT-1"})
	if result.Listed != 1 {
		t.Errorf("Listed = %d, want 1", result.Listed)
	}
	if result.Synced != 1 {
		t.Errorf("Synced = %d, want 1", result.Synced)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
	// Verify lister was called with correct status.
	if len(lister.statusCalls) != 1 || lister.statusCalls[0].status != DHStatusListed {
		t.Errorf("expected status update to %q, got %v", DHStatusListed, lister.statusCalls)
	}
	if len(lister.syncCalls) != 1 {
		t.Errorf("expected 1 sync call, got %d", len(lister.syncCalls))
	}
}

func TestListPurchases_StatusUpdateFails(t *testing.T) {
	lister := &stubInventoryLister{updateStatusErr: errors.New("api error")}
	svc, _ := NewDHListingService(
		&stubPurchaseLookup{
			result: map[string]*Purchase{
				"CERT-1": {ID: "p1", CertNumber: "CERT-1", DHInventoryID: 100},
			},
		},
		observability.NewNoopLogger(),
		WithDHListingLister(lister),
	)
	result := svc.ListPurchases(context.Background(), []string{"CERT-1"})
	if result.Listed != 0 {
		t.Errorf("Listed = %d, want 0 (status update failed)", result.Listed)
	}
	if result.Synced != 0 {
		t.Errorf("Synced = %d, want 0", result.Synced)
	}
}

func TestListPurchases_SyncFailsReverts(t *testing.T) {
	lister := &stubInventoryLister{syncErr: errors.New("sync error")}
	svc, _ := NewDHListingService(
		&stubPurchaseLookup{
			result: map[string]*Purchase{
				"CERT-1": {ID: "p1", CertNumber: "CERT-1", DHInventoryID: 100},
			},
		},
		observability.NewNoopLogger(),
		WithDHListingLister(lister),
	)
	result := svc.ListPurchases(context.Background(), []string{"CERT-1"})

	// Sync failed → listed count should be 0 (reverted).
	if result.Listed != 0 {
		t.Errorf("Listed = %d, want 0 (sync failed → reverted)", result.Listed)
	}
	if result.Synced != 0 {
		t.Errorf("Synced = %d, want 0", result.Synced)
	}

	// Verify revert call: first call is "listed", second is "in_stock".
	if len(lister.statusCalls) < 2 {
		t.Fatalf("expected at least 2 status calls, got %d", len(lister.statusCalls))
	}
	if lister.statusCalls[0].status != DHStatusListed {
		t.Errorf("first status call = %q, want %q", lister.statusCalls[0].status, DHStatusListed)
	}
	if lister.statusCalls[1].status != DHStatusInStock {
		t.Errorf("revert status call = %q, want %q", lister.statusCalls[1].status, DHStatusInStock)
	}
}

func TestListPurchases_SkipsPurchaseWithoutInventoryID(t *testing.T) {
	lister := &stubInventoryLister{}
	svc, _ := NewDHListingService(
		&stubPurchaseLookup{
			result: map[string]*Purchase{
				"CERT-1": {ID: "p1", CertNumber: "CERT-1", DHInventoryID: 0}, // not yet pushed
			},
		},
		observability.NewNoopLogger(),
		WithDHListingLister(lister),
	)
	result := svc.ListPurchases(context.Background(), []string{"CERT-1"})
	if result.Listed != 0 {
		t.Errorf("Listed = %d, want 0 (no inventory ID)", result.Listed)
	}
	if result.Total != 1 {
		t.Errorf("Total = %d, want 1", result.Total)
	}
}

func TestListPurchases_InlineMatchAndPush(t *testing.T) {
	lister := &stubInventoryLister{}
	resolver := &stubCertResolver{
		resp: &DHCertResolution{Status: DHCertStatusMatched, DHCardID: 42},
	}
	pusher := &stubInventoryPusher{
		resp: &DHInventoryPushResult{
			Results: []DHInventoryPushResultItem{
				{DHInventoryID: 200, Status: "in_stock", AssignedPriceCents: 5000},
			},
		},
	}

	svc, _ := NewDHListingService(
		&stubPurchaseLookup{
			result: map[string]*Purchase{
				"CERT-1": {
					ID: "p1", CertNumber: "CERT-1",
					DHInventoryID: 0, DHPushStatus: DHPushStatusPending,
					ReviewedPriceCents: 5000, // needed for ResolveMarketValueCents
				},
			},
		},
		observability.NewNoopLogger(),
		WithDHListingLister(lister),
		WithDHListingCertResolver(resolver),
		WithDHListingPusher(pusher),
	)
	result := svc.ListPurchases(context.Background(), []string{"CERT-1"})
	if result.Listed != 1 {
		t.Errorf("Listed = %d, want 1 (inline push succeeded)", result.Listed)
	}
	if result.Synced != 1 {
		t.Errorf("Synced = %d, want 1", result.Synced)
	}
}

// --- disambiguateCandidates tests ---

func TestDisambiguateCandidates(t *testing.T) {
	tests := []struct {
		name           string
		candidates     []DHCertCandidate
		cardNumber     string
		saveFn         func(string) error
		wantID         int
		wantErr        bool
		wantSaveCalled bool
	}{
		{
			name: "match without save",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "42"},
				{DHCardID: 20, CardNumber: "99"},
			},
			cardNumber: "42",
			wantID:     10,
		},
		{
			name: "no match saves JSON",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "1"},
				{DHCardID: 20, CardNumber: "2"},
			},
			cardNumber:     "99",
			wantID:         0,
			wantSaveCalled: true,
		},
		{
			name: "save function error",
			candidates: []DHCertCandidate{
				{DHCardID: 10, CardNumber: "1"},
			},
			cardNumber: "99",
			saveFn:     func(_ string) error { return errors.New("save failed") },
			wantID:     0,
			wantErr:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var saveCalled bool
			saveFn := tc.saveFn
			if saveFn == nil && tc.wantSaveCalled {
				saveFn = func(j string) error {
					saveCalled = true
					if j == "" {
						t.Error("expected non-empty candidates JSON")
					}
					return nil
				}
			}
			id, err := disambiguateCandidates(tc.candidates, tc.cardNumber, saveFn)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tc.wantID {
				t.Errorf("got %d, want %d", id, tc.wantID)
			}
			if tc.wantSaveCalled && !saveCalled {
				t.Error("expected save function to be called")
			}
		})
	}
}

// --- ComputeCapitalSummary tests ---

func TestComputeCapitalSummary(t *testing.T) {
	tests := []struct {
		name       string
		input      *CapitalRawData
		wantWeeks  float64
		wantTrend  RecoveryTrend
		wantAlert  AlertLevel
		checkExact bool // if true, check WeeksToCover == wantWeeks; otherwise just check != sentinel
		checkTrend bool
		checkAlert bool
	}{
		{
			name:       "nil input returns safe defaults",
			input:      nil,
			wantWeeks:  WeeksToCoverNoData,
			wantTrend:  TrendStable,
			wantAlert:  AlertOK,
			checkExact: true,
			checkTrend: true,
			checkAlert: true,
		},
		{
			name:       "zero recovery rate",
			input:      &CapitalRawData{OutstandingCents: 600000},
			wantWeeks:  WeeksToCoverNoData,
			wantTrend:  TrendStable,
			wantAlert:  AlertWarning,
			checkExact: true,
			checkTrend: true,
			checkAlert: true,
		},
		{
			name: "with recovery - easily covered",
			input: &CapitalRawData{
				OutstandingCents:          10000,
				RecoveryRate30dCents:      43000,
				RecoveryRate30dPriorCents: 43000,
			},
			wantTrend:  TrendStable,
			wantAlert:  AlertOK,
			checkExact: false,
			checkTrend: true,
			checkAlert: true,
		},
		{
			name: "improving trend",
			input: &CapitalRawData{
				OutstandingCents:          100000,
				RecoveryRate30dCents:      60000,
				RecoveryRate30dPriorCents: 40000,
			},
			wantTrend:  TrendImproving,
			checkTrend: true,
		},
		{
			name: "declining trend",
			input: &CapitalRawData{
				OutstandingCents:          100000,
				RecoveryRate30dCents:      30000,
				RecoveryRate30dPriorCents: 60000,
			},
			wantTrend:  TrendDeclining,
			checkTrend: true,
		},
		{
			name: "recovery collapsed to zero from positive prior",
			input: &CapitalRawData{
				OutstandingCents:          100000,
				RecoveryRate30dCents:      0,
				RecoveryRate30dPriorCents: 50000,
			},
			wantWeeks:  WeeksToCoverNoData,
			wantTrend:  TrendDeclining,
			checkExact: true,
			checkTrend: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ComputeCapitalSummary(tc.input)
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			if tc.checkExact {
				if result.WeeksToCover != tc.wantWeeks {
					t.Errorf("WeeksToCover = %f, want %f", result.WeeksToCover, tc.wantWeeks)
				}
			} else if !tc.checkExact && tc.wantWeeks == 0 {
				// For cases where we just verify it's computed (not sentinel)
				if result.WeeksToCover == WeeksToCoverNoData {
					t.Error("expected computed WeeksToCover, got sentinel")
				}
			}
			if tc.checkTrend && result.RecoveryTrend != tc.wantTrend {
				t.Errorf("RecoveryTrend = %q, want %q", result.RecoveryTrend, tc.wantTrend)
			}
			if tc.checkAlert && result.AlertLevel != tc.wantAlert {
				t.Errorf("AlertLevel = %q, want %q", result.AlertLevel, tc.wantAlert)
			}
		})
	}
}
