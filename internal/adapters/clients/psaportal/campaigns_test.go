package psaportal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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
	got, _, err := c.FetchCampaigns(context.Background())
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
	got, _, err := c.FetchCampaigns(context.Background())
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
	_, _, err := c.FetchCampaigns(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid pageSize, got nil")
	}
}

func TestFetchCampaignFormData_DecodesSpecListCatalog(t *testing.T) {
	editRoot := map[string]any{
		"formData": map[string]any{
			"prepackagedSpecListIds": []any{"list-en-base", "list-en-holo"},
		},
		"prepackagedSpecLists": []any{
			map[string]any{"id": "list-en-base", "name": "English Base Set", "status": "ENABLED"},
			map[string]any{"id": "list-en-holo", "name": "English Holo Rares", "status": "ENABLED"},
			map[string]any{"id": "list-jp-base", "name": "Japanese Base Set", "status": "DISABLED"},
		},
	}
	packed, err := EncodeRefPacked(editRoot)
	if err != nil {
		t.Fatalf("EncodeRefPacked: %v", err)
	}
	env := map[string]any{
		"type": "data",
		"nodes": []any{
			map[string]any{"type": "data", "data": packed},
		},
	}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	ff := &fakeFetcher{routes: map[string]string{
		fmt.Sprintf(campaignEditPathF, "camp-1"): string(body),
	}}

	c := New(ff, Config{})
	fd, specLists, err := c.fetchCampaignFormData(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("fetchCampaignFormData: %v", err)
	}
	if want := []string{"list-en-base", "list-en-holo"}; !reflect.DeepEqual(fd.PrepackagedSpecListIDs, want) {
		t.Errorf("PrepackagedSpecListIDs = %v, want %v", fd.PrepackagedSpecListIDs, want)
	}
	if len(specLists) != 3 {
		t.Fatalf("expected 3 catalog entries, got %d: %+v", len(specLists), specLists)
	}
	if specLists[2].ID != "list-jp-base" || specLists[2].Status != "DISABLED" {
		t.Errorf("specLists[2] = %+v, want {list-jp-base ... DISABLED}", specLists[2])
	}
}

func TestFetchCampaigns_EditFetchFailure_TargetingIncomplete(t *testing.T) {
	page := buildListEnvelope(t, []any{campaignItem("id-1", "Flaky")}, 1, 1)
	editURL := fmt.Sprintf(campaignEditPathF, "id-1")
	ff := &fakeFetcher{
		routes: map[string]string{
			campaignsListPath: string(page),
			editURL:           "",
		},
		statusFor: map[string]int{
			editURL: 403,
		},
	}

	c := New(ff, Config{})
	got, catalog, err := c.FetchCampaigns(context.Background())
	if err != nil {
		t.Fatalf("FetchCampaigns: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 campaign despite edit-form failure, got %d", len(got))
	}
	if got[0].TargetingComplete {
		t.Error("TargetingComplete = true, want false after edit-form fetch failure")
	}
	if len(got[0].SpecListIDs) != 0 {
		t.Errorf("SpecListIDs = %v, want empty when targeting is incomplete", got[0].SpecListIDs)
	}
	if catalog != nil {
		t.Errorf("catalog = %+v, want nil when no edit-form fetch succeeded", catalog)
	}
}

// editEnvelope packs a formData + prepackagedSpecLists pair into a SvelteKit
// __data.json envelope, the shape fetchCampaignFormData decodes.
func editEnvelope(t *testing.T, specListIDs []any, catalog []any) string {
	t.Helper()
	packed, err := EncodeRefPacked(map[string]any{
		"formData":             map[string]any{"prepackagedSpecListIds": specListIDs},
		"prepackagedSpecLists": catalog,
	})
	if err != nil {
		t.Fatalf("EncodeRefPacked: %v", err)
	}
	b, err := json.Marshal(map[string]any{
		"type":  "data",
		"nodes": []any{map[string]any{"type": "data", "data": packed}},
	})
	if err != nil {
		t.Fatalf("marshal edit envelope: %v", err)
	}
	return string(b)
}

