package inventory_test

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// stubPSAResolver is a minimal PSACampaignResolver for import tests.
type stubPSAResolver struct {
	campaignID string
	ok         bool
	err        error
}

func (r stubPSAResolver) ResolveCampaignID(_ context.Context, psaName string) (string, bool, error) {
	if psaName == "" {
		return "", false, nil
	}
	return r.campaignID, r.ok, r.err
}

// newPSAImportFixture builds a service wired with an in-memory campaign store,
// a stub PSA resolver, and both a PSA-resolvable campaign ("camp-psa") and an
// inference-only fallback campaign ("camp-inferred").
func newPSAImportFixture(t *testing.T, resolveTo string, resolveOK bool) (inventory.Service, *mocks.InMemoryCampaignStore) {
	t.Helper()
	repo := mocks.NewInMemoryCampaignStore()
	resolver := stubPSAResolver{campaignID: resolveTo, ok: resolveOK}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), withDisabledBackgroundWorkers(),
		inventory.WithPSACampaignResolver(resolver))
	ctx := context.Background()

	// The PSA-resolved campaign. Its inclusion list intentionally cannot match
	// the test row's card name, so the fallback scenarios are unambiguous.
	psaCampaign := &inventory.Campaign{
		ID: "camp-psa", Name: "PSA-resolved campaign", Sport: "Pokemon",
		Phase: inventory.PhaseActive, GradeRange: "8-10", InclusionList: "ZZZNoMatch",
	}
	if err := svc.CreateCampaign(ctx, psaCampaign); err != nil {
		t.Fatalf("CreateCampaign(psa): %v", err)
	}

	// The inference-only fallback campaign — the only campaign FindMatchingCampaign
	// can match against the test row's Umbreon listing title.
	inferredCampaign := &inventory.Campaign{
		ID: "camp-inferred", Name: "Inferred campaign", Sport: "Pokemon",
		Phase: inventory.PhaseActive, GradeRange: "8-10", InclusionList: "Umbreon",
	}
	if err := svc.CreateCampaign(ctx, inferredCampaign); err != nil {
		t.Fatalf("CreateCampaign(inferred): %v", err)
	}

	return svc, repo
}

// runSingleRowImport imports a single PSA row with the given raw PSA campaign
// name and returns the resulting Purchase. The resolver resolves "Modern" to
// "camp-psa" and rejects everything else, mirroring a PSA name that no longer
// maps to any known campaign.
func runSingleRowImport(t *testing.T, psaName string) *inventory.Purchase {
	t.Helper()
	svc, repo := newPSAImportFixture(t, "camp-psa", psaName == "Modern")
	ctx := context.Background()

	row := inventory.PSAExportRow{
		CertNumber: "163772677", ListingTitle: "UMBREON PSA 9", Grade: 9,
		PricePaid: 789.96, Date: "2026-08-01", Category: "Pokemon",
		PSACampaignName: psaName,
	}
	result, err := svc.ImportPSAExportGlobal(ctx, []inventory.PSAExportRow{row})
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}
	if result.Allocated != 1 {
		t.Fatalf("Allocated = %d, want 1 (results: %+v)", result.Allocated, result.Results)
	}

	purchase, err := repo.GetPurchaseByCertNumber(ctx, "PSA", row.CertNumber)
	if err != nil {
		t.Fatalf("GetPurchaseByCertNumber: %v", err)
	}
	if purchase == nil {
		t.Fatal("allocated purchase not found")
	}
	return purchase
}

func TestImportPSA_PrefersPSAAttribution(t *testing.T) {
	tests := []struct {
		name           string
		psaName        string
		wantCampaignID string
		wantSource     string
	}{
		{"psa resolves", "Modern", "camp-psa", inventory.AttributionSourcePSA},
		{"psa name dead", "Brady modern", "camp-inferred", inventory.AttributionSourceInferred},
		{"no psa name", "", "camp-inferred", inventory.AttributionSourceInferred},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runSingleRowImport(t, tt.psaName)
			if got.CampaignID != tt.wantCampaignID {
				t.Errorf("CampaignID = %q, want %q", got.CampaignID, tt.wantCampaignID)
			}
			if got.AttributionSource != tt.wantSource {
				t.Errorf("AttributionSource = %q, want %q", got.AttributionSource, tt.wantSource)
			}
			if got.PSACampaignName != tt.psaName {
				t.Errorf("PSACampaignName = %q, want %q", got.PSACampaignName, tt.psaName)
			}
		})
	}
}

// TestImportPSA_ResolvedRowIsAllocatedNotPending guards against regressing the
// synthesized MatchResult in ImportPSAExportGlobal: if a PSA-resolved campaign
// is ever left as a zero-value MatchResult, handleNewPSAPurchase's switch on
// match.Status falls through to "unmatched" and the row becomes pending —
// the exact inverse of this feature's purpose.
func TestImportPSA_ResolvedRowIsAllocatedNotPending(t *testing.T) {
	repo := mocks.NewInMemoryCampaignStore()
	psaCampaign := &inventory.Campaign{
		ID: "camp-psa", Name: "PSA-resolved campaign", Sport: "Pokemon",
		Phase: inventory.PhaseActive, GradeRange: "8-10", InclusionList: "ZZZNoMatch",
	}
	ctx := context.Background()
	if err := repo.CreateCampaign(ctx, psaCampaign); err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}

	var savedPending []inventory.PendingItem
	pendingRepo := &mocks.MockPendingItemRepository{
		SavePendingItemsFn: func(_ context.Context, items []inventory.PendingItem) error {
			savedPending = append(savedPending, items...)
			return nil
		},
	}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), withDisabledBackgroundWorkers(),
		inventory.WithPSACampaignResolver(stubPSAResolver{campaignID: "camp-psa", ok: true}),
		inventory.WithPendingItemRepository(pendingRepo))

	res, err := svc.ImportPSAExportGlobal(ctx, []inventory.PSAExportRow{
		{CertNumber: "123", PSACampaignName: "Modern", ListingTitle: "UMBREON PSA 9", Grade: 9, PricePaid: 789.96, Date: "2026-08-01", Category: "Pokemon"},
	})
	if err != nil {
		t.Fatalf("ImportPSAExportGlobal: %v", err)
	}
	if res.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1", res.Allocated)
	}
	if got := res.Results[0].Status; got != "allocated" {
		t.Errorf("Status = %q, want allocated — a zero-value MatchResult falls through to unmatched", got)
	}
	if len(savedPending) != 0 {
		t.Errorf("pending items = %d, want 0", len(savedPending))
	}
}
