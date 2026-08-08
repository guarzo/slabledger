// DH Enterprise API v2 integration tests.
// Run with: go test ./internal/integration/ -tags integration -v -run TestDHEnterprise -timeout 5m
//
//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	_ "github.com/joho/godotenv/autoload"
)

func newDHEnterpriseClient(t *testing.T) *dh.Client {
	t.Helper()
	baseURL := os.Getenv("DH_API_BASE_URL")
	enterpriseKey := os.Getenv("DH_ENTERPRISE_API_KEY")

	if baseURL == "" || enterpriseKey == "" {
		t.Skip("DH_API_BASE_URL and DH_ENTERPRISE_API_KEY required")
	}

	return dh.NewClient(baseURL,
		dh.WithEnterpriseKey(enterpriseKey),
		dh.WithRateLimitRPS(1),
	)
}

// TestDHEnterprise_ResolveCert verifies single cert resolution against the live API.
func TestDHEnterprise_ResolveCert(t *testing.T) {
	c := newDHEnterpriseClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use a known PSA cert from our inventory (testdata_test.go has real certs)
	resp, err := c.ResolveCert(ctx, dh.CertResolveRequest{
		CertNumber: "84189955",
		CardName:   "Charizard",
	})
	if err != nil {
		t.Fatalf("ResolveCert: %v", err)
	}

	t.Logf("ResolveCert result: cert=%s status=%s dh_card_id=%d card=%q set=%q grade=%q",
		resp.CertNumber, resp.Status, resp.DHCardID, resp.CardName, resp.SetName, resp.Grade)

	if resp.CertNumber != "84189955" {
		t.Errorf("CertNumber = %q, want 84189955", resp.CertNumber)
	}
	if resp.Status != dh.CertStatusMatched && resp.Status != dh.CertStatusAmbiguous && resp.Status != dh.CertStatusNotFound {
		t.Errorf("unexpected status: %q", resp.Status)
	}
}

// TestDHEnterprise_ResolveCertsBatch verifies batch cert resolution.
func TestDHEnterprise_ResolveCertsBatch(t *testing.T) {
	c := newDHEnterpriseClient(t)

	tests := []struct {
		name      string
		certs     []dh.CertResolveRequest
		wantTotal int
	}{
		{
			name: "two certs",
			certs: []dh.CertResolveRequest{
				{CertNumber: "84189955", CardName: "Charizard"},
				{CertNumber: "84149614", CardName: "Pikachu"},
			},
			wantTotal: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := c.ResolveCertsBatch(ctx, tc.certs)
			if err != nil {
				t.Fatalf("ResolveCertsBatch: %v", err)
			}

			t.Logf("Batch submitted: batch_id=%s status=%s total=%d",
				resp.BatchID, resp.Status, resp.Total)

			if resp.BatchID == "" {
				t.Error("expected non-empty BatchID")
			}
			if resp.Total != tc.wantTotal {
				t.Errorf("expected Total=%d, got %d", tc.wantTotal, resp.Total)
			}

			// Poll once to confirm the batch_status path resolves. The batch is
			// asynchronous, so this asserts reachability, not completion.
			status, err := c.GetCertResolutionJob(ctx, resp.BatchID)
			if err != nil {
				t.Fatalf("GetCertResolutionJob(%s): %v", resp.BatchID, err)
			}
			t.Logf("Batch status: status=%s completed=%d/%d", status.Status, status.Completed, status.Total)
			if status.Status == "" {
				t.Error("expected non-empty status from batch_status")
			}
		})
	}
}

// TestDHEnterprise_ListInventory verifies inventory listing.
func TestDHEnterprise_ListInventory(t *testing.T) {
	c := newDHEnterpriseClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.ListInventory(ctx, dh.InventoryFilters{
		Status:  "active",
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("ListInventory: %v", err)
	}

	t.Logf("ListInventory: %d items, total=%d (page %d/%d)",
		len(resp.Items), resp.Meta.TotalCount, resp.Meta.Page, (resp.Meta.TotalCount+resp.Meta.PerPage-1)/max(resp.Meta.PerPage, 1))

	for i, item := range resp.Items {
		if i >= 3 {
			t.Logf("  ... and %d more", len(resp.Items)-3)
			break
		}
		t.Logf("  [%d] cert=%s card=%q status=%s price=$%.2f channels=%d",
			item.DHInventoryID, item.CertNumber, item.CardName, item.Status,
			float64(item.ListingPriceCents)/100.0, len(item.Channels))
	}

	// Verify pagination meta is populated
	if resp.Meta.PerPage <= 0 {
		t.Error("expected Meta.PerPage > 0")
	}
}

// TestDHEnterprise_GetOrders verifies orders retrieval.
func TestDHEnterprise_GetOrders(t *testing.T) {
	c := newDHEnterpriseClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	since := time.Now().UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	resp, err := c.GetOrders(ctx, dh.OrderFilters{
		Since:   since,
		PerPage: 10,
	})
	if err != nil {
		t.Fatalf("GetOrders: %v", err)
	}

	t.Logf("GetOrders (since %s): %d orders, total=%d", since[:10], len(resp.Orders), resp.Meta.TotalCount)

	for i, order := range resp.Orders {
		if i >= 3 {
			t.Logf("  ... and %d more", len(resp.Orders)-3)
			break
		}
		t.Logf("  [%s] cert=%s card=%q channel=%s price=$%.2f sold_at=%s",
			order.OrderID, order.CertNumber, order.CardName, order.Channel,
			float64(order.SalePriceCents)/100.0, order.SoldAt)
	}

	// Verify pagination meta
	if resp.Meta.PerPage <= 0 {
		t.Error("expected Meta.PerPage > 0")
	}
}
