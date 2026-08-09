package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

func TestPushCampaign_MutatesAndPosts(t *testing.T) {
	edit, err := os.ReadFile("testdata/psa-campaign-edit-raw.json")
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
	// PSA's own client calls updateCampaign with a BARE object argument —
	// `K({id, formData})` in the compiled edit-page chunk (nodes/11.*.js) —
	// mirroring createCampaign's `I({...formData})`. Wrapping it in a
	// single-element array makes the remote function destructure `undefined`
	// and return an opaque 500 "Internal Error".
	entry, ok := resolved.(map[string]any)
	if !ok {
		t.Fatalf("expected bare object payload, got %#v", resolved)
	}
	if got := entry["id"]; got != "660a980d-bf1c-4988-9958-1eb2d1853c66" {
		t.Errorf("id = %#v, want the campaign id", got)
	}
	formData, ok := entry["formData"].(map[string]any)
	if !ok {
		t.Fatalf("expected formData object, got %#v", entry["formData"])
	}

	// Each changed/preserved field is checked for both its JSON type and its
	// value: numerics must stay JSON numbers and grade bounds must stay
	// strings, since PSA rejects a type-shifted record.
	tests := []struct {
		name  string
		field string
		want  any
	}{
		{name: "changed numeric field", field: "bidPercentage", want: float64(80)},
		{name: "changed numeric field with cents-like value", field: "priceMaximum", want: float64(600)},
		{name: "grade bound stays a string", field: "gradeMinimum", want: "1"},
		{name: "grade bound stays a string (max)", field: "gradeMaximum", want: "10"},
		{name: "untouched numeric field preserved", field: "yearMinimum", want: float64(2002)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := formData[tt.field]
			if !ok {
				t.Fatalf("formData missing field %q", tt.field)
			}
			if got != tt.want {
				t.Errorf("%s = %#v (%T), want %#v (%T)", tt.field, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestPushCampaign_ListValuedFieldUsesValueOverNew(t *testing.T) {
	edit, err := os.ReadFile("testdata/psa-campaign-edit-raw.json")
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
			{Field: "selectedSubjects", Old: "4807:Charizard,4836:Umbreon,4842:Lugia,4844:Celebi,4851:Mudkip", New: "22210:Machamp", Value: []psacampaign.SubjectRef{{ID: 22210, Name: "Machamp"}}},
		})
	if err != nil {
		t.Fatalf("PushCampaign: %v", err)
	}
	payloadStr := extractPayload(t, ff.captured["/buyercampaignmanager/_app/remote/abc123/updateCampaign"])
	decoded, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	var packed []json.RawMessage
	if err := json.Unmarshal(decoded, &packed); err != nil {
		t.Fatalf("unmarshal packed: %v", err)
	}
	resolved, err := DecodeRefPacked(packed)
	if err != nil {
		t.Fatalf("DecodeRefPacked: %v", err)
	}
	entry := resolved.(map[string]any)
	formData := entry["formData"].(map[string]any)
	subjects, ok := formData["selectedSubjects"].([]any)
	if !ok {
		t.Fatalf("selectedSubjects = %#v (%T), want a JSON array (from Value, not the New string)", formData["selectedSubjects"], formData["selectedSubjects"])
	}
	if len(subjects) != 1 {
		t.Fatalf("len(subjects) = %d, want 1", len(subjects))
	}
	entry0 := subjects[0].(map[string]any)
	if entry0["id"] != float64(22210) || entry0["name"] != "Machamp" {
		t.Errorf("subjects[0] = %+v, want {id:22210 name:Machamp}", entry0)
	}
}

func TestPushCampaign_UnknownFieldRejected(t *testing.T) {
	edit, err := os.ReadFile("testdata/psa-campaign-edit-raw.json")
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

// A 200 response whose envelope type is not "result" is a portal-side rejection;
// the error must surface the response body so the failure is diagnosable without
// re-running against the live portal (W-016).
func TestPushCampaign_ErrorEnvelopeSurfacesBody(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantIn   string
	}{
		{
			name:     "short body surfaced verbatim",
			response: `{"type":"error","error":{"message":"priceMinimum below floor"}}`,
			wantIn:   "priceMinimum below floor",
		},
		{
			name:     "long body is truncated",
			response: `{"type":"error","msg":"` + strings.Repeat("x", 600) + `"}`,
			wantIn:   "…(truncated)",
		},
	}
	edit, err := os.ReadFile("testdata/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := bundleRoutes()
			routes["/edit/__data.json?x-sveltekit-invalidated=0001"] = string(edit)
			routes["/buyercampaignmanager/_app/remote/abc123/updateCampaign"] = tt.response
			ff := &fakeFetcher{routes: routes}

			c := New(ff, Config{})
			err := c.PushCampaign(context.Background(), "660a980d-bf1c-4988-9958-1eb2d1853c66",
				[]psacampaign.FieldChange{{Field: "priceMinimum", Old: "500", New: "200"}})
			if err == nil {
				t.Fatal("expected error for non-result envelope, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("expected error to contain %q, got: %v", tt.wantIn, err)
			}
		})
	}
}

// A push whose FieldChange.Old no longer matches the live portal record would
// silently overwrite an edit made after approval. The signature cannot catch
// this — it proves the operator approved the diff, not that the portal still
// holds what they were shown — so PushCampaign refuses before mutating anything.
func TestPushCampaign_RefusesStaleChange(t *testing.T) {
	tests := []struct {
		name    string
		change  psacampaign.FieldChange
		wantErr error
		wantIn  string
	}{
		{
			name:   "live value matches approval",
			change: psacampaign.FieldChange{Field: "bidPercentage", Old: "70", New: "80"},
		},
		{
			// Someone raised the bid in the PSA UI after the operator approved.
			name:    "scalar changed since approval",
			change:  psacampaign.FieldChange{Field: "bidPercentage", Old: "65", New: "80"},
			wantErr: psacampaign.ErrFieldStale,
			wantIn:  `was "65" at approval, portal now holds "70"`,
		},
		{
			name:    "list changed since approval",
			change:  psacampaign.FieldChange{Field: "deniedSpecs", Old: "1:Gone", New: "", Value: []psacampaign.SubjectRef{}},
			wantErr: psacampaign.ErrFieldStale,
		},
		{
			// A field with no canonical rendering cannot be compare-and-swapped,
			// and an unverifiable field is not a safe one to push.
			name:    "unrenderable field refused",
			change:  psacampaign.FieldChange{Field: "isActive", Old: "false", New: "true"},
			wantErr: psacampaign.ErrNoRenderer,
		},
	}
	edit, err := os.ReadFile("testdata/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := bundleRoutes()
			routes["/edit/__data.json?x-sveltekit-invalidated=0001"] = string(edit)
			routes["/buyercampaignmanager/_app/remote/abc123/updateCampaign"] = `{"type":"result","result":"[{}]"}`
			ff := &fakeFetcher{routes: routes}

			err := New(ff, Config{}).PushCampaign(context.Background(),
				"660a980d-bf1c-4988-9958-1eb2d1853c66", []psacampaign.FieldChange{tt.change})

			_, posted := ff.captured["/buyercampaignmanager/_app/remote/abc123/updateCampaign"]
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("PushCampaign: %v", err)
				}
				if !posted {
					t.Error("expected a POST to updateCampaign for a fresh change")
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantIn != "" && !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("err = %v, want it to contain %q", err, tt.wantIn)
			}
			if posted {
				t.Error("refused push still POSTed to updateCampaign")
			}
		})
	}
}

