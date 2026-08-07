package inventory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// reconcileFixtureRepo layers an in-memory PendingItemRepository on top of the
// shared InMemoryCampaignStore so reconciliation tests can both drive
// campaign/purchase state and directly inspect the pending-item work queue.
type reconcileFixtureRepo struct {
	*mocks.InMemoryCampaignStore
	PendingItems []inventory.PendingItem
}

func (r *reconcileFixtureRepo) SavePendingItems(_ context.Context, items []inventory.PendingItem) error {
	r.PendingItems = append(r.PendingItems, items...)
	return nil
}

func (r *reconcileFixtureRepo) ListPendingItems(_ context.Context) ([]inventory.PendingItem, error) {
	return r.PendingItems, nil
}

func (r *reconcileFixtureRepo) GetPendingItemByID(_ context.Context, id string) (*inventory.PendingItem, error) {
	for i := range r.PendingItems {
		if r.PendingItems[i].ID == id {
			return &r.PendingItems[i], nil
		}
	}
	return nil, inventory.ErrPendingItemNotFound
}

func (r *reconcileFixtureRepo) ResolvePendingItem(_ context.Context, id string, _ string) error {
	for i := range r.PendingItems {
		if r.PendingItems[i].ID == id {
			r.PendingItems = append(r.PendingItems[:i], r.PendingItems[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *reconcileFixtureRepo) DismissPendingItem(ctx context.Context, id string) error {
	return r.ResolvePendingItem(ctx, id, "")
}

func (r *reconcileFixtureRepo) CountPendingItems(_ context.Context) (int, error) {
	return len(r.PendingItems), nil
}

// newReconcileFixture builds a service with two campaigns ("camp-a", "camp-b")
// and a single PSA purchase ("purchase-1", cert "123") currently attributed to
// currentCID. resolveTo/resolveOK configure the stub PSA resolver's response
// for any non-empty PSA campaign name; resolveErr, when non-nil, makes every
// lookup fail outright regardless of resolveTo/resolveOK. initialSource sets
// the purchase's starting AttributionSource, so tests that assert a specific
// resulting source can start from something other than that value and prove
// the write actually happened.
func newReconcileFixture(t *testing.T, currentCID string, sold bool, resolveTo string, resolveOK bool, resolveErr error, initialSource string) (inventory.ImportService, *reconcileFixtureRepo) {
	t.Helper()
	repo := &reconcileFixtureRepo{InMemoryCampaignStore: mocks.NewInMemoryCampaignStore()}
	resolver := stubPSAResolver{campaignID: resolveTo, ok: resolveOK, err: resolveErr}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), withDisabledBackgroundWorkers(),
		inventory.WithPSACampaignResolver(resolver),
		inventory.WithPendingItemRepository(repo))
	ctx := context.Background()

	for _, id := range []string{"camp-a", "camp-b"} {
		c := &inventory.Campaign{
			ID: id, Name: id, Sport: "Pokemon",
			Phase: inventory.PhaseActive, GradeRange: "8-10",
		}
		if err := svc.CreateCampaign(ctx, c); err != nil {
			t.Fatalf("CreateCampaign(%s): %v", id, err)
		}
	}

	repo.Purchases["purchase-1"] = &inventory.Purchase{
		ID: "purchase-1", CampaignID: currentCID, Grader: "PSA", CertNumber: "123",
		PurchaseDate: "2026-08-01", GradeValue: 9, BuyCostCents: 1000,
		AttributionSource: initialSource,
	}
	if sold {
		repo.PurchaseSales["purchase-1"] = true
	}
	return svc, repo
}

// newReconcileFixtureWithDates builds a fixture for the CL-confidence freezing
// tests: one campaign ("camp-a", resolved to by the stub PSA resolver) with the
// given UpdatedAt, and a purchase dated purchaseDate currently on "camp-b" so
// reconciliation always takes the move path.
func newReconcileFixtureWithDates(t *testing.T, purchaseDate string, campaignUpdatedAt time.Time) (inventory.ImportService, *reconcileFixtureRepo) {
	t.Helper()
	repo := &reconcileFixtureRepo{InMemoryCampaignStore: mocks.NewInMemoryCampaignStore()}
	resolver := stubPSAResolver{campaignID: "camp-a", ok: true}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), withDisabledBackgroundWorkers(),
		inventory.WithPSACampaignResolver(resolver))
	ctx := context.Background()

	campA := &inventory.Campaign{
		ID: "camp-a", Name: "camp-a", Sport: "Pokemon",
		Phase: inventory.PhaseActive, GradeRange: "8-10", CLConfidence: "2.5-4",
	}
	if err := svc.CreateCampaign(ctx, campA); err != nil {
		t.Fatalf("CreateCampaign(camp-a): %v", err)
	}
	campB := &inventory.Campaign{
		ID: "camp-b", Name: "camp-b", Sport: "Pokemon",
		Phase: inventory.PhaseActive, GradeRange: "8-10",
	}
	if err := svc.CreateCampaign(ctx, campB); err != nil {
		t.Fatalf("CreateCampaign(camp-b): %v", err)
	}
	// CreateCampaign stamps UpdatedAt; overwrite directly to the fixture's value.
	repo.Campaigns["camp-a"].UpdatedAt = campaignUpdatedAt

	repo.Purchases["purchase-1"] = &inventory.Purchase{
		ID: "purchase-1", CampaignID: "camp-b", Grader: "PSA", CertNumber: "123",
		PurchaseDate: purchaseDate, GradeValue: 9, BuyCostCents: 1000,
		AttributionSource: inventory.AttributionSourceInferred,
	}
	return svc, repo
}

func mustTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestReconcilePSAAttribution(t *testing.T) {
	tests := []struct {
		name          string
		psaName       string
		resolveTo     string
		resolveOK     bool
		currentCID    string
		sold          bool
		initialSource string
		want          inventory.ReconcileResult
		wantSource    string
	}{
		{"agreement", "Modern", "camp-a", true, "camp-a", false, inventory.AttributionSourceInferred,
			inventory.ReconcileResult{Agreed: 1}, inventory.AttributionSourcePSA},
		{"disagreement moves", "Modern", "camp-a", true, "camp-b", false, inventory.AttributionSourceInferred,
			inventory.ReconcileResult{Moved: 1}, inventory.AttributionSourcePSA},
		// initialSource starts at PSA (not the default Inferred) so the
		// assertion below proves UpdatePurchaseAttributionName actually ran
		// and left the source unchanged, rather than merely matching the
		// fixture's baseline.
		{"sold purchase skipped", "Modern", "camp-a", true, "camp-b", true, inventory.AttributionSourcePSA,
			inventory.ReconcileResult{SoldSkipped: 1}, inventory.AttributionSourcePSA},
		// Same reasoning: starts at PSA so the resulting Inferred proves the
		// downgrade wrote, rather than the fixture default coinciding with it.
		{"dead name unresolved", "Brady modern", "", false, "camp-b", false, inventory.AttributionSourcePSA,
			inventory.ReconcileResult{Unresolved: 1}, inventory.AttributionSourceInferred},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newReconcileFixture(t, tt.currentCID, tt.sold, tt.resolveTo, tt.resolveOK, nil, tt.initialSource)
			got, err := svc.ReconcilePSAAttribution(context.Background(),
				[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: tt.psaName}})
			if err != nil {
				t.Fatalf("ReconcilePSAAttribution: %v", err)
			}
			if got != tt.want {
				t.Errorf("result = %+v, want %+v", got, tt.want)
			}
			if gotSource := repo.Purchases["purchase-1"].AttributionSource; gotSource != tt.wantSource {
				t.Errorf("AttributionSource = %q, want %q", gotSource, tt.wantSource)
			}
			if tt.name == "sold purchase skipped" {
				if got := repo.Purchases["purchase-1"].PSACampaignName; got != tt.psaName {
					t.Errorf("PSACampaignName = %q, want %q", got, tt.psaName)
				}
			}
		})
	}
}

