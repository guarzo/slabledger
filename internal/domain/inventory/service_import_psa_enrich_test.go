package inventory

import (
	"context"
	"errors"
	"testing"
)

// stubCardIDResolver is a test-local mock for the inventory.CardIDResolver
// interface. Lives in this file only — do not promote to testutil/mocks.
type stubCardIDResolver struct {
	fn func(ctx context.Context, certs []string, grader string) (map[string]string, error)
}

func (s *stubCardIDResolver) ResolveCardIDsByCerts(ctx context.Context, certs []string, grader string) (map[string]string, error) {
	return s.fn(ctx, certs, grader)
}

func TestBatchResolveCardIDs(t *testing.T) {
	errResolver := errors.New("boom")

	tests := []struct {
		name             string
		hasResolver      bool
		resolver         func(ctx context.Context, certs []string, grader string) (map[string]string, error)
		seedPurchases    map[string]*Purchase
		certs            []string
		wantPersistedIDs map[string]int // purchaseID -> expected DHCardID after call
		wantUnchangedIDs []string       // purchaseIDs that must NOT have DHCardID set
	}{
		{
			name:        "happy path — two certs resolved and persisted",
			hasResolver: true,
			resolver: func(_ context.Context, _ []string, _ string) (map[string]string, error) {
				return map[string]string{
					"C1": "100",
					"C2": "200",
				}, nil
			},
			seedPurchases: map[string]*Purchase{
				"p1": {ID: "p1", CertNumber: "C1"},
				"p2": {ID: "p2", CertNumber: "C2"},
			},
			certs: []string{"C1", "C2"},
			wantPersistedIDs: map[string]int{
				"p1": 100,
				"p2": 200,
			},
		},
		{
			name:        "partial match — resolver returns two, only one purchase exists",
			hasResolver: true,
			resolver: func(_ context.Context, _ []string, _ string) (map[string]string, error) {
				return map[string]string{
					"C1": "100",
					"C2": "200",
				}, nil
			},
			seedPurchases: map[string]*Purchase{
				"p1": {ID: "p1", CertNumber: "C1"},
			},
			certs: []string{"C1", "C2"},
			wantPersistedIDs: map[string]int{
				"p1": 100,
			},
		},
		{
			name:        "non-numeric card id — skipped, no update",
			hasResolver: true,
			resolver: func(_ context.Context, _ []string, _ string) (map[string]string, error) {
				return map[string]string{"C1": "not-a-number"}, nil
			},
			seedPurchases: map[string]*Purchase{
				"p1": {ID: "p1", CertNumber: "C1"},
			},
			certs:            []string{"C1"},
			wantPersistedIDs: map[string]int{},
			wantUnchangedIDs: []string{"p1"},
		},
		{
			name:        "resolver error — no updates",
			hasResolver: true,
			resolver: func(_ context.Context, _ []string, _ string) (map[string]string, error) {
				return nil, errResolver
			},
			seedPurchases: map[string]*Purchase{
				"p1": {ID: "p1", CertNumber: "C1"},
			},
			certs:            []string{"C1"},
			wantPersistedIDs: map[string]int{},
			wantUnchangedIDs: []string{"p1"},
		},
		{
			name:             "nil resolver — no panic, no updates",
			hasResolver:      false,
			seedPurchases:    map[string]*Purchase{"p1": {ID: "p1", CertNumber: "C1"}},
			certs:            []string{"C1"},
			wantPersistedIDs: map[string]int{},
			wantUnchangedIDs: []string{"p1"},
		},
		{
			name:        "empty certs — resolver not invoked, no updates",
			hasResolver: true,
			resolver: func(_ context.Context, _ []string, _ string) (map[string]string, error) {
				t.Error("resolver should not be invoked on empty certs")
				return nil, nil
			},
			seedPurchases:    map[string]*Purchase{"p1": {ID: "p1", CertNumber: "C1"}},
			certs:            nil,
			wantPersistedIDs: map[string]int{},
			wantUnchangedIDs: []string{"p1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			for id, p := range tc.seedPurchases {
				cp := *p
				repo.purchases[id] = &cp
			}

			svc := &service{purchases: repo}
			if tc.hasResolver {
				svc.cardIDResolver = &stubCardIDResolver{fn: tc.resolver}
			}

			svc.batchResolveCardIDs(context.Background(), tc.certs)

			for id, want := range tc.wantPersistedIDs {
				got, ok := repo.purchases[id]
				if !ok {
					t.Fatalf("purchase %q missing from repo", id)
				}
				if got.DHCardID != want {
					t.Errorf("purchase %q DHCardID: got %d, want %d", id, got.DHCardID, want)
				}
			}
			for _, id := range tc.wantUnchangedIDs {
				got, ok := repo.purchases[id]
				if !ok {
					t.Fatalf("purchase %q missing from repo", id)
				}
				if got.DHCardID != 0 {
					t.Errorf("purchase %q DHCardID: got %d, want 0 (unchanged)", id, got.DHCardID)
				}
			}
		})
	}
}