// The staleness check compares a FieldChange.Old rendered from the LIST endpoint
// (TranslateToDiff's input) against the live EDIT FORM. Nothing in the types
// forces those two portal views to agree, so this pins it with the one campaign
// that appears in both captured fixtures. If PSA ever changes units or spelling
// on one endpoint, this fails here rather than by refusing every real update.
func TestPortalListAndEditFormRenderIdentically(t *testing.T) {
	const campaignID = "660a980d-bf1c-4988-9958-1eb2d1853c66"

	listRaw, err := os.ReadFile("testdata/psa-campaigns-raw.json")
	if err != nil {
		t.Fatalf("list fixture missing: %v", err)
	}
	editRaw, err := os.ReadFile("testdata/psa-campaign-edit-raw.json")
	if err != nil {
		t.Fatalf("edit fixture missing: %v", err)
	}

	listRoot := decodeEnvelope(t, listRaw)
	items, _, _, err := campaignItems(listRoot)
	if err != nil {
		t.Fatalf("campaignItems: %v", err)
	}
	var listSide psacampaign.CampaignFormData
	var found bool
	for _, it := range items {
		pc, err := mapListItem(it)
		if err != nil {
			t.Fatalf("mapListItem: %v", err)
		}
		if pc.CampaignRequestID == campaignID {
			listSide, found = psacampaign.PortalFormData(pc), true
		}
	}
	if !found {
		t.Fatalf("campaign %s not in the list fixture", campaignID)
	}

	editRoot, ok := decodeEnvelope(t, editRaw).(map[string]any)
	if !ok {
		t.Fatal("edit root not an object")
	}
	editSide, err := decodeFormData(editRoot["formData"].(map[string]any))
	if err != nil {
		t.Fatalf("decodeFormData: %v", err)
	}

	// Every field TranslateToDiff can emit. The three subject/spec-list fields
	// are excluded: the list endpoint does not carry them at all (FetchCampaigns
	// backfills them from this same edit form via applyFormData), so comparing
	// the two views of them would only restate that projection.
	for _, field := range []string{
		"bidPercentage", "dailyBudget", "gradeMinimum", "gradeMaximum",
		"yearMinimum", "yearMaximum", "priceMinimum", "priceMaximum",
		"cardLadderConfidenceMinimum", "flatFee", "dailySpecLimit", "campaignName",
	} {
		t.Run(field, func(t *testing.T) {
			fromList, ok := psacampaign.RenderFieldValue(field, listSide)
			if !ok {
				t.Fatalf("no renderer for %q", field)
			}
			fromEdit, _ := psacampaign.RenderFieldValue(field, editSide)
			if fromList != fromEdit {
				t.Errorf("list renders %q, edit form renders %q", fromList, fromEdit)
			}
		})
	}
}