func TestReconcilePSAAttribution_SoldPurchaseRecordsNameWithoutMoving(t *testing.T) {
	// Starts at PSA (not the fixture's default Inferred) so the "unchanged"
	// assertion below proves the sold-skip branch preserves whatever source
	// was already there, rather than coincidentally matching the default.
	svc, repo := newReconcileFixture(t, "camp-b", true, "camp-a", true, nil, inventory.AttributionSourcePSA)
	if _, err := svc.ReconcilePSAAttribution(context.Background(),
		[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Modern"}}); err != nil {
		t.Fatalf("ReconcilePSAAttribution: %v", err)
	}
	got := repo.Purchases["purchase-1"]
	if got.CampaignID != "camp-b" {
		t.Errorf("CampaignID = %q, want unchanged camp-b", got.CampaignID)
	}
	if got.PSACampaignName != "Modern" {
		t.Errorf("PSACampaignName = %q, want Modern", got.PSACampaignName)
	}
	if got.AttributionSource != inventory.AttributionSourcePSA {
		t.Errorf("AttributionSource = %q, want unchanged psa", got.AttributionSource)
	}
}

func TestReconcilePSAAttribution_UnresolvedEnqueuesPendingItem(t *testing.T) {
	// Starts at PSA so the resulting Inferred proves recordUnresolvedAttribution
	// actually wrote the downgrade, rather than matching the fixture default.
	svc, repo := newReconcileFixture(t, "camp-b", false, "", false, nil, inventory.AttributionSourcePSA)
	if _, err := svc.ReconcilePSAAttribution(context.Background(),
		[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Brady modern"}}); err != nil {
		t.Fatalf("ReconcilePSAAttribution: %v", err)
	}
	if len(repo.PendingItems) != 1 {
		t.Fatalf("pending items = %d, want 1", len(repo.PendingItems))
	}
	if got := repo.Purchases["purchase-1"].AttributionSource; got != inventory.AttributionSourceInferred {
		t.Errorf("AttributionSource = %q, want inferred", got)
	}
}

// TestReconcilePSAAttribution_UnresolvedPreservesManualAttribution covers the
// human ruling: an unresolvable PSA name is not a usable answer from PSA, so a
// prior manual override must survive it rather than being downgraded to
// inferred. This supersedes the brief's hardcoded AttributionSourceInferred
// for this one case, by explicit human ruling (see task-7 fix round 1 report).
func TestReconcilePSAAttribution_UnresolvedPreservesManualAttribution(t *testing.T) {
	svc, repo := newReconcileFixture(t, "camp-b", false, "", false, nil, inventory.AttributionSourceManual)
	if _, err := svc.ReconcilePSAAttribution(context.Background(),
		[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Brady modern"}}); err != nil {
		t.Fatalf("ReconcilePSAAttribution: %v", err)
	}
	if got := repo.Purchases["purchase-1"].AttributionSource; got != inventory.AttributionSourceManual {
		t.Errorf("AttributionSource = %q, want manual preserved", got)
	}
	if repo.Purchases["purchase-1"].CampaignID != "camp-b" {
		t.Errorf("CampaignID = %q, want unchanged camp-b", repo.Purchases["purchase-1"].CampaignID)
	}
}

func TestReconcilePSAAttribution_ResolvingClearsStalePendingItem(t *testing.T) {
	svc, repo := newReconcileFixture(t, "camp-b", false, "camp-a", true, nil, inventory.AttributionSourceInferred)
	repo.PendingItems = []inventory.PendingItem{{CertNumber: "123"}}
	if _, err := svc.ReconcilePSAAttribution(context.Background(),
		[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Modern"}}); err != nil {
		t.Fatalf("ReconcilePSAAttribution: %v", err)
	}
	if len(repo.PendingItems) != 0 {
		t.Errorf("pending items = %d, want 0 (resolved)", len(repo.PendingItems))
	}
}

// TestReconcilePSAAttribution_ResolverErrorHardStops covers the untested hard
// stop: a resolver lookup that fails outright must abort the whole pass with
// a zero-valued result, not be treated as "unresolved" for this row and
// continued for the rest (which would silently mass-downgrade every
// remaining row).
func TestReconcilePSAAttribution_ResolverErrorHardStops(t *testing.T) {
	resolveErr := errors.New("stale snapshot")
	svc, repo := newReconcileFixture(t, "camp-b", false, "camp-a", true, resolveErr, inventory.AttributionSourceInferred)
	got, err := svc.ReconcilePSAAttribution(context.Background(),
		[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Modern"}})
	if !errors.Is(err, resolveErr) {
		t.Fatalf("err = %v, want %v", err, resolveErr)
	}
	if got != (inventory.ReconcileResult{}) {
		t.Errorf("result = %+v, want zero value", got)
	}
	if repo.Purchases["purchase-1"].CampaignID != "camp-b" {
		t.Errorf("CampaignID = %q, want unchanged camp-b", repo.Purchases["purchase-1"].CampaignID)
	}
}

func TestReconcilePSAAttribution_CLConfidenceFreezing(t *testing.T) {
	tests := []struct {
		name              string
		purchaseDate      string
		campaignUpdatedAt time.Time
		wantNil           bool
	}{
		{"campaign untouched since purchase", "2026-08-01", mustTime("2026-07-01"), false},
		{"campaign written after purchase", "2026-07-01", mustTime("2026-08-01"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newReconcileFixtureWithDates(t, tt.purchaseDate, tt.campaignUpdatedAt)
			if _, err := svc.ReconcilePSAAttribution(context.Background(),
				[]inventory.PSAExportRow{{CertNumber: "123", PSACampaignName: "Modern"}}); err != nil {
				t.Fatalf("ReconcilePSAAttribution: %v", err)
			}
			got := repo.Purchases["purchase-1"].CLConfidenceAtPurchase
			if tt.wantNil && got != nil {
				t.Errorf("CLConfidenceAtPurchase = %d, want nil", *got)
			}
			if !tt.wantNil && got == nil {
				t.Error("CLConfidenceAtPurchase = nil, want re-derived value")
			}
		})
	}
}
