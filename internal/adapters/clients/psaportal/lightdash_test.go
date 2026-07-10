package psaportal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLightdashClient_TileRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.Header.Get(embedTokenHeader); got != "jwt_123" {
			t.Errorf("embed token header = %q", got)
		}
		if !strings.Contains(r.URL.Path, "/api/v1/embed/proj_1/chart-and-results") {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if req["tileUuid"] != "tile_1" {
			t.Errorf("tileUuid = %v", req["tileUuid"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","results":{"rows":[
			{"fct_instantoffers_offers_cert_number":{"value":{"raw":"12345678","formatted":"12345678"}},
			 "fct_instantoffers_offers_grade_value":{"value":{"raw":10,"formatted":"10"}},
			 "fct_instantoffers_offers_is_offer_refunded":{"value":{"raw":false,"formatted":"No"}},
			 "marketplace_listings_listing_title":{"value":{"raw":null,"formatted":""}}}
		]}}`))
	}))
	defer srv.Close()

	lc := newLightdashClient(srv.URL)
	rows, err := lc.tileRows(context.Background(), "proj_1", "jwt_123", "tile_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	r0 := rows[0]
	if r0["fct_instantoffers_offers_cert_number"] != "12345678" {
		t.Errorf("cert = %q", r0["fct_instantoffers_offers_cert_number"])
	}
	if r0["fct_instantoffers_offers_grade_value"] != "10" {
		t.Errorf("grade = %q", r0["fct_instantoffers_offers_grade_value"])
	}
	if r0["fct_instantoffers_offers_is_offer_refunded"] != "false" {
		t.Errorf("refunded = %q", r0["fct_instantoffers_offers_is_offer_refunded"])
	}
	if r0["marketplace_listings_listing_title"] != "" {
		t.Errorf("title(null) = %q", r0["marketplace_listings_listing_title"])
	}
}

func TestLightdashClient_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":"error"}`))
	}))
	defer srv.Close()
	lc := newLightdashClient(srv.URL)
	if _, err := lc.tileRows(context.Background(), "p", "j", "t"); err == nil {
		t.Fatal("expected error on 401")
	}
}
