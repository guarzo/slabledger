package inventory_test

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// newConditionalTestService returns a service backed by one in-memory store,
// plus the store so a test can move the row behind the caller's back.
func newConditionalTestService(t *testing.T) (inventory.Service, *mocks.InMemoryCampaignStore) {
	t.Helper()
	repo := mocks.NewInMemoryCampaignStore()
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), withDisabledBackgroundWorkers())
	return svc, repo
}

func seedCampaign(t *testing.T, repo *mocks.InMemoryCampaignStore, updatedAt time.Time) *inventory.Campaign {
	t.Helper()
	c := &inventory.Campaign{
		ID:        "camp-1",
		Name:      "Vintage Core",
		Sport:     "Pokemon",
		Phase:     inventory.PhaseActive,
		CreatedAt: updatedAt,
		UpdatedAt: updatedAt,
	}
	if err := repo.CreateCampaign(context.Background(), c); err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	return c
}

func TestService_UpdateCampaignIfUnchanged(t *testing.T) {
	base := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		// mutate runs after seeding, standing in for a concurrent writer.
		mutate    func(repo *mocks.InMemoryCampaignStore)
		expected  time.Time
		wantErrIs func(error) bool
		wantWrite bool
	}{
		{
			name:      "writes when the row is untouched",
			expected:  base,
			wantWrite: true,
		},
		{
			name: "rejects when another writer moved the row",
			mutate: func(repo *mocks.InMemoryCampaignStore) {
				repo.Campaigns["camp-1"].UpdatedAt = base.Add(time.Minute)
				repo.Campaigns["camp-1"].Name = "Renamed by the harvester"
			},
			expected:  base,
			wantErrIs: inventory.IsCampaignConflict,
		},
		{
			name: "reports a missing row as not-found, not as a conflict",
			mutate: func(repo *mocks.InMemoryCampaignStore) {
				delete(repo.Campaigns, "camp-1")
			},
			expected:  base,
			wantErrIs: inventory.IsCampaignNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newConditionalTestService(t)
			seedCampaign(t, repo, base)
			if tt.mutate != nil {
				tt.mutate(repo)
			}

			update := &inventory.Campaign{
				ID: "camp-1", Name: "Pasted name", Sport: "Pokemon",
				Phase: inventory.PhaseActive, CreatedAt: base,
			}
			err := svc.UpdateCampaignIfUnchanged(context.Background(), update, tt.expected)

			if tt.wantErrIs != nil {
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
				if !tt.wantErrIs(err) {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateCampaignIfUnchanged: %v", err)
			}
			if got := repo.Campaigns["camp-1"].Name; got != "Pasted name" {
				t.Errorf("row not written: name = %q", got)
			}
		})
	}
}

func TestService_UpdateCampaignIfUnchanged_StampsANewUpdatedAt(t *testing.T) {
	// The precondition and the new stamp are different values. If the service
	// reused the incoming UpdatedAt as the precondition — or wrote the expected
	// time back into the row — the next conditional write from the same client
	// would either always pass or always fail.
	base := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	svc, repo := newConditionalTestService(t)
	seedCampaign(t, repo, base)

	update := &inventory.Campaign{
		ID: "camp-1", Name: "Pasted name", Sport: "Pokemon",
		Phase: inventory.PhaseActive, CreatedAt: base, UpdatedAt: base,
	}
	if err := svc.UpdateCampaignIfUnchanged(context.Background(), update, base); err != nil {
		t.Fatalf("UpdateCampaignIfUnchanged: %v", err)
	}

	if !update.UpdatedAt.After(base) {
		t.Errorf("expected a fresh UpdatedAt stamp, got %v", update.UpdatedAt)
	}

	// A second write reusing the original timestamp must now be rejected.
	err := svc.UpdateCampaignIfUnchanged(context.Background(), &inventory.Campaign{
		ID: "camp-1", Name: "Second write", Sport: "Pokemon",
		Phase: inventory.PhaseActive, CreatedAt: base,
	}, base)
	if !inventory.IsCampaignConflict(err) {
		t.Fatalf("expected a conflict on the stale precondition, got %v", err)
	}
}

func TestService_UpdateCampaignIfUnchanged_ValidatesBeforeWriting(t *testing.T) {
	// Validation must short-circuit ahead of the repository, exactly as the
	// unconditional path does; otherwise an invalid payload consumes the
	// precondition and the caller cannot tell why it failed.
	base := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	svc, repo := newConditionalTestService(t)
	seedCampaign(t, repo, base)

	err := svc.UpdateCampaignIfUnchanged(context.Background(), &inventory.Campaign{
		ID: "camp-1", Name: "", Sport: "Pokemon", Phase: inventory.PhaseActive,
	}, base)
	if err == nil {
		t.Fatal("expected a validation error for an empty name")
	}
	if inventory.IsCampaignConflict(err) || inventory.IsCampaignNotFound(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
	if got := repo.Campaigns["camp-1"].Name; got != "Vintage Core" {
		t.Errorf("row was written despite failing validation: name = %q", got)
	}
}
