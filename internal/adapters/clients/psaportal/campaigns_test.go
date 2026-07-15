package psaportal

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFetchCampaigns_ParsesListAndEdit(t *testing.T) {
	list, err := os.ReadFile("../../../../docs/psa-campaigns-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	edit, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	ff := &fakeFetcher{routes: map[string]string{
		campaignsListPath: string(list),
		"/edit/__data.json?x-sveltekit-invalidated=0001": string(edit),
	}}

	c := New(ff, Config{})
	got, err := c.FetchCampaigns(context.Background())
	if err != nil {
		t.Fatalf("FetchCampaigns: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected >=1 campaign")
	}
	if got[0].CampaignRequestID == "" || got[0].Name == "" {
		t.Errorf("campaign missing id/name: %+v", got[0])
	}

	// Find the "Crystal" campaign and assert its price fields are exact cents
	// (fixture: priceMin=$500, priceMax=$3000 USD -> 50000/300000 cents).
	var crystal *struct {
		found bool
		min   int
		max   int
	}
	for _, pc := range got {
		if pc.Name == "Crystal" {
			crystal = &struct {
				found bool
				min   int
				max   int
			}{found: true, min: pc.BuyBox.PriceMinCents, max: pc.BuyBox.PriceMaxCents}
			break
		}
	}
	if crystal == nil || !crystal.found {
		t.Fatalf("Crystal campaign not found in %+v", got)
	}
	if crystal.min != 50000 {
		t.Errorf("Crystal PriceMinCents = %d, want 50000", crystal.min)
	}
	if crystal.max != 300000 {
		t.Errorf("Crystal PriceMaxCents = %d, want 300000", crystal.max)
	}
}

// buildListEnvelope packs a minimal campaignsResponse into a SvelteKit
// __data.json envelope for use in synthetic pagination tests.
func buildListEnvelope(t *testing.T, items []any, pageSize, totalCount int) []byte {
	t.Helper()
	root := map[string]any{
		"campaignsResponse": map[string]any{
			"items":      items,
			"pageSize":   float64(pageSize),
			"totalCount": float64(totalCount),
		},
	}
	packed, err := EncodeRefPacked(root)
	if err != nil {
		t.Fatalf("EncodeRefPacked: %v", err)
	}
	env := map[string]any{
		"type": "data",
		"nodes": []any{
			map[string]any{"type": "data", "data": packed},
		},
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return b
}

func campaignItem(id, name string) map[string]any {
	return map[string]any{
		"campaignRequestId": id,
		"campaignName":      name,
		"campaignType":      "CATEGORY",
		"status":            "ACTIVE",
		"category":          "POKEMON",
		"buyBox": map[string]any{
			"gradeMin": "1", "gradeMax": "10",
			"yearMin": float64(2000), "yearMax": float64(2020),
			"priceMin": float64(1), "priceMax": float64(2),
			"clvConfidenceMin": float64(0), "buyerFlatFee": float64(0),
		},
		"budget": map[string]any{
			"dailyBudget": float64(0), "dailySpecQuantityLimit": float64(0),
		},
	}
}

func TestFetchCampaigns_MultiPage(t *testing.T) {
	page1 := buildListEnvelope(t, []any{campaignItem("id-1", "Page1A")}, 1, 2)
	page2 := buildListEnvelope(t, []any{campaignItem("id-2", "Page2A")}, 1, 2)

	// Build a minimal edit envelope with the formData shape the client expects.
	editRoot := map[string]any{"formData": map[string]any{}}
	packed, err := EncodeRefPacked(editRoot)
	if err != nil {
		t.Fatalf("EncodeRefPacked edit: %v", err)
	}
	editEnvMap := map[string]any{
		"type": "data",
		"nodes": []any{
			map[string]any{"type": "data", "data": packed},
		},
	}
	editEnv, err := json.Marshal(editEnvMap)
	if err != nil {
		t.Fatalf("marshal edit envelope: %v", err)
	}

	var listCalls int
	pageFetcher := &multiPageFetcher{page1: page1, page2: page2, editEnv: editEnv, calls: &listCalls}

	c := New(pageFetcher, Config{})
	got, err := c.FetchCampaigns(context.Background())
	if err != nil {
		t.Fatalf("FetchCampaigns: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 campaigns across both pages, got %d: %+v", len(got), got)
	}
	names := map[string]bool{got[0].Name: true, got[1].Name: true}
	if !names["Page1A"] || !names["Page2A"] {
		t.Errorf("expected Page1A and Page2A, got %+v", got)
	}
	if listCalls != 2 {
		t.Errorf("expected exactly 2 list page fetches, got %d", listCalls)
	}
}

// multiPageFetcher serves list pages 1 and 2 distinctly (keyed on the
// "&page=" query suffix) and a canned edit-form response for everything else.
type multiPageFetcher struct {
	page1, page2, editEnv []byte
	calls                 *int
}

func (f *multiPageFetcher) Do(_ context.Context, req FetchRequest) (FetchResponse, error) {
	if strings.Contains(req.URL, campaignsListPath) {
		*f.calls++
		if strings.Contains(req.URL, "&page=2") {
			return FetchResponse{Status: 200, Body: string(f.page2)}, nil
		}
		return FetchResponse{Status: 200, Body: string(f.page1)}, nil
	}
	return FetchResponse{Status: 200, Body: string(f.editEnv)}, nil
}

func TestFetchCampaigns_InvalidPageSize(t *testing.T) {
	page := buildListEnvelope(t, []any{campaignItem("id-1", "A")}, 0, 5) // pageSize missing/zero

	ff := &fakeFetcher{routes: map[string]string{
		campaignsListPath: string(page),
	}}

	c := New(ff, Config{})
	_, err := c.FetchCampaigns(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid pageSize, got nil")
	}
}