// decodeEnvelope resolves a captured SvelteKit __data.json fixture.
func decodeEnvelope(t *testing.T, raw []byte) any {
	t.Helper()
	packed, err := packedFromEnvelope(raw)
	if err != nil {
		t.Fatalf("packedFromEnvelope: %v", err)
	}
	root, err := DecodeRefPacked(packed)
	if err != nil {
		t.Fatalf("DecodeRefPacked: %v", err)
	}
	return root
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		wantSuffix   string
		wantExact    string // when set, output must equal this exactly
		wantRuneLen  int    // when > 0, expected rune count of output
		wantValidUTF bool
	}{
		{name: "short passes through", in: "hello", wantExact: "hello", wantValidUTF: true},
		{name: "exactly 500 runes passes through", in: strings.Repeat("a", 500), wantExact: strings.Repeat("a", 500), wantValidUTF: true},
		{name: "501 ascii runes truncated", in: strings.Repeat("a", 501), wantSuffix: "…(truncated)", wantRuneLen: 500 + len([]rune("…(truncated)")), wantValidUTF: true},
		// 501 multi-byte runes: a naive byte slice at 500 would split a rune;
		// rune-based truncation must keep the output valid UTF-8.
		{name: "utf8 boundary not split", in: strings.Repeat("é", 501), wantSuffix: "…(truncated)", wantRuneLen: 500 + len([]rune("…(truncated)")), wantValidUTF: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateBody(tt.in)
			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("truncateBody() = %q, want %q", got, tt.wantExact)
			}
			if tt.wantSuffix != "" && !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("truncateBody() = %q, want suffix %q", got, tt.wantSuffix)
			}
			if tt.wantRuneLen > 0 && utf8.RuneCountInString(got) != tt.wantRuneLen {
				t.Errorf("truncateBody() rune count = %d, want %d", utf8.RuneCountInString(got), tt.wantRuneLen)
			}
			if tt.wantValidUTF && !utf8.ValidString(got) {
				t.Errorf("truncateBody() produced invalid UTF-8: %q", got)
			}
		})
	}
}

func TestDumpFormData(t *testing.T) {
	tests := []struct {
		name       string
		in         map[string]any
		wantSubs   []string // substrings that must all be present
		wantPrefix string   // when set, output must start with this
		maxLen     int      // when > 0, output length must not exceed this
	}{
		{
			name:     "sorted keys with types and values",
			in:       map[string]any{"priceMinimum": 200.0, "gradeMinimum": "9"},
			wantSubs: []string{"gradeMinimum(string)=9", "priceMinimum(float64)=200"},
		},
		{
			name:     "nil value renders",
			in:       map[string]any{"selectedSubjects": nil},
			wantSubs: []string{"selectedSubjects(<nil>)=<nil>"},
		},
		{
			name:     "long ascii value truncated",
			in:       map[string]any{"note": strings.Repeat("z", 200)},
			wantSubs: []string{"note(string)=" + strings.Repeat("z", 80) + "…"},
		},
		{
			// A byte-based cap would split the final multi-byte rune; the value
			// must be exactly 80 whole runes followed by the ellipsis.
			name:     "long multibyte value truncated on rune boundary",
			in:       map[string]any{"note": strings.Repeat("é", 81)},
			wantSubs: []string{"note(string)=" + strings.Repeat("é", 80) + "…"},
		},
		{
			// Keys are capped at 64 runes, rune-safe.
			name:     "long multibyte key truncated on rune boundary",
			in:       map[string]any{strings.Repeat("é", 65): 1},
			wantSubs: []string{strings.Repeat("é", 64) + "…(int)=1"},
		},
		{
			name:       "keys emitted in sorted order",
			in:         map[string]any{"c": 3, "a": 1, "b": 2},
			wantPrefix: "a(int)=1, b(int)=2, c(int)=3",
		},
		{
			// Many fields must not produce an unbounded line.
			name:     "total output is budget-capped",
			in:       manyFields(500),
			wantSubs: []string{"more fields)"},
			maxLen:   2100, // totalBudget (2000) + one trailing entry + suffix
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dumpFormData(tt.in)
			for _, sub := range tt.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("dumpFormData() = %q, want substring %q", got, sub)
				}
			}
			if tt.wantPrefix != "" && !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("dumpFormData() = %q, want prefix %q", got, tt.wantPrefix)
			}
			if tt.maxLen > 0 && len(got) > tt.maxLen {
				t.Errorf("dumpFormData() length = %d, want <= %d", len(got), tt.maxLen)
			}
		})
	}
}

// manyFields builds a formData map with n distinct keys for budget testing.
func manyFields(n int) map[string]any {
	fd := make(map[string]any, n)
	for i := 0; i < n; i++ {
		fd[fmt.Sprintf("field%04d", i)] = i
	}
	return fd
}
