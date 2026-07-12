package psaportal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetchCampaigns_ParsesListAndEdit(t *testing.T) {
	list, err := os.ReadFile("../../../../docs/psa-campaigns-raw.json")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	edit, err := os.ReadFile("../../../../docs/psa-campaign-edit-raw.json")
	if err != nil {
		t.Skipf("fixture missing: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/buyercampaignmanager/__data.json" {
			_, _ = w.Write(list)
			return
		}
		_, _ = w.Write(edit) // any /campaigns/{id}/edit path
	}))
	defer srv.Close()

	c := New(stubTokens{tok: "tok"}, Config{PSABaseURL: srv.URL})
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
	// Price min/max must be cents (portal 500/3000 USD -> 50000/300000).
	if got[0].BuyBox.PriceMaxCents%100 != 0 {
		t.Errorf("price not in cents: %d", got[0].BuyBox.PriceMaxCents)
	}
}
