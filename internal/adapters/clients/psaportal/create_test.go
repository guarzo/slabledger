package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

// fakeFetcher routes requests by URL substring to canned responses, recording
// the last POST body seen per matched key. It replaces the httptest servers now
// that the Client no longer speaks HTTP directly.
type fakeFetcher struct {
	// routes maps a URL substring to the response body to return (status 200).
	routes    map[string]string
	captured  map[string]string
	statusFor map[string]int // optional non-200 status per substring
	errFor    map[string]string
}

func (f *fakeFetcher) Do(_ context.Context, req FetchRequest) (FetchResponse, error) {
	bestSub, bestBody, found := "", "", false
	for sub, body := range f.routes {
		if strings.Contains(req.URL, sub) && len(sub) > len(bestSub) {
			bestSub, bestBody, found = sub, body, true
		}
	}
	if !found {
		return FetchResponse{Status: 404}, nil
	}
	if f.captured == nil {
		f.captured = map[string]string{}
	}
	f.captured[bestSub] = req.Body
	if e, ok := f.errFor[bestSub]; ok {
		return FetchResponse{}, fmt.Errorf("%s", e)
	}
	st := 200
	if s, ok := f.statusFor[bestSub]; ok {
		st = s
	}
	return FetchResponse{Status: st, Body: bestBody}, nil
}

// extractPayload base64-decodes the "payload" field of a captured POST body.
func extractPayload(t *testing.T, body string) string {
	t.Helper()
	var gotBody map[string]any
	if err := json.Unmarshal([]byte(body), &gotBody); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	payloadStr, ok := gotBody["payload"].(string)
	if !ok {
		t.Fatalf("payload missing or not a string: %#v", gotBody)
	}
	return payloadStr
}

func createFormData() psacampaign.CampaignFormData {
	return psacampaign.CampaignFormData{
		CampaignName: "Modern 10s", CampaignType: "CATEGORY", Category: "POKEMON",
		PrepackagedSpecListIDs: []string{}, IsActive: false,
		BidPercentage: 72, FlatFee: 3, DailyBudget: 3000, DailySpecLimit: 2,
		GradeMinimum: "10", GradeMaximum: "10", YearMinimum: 2024, YearMaximum: 2026,
		PriceMinimum: 500, PriceMaximum: 3000, CardLadderConfidenceMinimum: 3,
		PublisherFilterType: "Target", SelectedPublishers: []psacampaign.SubjectRef{},
		SubjectFilterType: "Target", SelectedSubjects: []psacampaign.SubjectRef{},
		DeniedSpecs: []psacampaign.SubjectRef{},
	}
}

// bundleRoutes returns fakeFetcher routes that model the SvelteKit client bundle
// crawl fetchRemoteHash performs: landing page -> app entry chunk -> a chunk that
// carries the remote-function id literals. The remote hash here is "abc123", so
// create/update POSTs must target /_app/remote/abc123/{fn}.
func bundleRoutes() map[string]string {
	return map[string]string{
		"/buyercampaignmanager":          `<html><script src="/buyercampaignmanager/_app/immutable/entry/app.HASH123.js"></script></html>`,
		"immutable/entry/app.HASH123.js": `const __vite__mapDeps=(d=["../nodes/6.NODE.js","../chunks/REMOTE.js"]);`,
		"immutable/nodes/6.NODE.js":      `export const component=()=>{};`,
		"immutable/chunks/REMOTE.js":     `x=_t("abc123/createCampaign"),y=_t("abc123/updateCampaign")`,
	}
}

func TestCreateCampaign_PostsBareFormDataAndDecodesID(t *testing.T) {
	routes := bundleRoutes()
	// result is a JSON *string* containing a ref-packed array (verified live 2026-07-14)
	routes["/buyercampaignmanager/_app/remote/abc123/createCampaign"] = `{"type":"result","result":"[{\"campaignRequestId\":1,\"status\":2},\"uuid-new-1\",\"PAUSED\"]"}`
	ff := &fakeFetcher{routes: routes}

	c := New(ff, Config{})
	id, err := c.CreateCampaign(context.Background(), createFormData())
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if id != "uuid-new-1" {
		t.Fatalf("id = %q, want uuid-new-1", id)
	}

	payloadStr := extractPayload(t, ff.captured["/buyercampaignmanager/_app/remote/abc123/createCampaign"])
	decoded, err := base64.StdEncoding.DecodeString(payloadStr)
	if err != nil {
		t.Fatalf("base64: %v", err)
	}
	var packed []json.RawMessage
	if err := json.Unmarshal(decoded, &packed); err != nil {
		t.Fatalf("unmarshal packed: %v", err)
	}
	root, err := DecodeRefPacked(packed)
	if err != nil {
		t.Fatalf("DecodeRefPacked: %v", err)
	}
	// Root must be the BARE formData object — not the update path's [{id, formData}].
	fd, ok := root.(map[string]any)
	if !ok {
		t.Fatalf("root is %T, want bare formData object", root)
	}
	if fd["campaignName"] != "Modern 10s" {
		t.Fatalf("campaignName = %v", fd["campaignName"])
	}
	if v, ok := fd["isActive"].(bool); !ok || v {
		t.Fatalf("isActive = %#v, want false (born paused)", fd["isActive"])
	}
	if v, ok := fd["bidPercentage"].(float64); !ok || v != 72 {
		t.Fatalf("bidPercentage = %#v, want JSON number 72", fd["bidPercentage"])
	}
	if v, ok := fd["gradeMinimum"].(string); !ok || v != "10" {
		t.Fatalf("gradeMinimum = %#v, want string \"10\"", fd["gradeMinimum"])
	}
	if v, ok := fd["priceMaximum"].(float64); !ok || v != 3000 {
		t.Fatalf("priceMaximum = %#v, want JSON number 3000 (whole USD)", fd["priceMaximum"])
	}
	if _, ok := fd["selectedSubjects"].([]any); !ok {
		t.Fatalf("selectedSubjects = %#v, want JSON array (not null)", fd["selectedSubjects"])
	}
}

func TestCreateCampaign_Failures(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		statusCode int
		wantErr    string
	}{
		{name: "non-200", response: `{}`, statusCode: 500, wantErr: "status 500"},
		{name: "wrong envelope type", response: `{"type":"error"}`, statusCode: 200, wantErr: `type "error"`},
		{name: "undecodable result", response: `{"type":"result","result":"not-json"}`, statusCode: 200, wantErr: "may exist on portal"},
		{name: "missing id", response: `{"type":"result","result":"[{\"status\":1},\"PAUSED\"]"}`, statusCode: 200, wantErr: "may exist on portal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := bundleRoutes()
			routes["/buyercampaignmanager/_app/remote/abc123/createCampaign"] = tt.response
			ff := &fakeFetcher{
				routes:    routes,
				statusFor: map[string]int{"/buyercampaignmanager/_app/remote/abc123/createCampaign": tt.statusCode},
			}

			c := New(ff, Config{})
			_, err := c.CreateCampaign(context.Background(), createFormData())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
