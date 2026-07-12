package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

func TestPushCampaign_MutatesAndPosts(t *testing.T) {
	edit, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/edit/"):
			_, _ = w.Write(edit)
		case strings.Contains(r.URL.Path, "/updateCampaign"):
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"type":"result","result":"[{}]"}`))
		default:
			_, _ = w.Write([]byte(`<html>build/app/immutable/entry/app.HASH123.js</html>`))
		}
	}))
	defer srv.Close()

	c := New(stubTokens{tok: "tok"}, Config{PSABaseURL: srv.URL})
	err = c.PushCampaign(context.Background(), "660a980d-bf1c-4988-9958-1eb2d1853c66",
		[]psacampaign.FieldChange{
			{Field: "bidPercentage", Old: "70", New: "80"},
			{Field: "priceMaximum", Old: "3000", New: "600"},
		})
	if err != nil {
		t.Fatalf("PushCampaign: %v", err)
	}
	payloadRaw, ok := gotBody["payload"]
	if !ok {
		t.Fatal("expected base64 payload in POST body")
	}
	payloadStr, ok := payloadRaw.(string)
	if !ok {
		t.Fatalf("payload not a string: %T", payloadRaw)
	}

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
		t.Skipf("fixture missing: %v", err)
	}
	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/edit/"):
			_, _ = w.Write(edit)
		case strings.Contains(r.URL.Path, "/updateCampaign"):
			posted = true
			_, _ = w.Write([]byte(`{"type":"result","result":"[{}]"}`))
		default:
			_, _ = w.Write([]byte(`<html>build/app/immutable/entry/app.HASH123.js</html>`))
		}
	}))
	defer srv.Close()

	c := New(stubTokens{tok: "tok"}, Config{PSABaseURL: srv.URL})
	err = c.PushCampaign(context.Background(), "660a980d-bf1c-4988-9958-1eb2d1853c66",
		[]psacampaign.FieldChange{{Field: "biddPercentage", Old: "70", New: "80"}})
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "biddPercentage") {
		t.Errorf("expected error to mention unknown field name, got: %v", err)
	}
	if posted {
		t.Error("expected no POST to updateCampaign for unknown field")
	}
}