// TestFetchCampaigns_MergesSpecListCatalog pins the merge semantics. Each
// edit-form response carries the catalog as the portal rendered it for that one
// campaign, so a truncated or partial response arriving last must not shrink
// what earlier responses already established — taking the last response's
// catalog would silently drop entries, and the harvester persists the result as
// the reference data the main server resolves portal ids against.
func TestFetchCampaigns_MergesSpecListCatalog(t *testing.T) {
	page := buildListEnvelope(t, []any{
		campaignItem("id-1", "Full"),
		campaignItem("id-2", "Truncated"),
	}, 2, 2)

	// id-1 sees the whole catalog; id-2's response is short and re-states
	// list-b under a newer name.
	full := editEnvelope(t, []any{"list-a"}, []any{
		map[string]any{"id": "list-a", "name": "A", "status": "ENABLED"},
		map[string]any{"id": "list-b", "name": "B (stale)", "status": "ENABLED"},
		// Blank ids are skipped rather than stored under "".
		map[string]any{"id": "", "name": "Nameless", "status": "ENABLED"},
	})
	short := editEnvelope(t, []any{"list-b"}, []any{
		map[string]any{"id": "list-b", "name": "B (fresh)", "status": "DISABLED"},
		map[string]any{"id": "list-c", "name": "C", "status": "ENABLED"},
	})

	ff := &fakeFetcher{routes: map[string]string{
		campaignsListPath:                      string(page),
		fmt.Sprintf(campaignEditPathF, "id-1"): full,
		fmt.Sprintf(campaignEditPathF, "id-2"): short,
	}}

	c := New(ff, Config{})
	_, catalog, err := c.FetchCampaigns(context.Background())
	if err != nil {
		t.Fatalf("FetchCampaigns: %v", err)
	}

	// Union, in first-seen order — not id-2's two entries.
	wantIDs := []string{"list-a", "list-b", "list-c"}
	gotIDs := make([]string, 0, len(catalog))
	for _, sl := range catalog {
		gotIDs = append(gotIDs, sl.ID)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("catalog ids = %v, want %v (union in first-seen order)", gotIDs, wantIDs)
	}
	// A later response re-stating an id updates its value in place: the portal
	// is the authority on the current name/status, and the order is what the
	// merge preserves.
	if len(catalog) == 3 && (catalog[1].Name != "B (fresh)" || catalog[1].Status != "DISABLED") {
		t.Errorf("catalog[1] = %+v, want the later response's name and status", catalog[1])
	}
}

// TestFetchCampaigns_AppliesSpecListTargeting exercises the happy path this
// task exists for: a successful edit-form fetch must land SpecListIDs,
// SpecListNames (including the catalog's skip-unknown-id branch), and
// TargetingComplete on the resulting PortalCampaign, and FetchCampaigns must
// return the catalog it decoded alongside it.
func TestFetchCampaigns_AppliesSpecListTargeting(t *testing.T) {
	page := buildListEnvelope(t, []any{campaignItem("id-1", "Targeted")}, 1, 1)
	editRoot := map[string]any{
		"formData": map[string]any{
			// list-missing has no matching catalog entry below, so
			// specListNames must skip it rather than fail the decode.
			"prepackagedSpecListIds": []any{"list-en-base", "list-missing"},
		},
		"prepackagedSpecLists": []any{
			map[string]any{"id": "list-en-base", "name": "English Base Set", "status": "ENABLED"},
		},
	}
	packed, err := EncodeRefPacked(editRoot)
	if err != nil {
		t.Fatalf("EncodeRefPacked: %v", err)
	}
	editEnv, err := json.Marshal(map[string]any{
		"type": "data",
		"nodes": []any{
			map[string]any{"type": "data", "data": packed},
		},
	})
	if err != nil {
		t.Fatalf("marshal edit envelope: %v", err)
	}
	ff := &fakeFetcher{routes: map[string]string{
		campaignsListPath:                      string(page),
		fmt.Sprintf(campaignEditPathF, "id-1"): string(editEnv),
	}}

	c := New(ff, Config{})
	got, catalog, err := c.FetchCampaigns(context.Background())
	if err != nil {
		t.Fatalf("FetchCampaigns: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 campaign, got %d: %+v", len(got), got)
	}
	pc := got[0]
	if !pc.TargetingComplete {
		t.Error("TargetingComplete = false, want true on a successful edit-form fetch")
	}
	if want := []string{"list-en-base", "list-missing"}; !reflect.DeepEqual(pc.SpecListIDs, want) {
		t.Errorf("SpecListIDs = %v, want %v", pc.SpecListIDs, want)
	}
	if want := []string{"English Base Set"}; !reflect.DeepEqual(pc.SpecListNames, want) {
		t.Errorf("SpecListNames = %v, want %v (list-missing skipped)", pc.SpecListNames, want)
	}
	if len(catalog) != 1 || catalog[0].ID != "list-en-base" {
		t.Errorf("catalog = %+v, want the single decoded catalog entry", catalog)
	}
}
