package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

func TestPushCampaign_MutatesAndPosts(t *testing.T) {
	edit, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	routes := bundleRoutes()
	routes["/edit/__data.json?x-sveltekit-invalidated=0001"] = string(edit)
	routes["/buyercampaignmanager/_app/remote/abc123/updateCampaign"] = `{"type":"result","result":"[{}]"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	err = c.PushCampaign(context.Background(), "660a980d-bf1c-4988-9958-1eb2d1853c66",
		[]psacampaign.FieldChange{
			{Field: "bidPercentage", Old: "70", New: "80"},
			{Field: "priceMaximum", Old: "3000", New: "600"},
		})
	if err != nil {
		t.Fatalf("PushCampaign: %v", err)
	}
	payloadStr := extractPayload(t, ff.captured["/buyercampaignmanager/_app/remote/abc123/updateCampaign"])

	decoded, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		t.Fatalf("decode base64 payload: %v", err)
	}
	var packed []json.RawMessage
	if err := json.Unmarshal(decoded, &packed); err != nil {
		t.Fatalf("unmarshal ref-packed array: %v", err)
	}
	resolved, err := DecodeRefPacked(packed)
	if err != nil {
		t.Fatalf("DecodeRefPacked: %v", err)
	}
	arr, ok := resolved.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("expected single-element array, got %#v", resolved)
	}
	entry, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("expected object entry, got %#v", arr[0])
	}
	formData, ok := entry["formData"].(map[string]any)
	if !ok {
		t.Fatalf("expected formData object, got %#v", entry["formData"])
	}

	bp, ok := formData["bidPercentage"].(float64)
	if !ok {
		t.Fatalf("expected bidPercentage as JSON number, got %T: %#v", formData["bidPercentage"], formData["bidPercentage"])
	}
	if bp != 80 {
		t.Errorf("bidPercentage = %v, want 80", bp)
	}

	// String fields (e.g. gradeMinimum) must remain strings, not be numified.
	if gm, ok := formData["gradeMinimum"]; ok {
		if _, isString := gm.(string); !isString {
			t.Errorf("gradeMinimum should remain a string, got %T: %#v", gm, gm)
		}
	}

	pm, ok := formData["priceMaximum"].(float64)
	if !ok {
		t.Fatalf("expected priceMaximum as JSON number, got %T: %#v", formData["priceMaximum"], formData["priceMaximum"])
	}
	if pm != 600 {
		t.Errorf("priceMaximum = %v, want 600", pm)
	}
}

func TestPushCampaign_UnknownFieldRejected(t *testing.T) {
	edit, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	routes := bundleRoutes()
	routes["/edit/__data.json?x-sveltekit-invalidated=0001"] = string(edit)
	routes["/buyercampaignmanager/_app/remote/abc123/updateCampaign"] = `{"type":"result","result":"[{}]"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	err = c.PushCampaign(context.Background(), "660a980d-bf1c-4988-9958-1eb2d1853c66",
		[]psacampaign.FieldChange{{Field: "biddPercentage", Old: "70", New: "80"}})
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "biddPercentage") {
		t.Errorf("expected error to mention unknown field name, got: %v", err)
	}
	if _, ok := ff.captured["/buyercampaignmanager/_app/remote/abc123/updateCampaign"]; ok {
		t.Error("expected no POST to updateCampaign for unknown field")
	}
}
