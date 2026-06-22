package psaportal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubTokens struct{ tok string }

func (s stubTokens) accessToken(ctx context.Context) (string, error) { return s.tok, nil }

func TestClient_FetchRows(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/buyercampaignmanager/analytics/__data.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			t.Error("missing cookie")
		}
		// embedUrl host can be the test server; only projectUuid + token are used.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"data","nodes":[{"type":"data","data":[{"embedUrl":1},"` +
			srv.URL + `/embed/proj_1#jwt_123"]}]}`))
	})
	mux.HandleFunc("/api/v1/embed/proj_1/dashboard", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":{"tiles":[
			{"uuid":"other","properties":{"chartSlug":"embed-daily"}},
			{"uuid":"tile_itemized","properties":{"chartSlug":"embed-itemized-purchases"}}]}}`))
	})
	mux.HandleFunc("/api/v1/embed/proj_1/chart-and-results", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","results":{"rows":[
			{"fct_instantoffers_offers_cert_number":{"value":{"raw":"12345678","formatted":"12345678"}},
			 "marketplace_listings_total_listing_final_price_metric":{"value":{"raw":"100.00","formatted":"$100.00"}}}]}}`))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	c := New(stubTokens{tok: "at_1"}, Config{PSABaseURL: srv.URL, LightdashBaseURL: srv.URL})
	rows, err := c.FetchRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].CertNumber != "12345678" || rows[0].PricePaid != 100.0 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestParseEmbedURL(t *testing.T) {
	p, j, err := parseEmbedURL("https://collectors.lightdash.cloud/embed/abc-123#tok.tok.tok")
	if err != nil || p != "abc-123" || j != "tok.tok.tok" {
		t.Fatalf("p=%q j=%q err=%v", p, j, err)
	}
	if _, _, err := parseEmbedURL("https://x/embed/abc-123"); err == nil {
		t.Fatal("expected error when missing #token")
	}
}
