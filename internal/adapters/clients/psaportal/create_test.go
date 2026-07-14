package psaportal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/psacampaign"
)

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

func TestCreateCampaign_PostsBareFormDataAndDecodesID(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/createCampaign"):
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			// result is a JSON *string* containing a ref-packed array (verified live 2026-07-14)
			_, _ = w.Write([]byte(`{"type":"result","result":"[{\"campaignRequestId\":1,\"status\":2},\"uuid-new-1\",\"PAUSED\"]"}`))
		default:
			_, _ = w.Write([]byte(`<html>build/app/immutable/entry/app.HASH123.js</html>`))
		}
	}))
	defer srv.Close()

	c := New(stubTokens{tok: "tok"}, Config{PSABaseURL: srv.URL})
	id, err := c.CreateCampaign(context.Background(), createFormData())
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if id != "uuid-new-1" {
		t.Fatalf("id = %q, want uuid-new-1", id)
	}

	payloadStr, ok := gotBody["payload"].(string)
	if !ok {
		t.Fatalf("payload missing or not a string: %#v", gotBody)
	}
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
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/createCampaign") {
					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.response))
					return
				}
				_, _ = w.Write([]byte(`<html>build/app/immutable/entry/app.HASH123.js</html>`))
			}))
			defer srv.Close()

			c := New(stubTokens{tok: "tok"}, Config{PSABaseURL: srv.URL})
			_, err := c.CreateCampaign(context.Background(), createFormData())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
