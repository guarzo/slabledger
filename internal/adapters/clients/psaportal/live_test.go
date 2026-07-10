//go:build integration

// Run with: PSA_LIVE_TOKEN=<access token> go test -tags integration -run TestLiveFetchRows ./internal/adapters/clients/psaportal/
package psaportal

import (
	"context"
	"os"
	"testing"
)

type staticTok struct{ t string }

func (s staticTok) AccessToken(ctx context.Context) (string, error) { return s.t, nil }

// TestLiveFetchRows exercises the full chain against the real portal using a
// real access token from PSA_LIVE_TOKEN. Skipped unless that env var is set.
func TestLiveFetchRows(t *testing.T) {
	tok := os.Getenv("PSA_LIVE_TOKEN")
	if tok == "" {
		t.Skip("set PSA_LIVE_TOKEN to run the live pipeline check")
	}
	c := New(staticTok{tok}, Config{})
	rows, err := c.FetchRows(context.Background())
	if err != nil {
		t.Fatalf("live FetchRows failed: %v", err)
	}
	t.Logf("LIVE FetchRows OK — %d rows", len(rows))
	for i, r := range rows {
		if i >= 3 {
			break
		}
		t.Logf("  row: cert=%q grade=%v price=%v title=%q ship=%q refunded=%v",
			r.CertNumber, r.Grade, r.PricePaid, r.ListingTitle, r.ShipDate, r.WasRefunded)
	}
}
