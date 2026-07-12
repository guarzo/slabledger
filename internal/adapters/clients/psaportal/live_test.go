//go:build integration

package psaportal

// Live smoke test for the portal chain. Skipped unless PSA_PORTAL_TEST_TOKEN is
// set (a valid accessToken cookie value, e.g. read from the prod DB). Must run
// from an IP Cloudflare trusts (the devcontainer qualifies; datacenter IPs do
// not — that is the whole reason the harvester exists).
//
//   PSA_PORTAL_TEST_TOKEN=<jwt> go test -tags integration ./internal/adapters/clients/psaportal/ -run TestLiveSnapshotChain -v

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func TestLiveSnapshotChain(t *testing.T) {
	tok := os.Getenv("PSA_PORTAL_TEST_TOKEN")
	if tok == "" {
		t.Skip("PSA_PORTAL_TEST_TOKEN not set")
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://www.psacard.com/buyercampaignmanager/analytics/__data.json?x-sveltekit-invalidated=001", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", "accessToken="+tok)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || strings.Contains(string(body), "Just a moment") {
		t.Fatalf("analytics fetch blocked or failed: status %d", resp.StatusCode)
	}

	h := &Harvester{ld: newLightdashClient(defaultLightdashBaseURL), logger: observability.NewNoopLogger()}
	rows, err := h.fetchRowsFromAnalytics(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fetched %d raw rows", len(rows))
	if len(rows) == 0 {
		t.Fatal("expected at least one row")
	}
}
