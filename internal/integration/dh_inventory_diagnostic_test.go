// DH Inventory diagnostic — lists what the enterprise API actually reports as inventory
// and cross-checks against known DH statuses.
//
// Run with:
//   go test ./internal/integration/ -tags integration -v -run TestDH_InventoryDiagnostic -timeout 5m
//
//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	_ "github.com/joho/godotenv/autoload"
)

func TestDH_InventoryDiagnostic(t *testing.T) {
	c := newDHEnterpriseClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Log("=== DH Inventory Diagnostic ===")
	t.Log("")

	// 1. Total count (no filter) — this is what the status page reports
	t.Log("--- Query 1: No status filter (what status page shows) ---")
	respAll, err := c.ListInventory(ctx, dh.InventoryFilters{PerPage: 100})
	if err != nil {
		t.Fatalf("ListInventory (no filter): %v", err)
	}
	t.Logf("TotalCount=%d  ItemsReturned=%d  Page=%d  PerPage=%d",
		respAll.Meta.TotalCount, len(respAll.Items), respAll.Meta.Page, respAll.Meta.PerPage)

	statusCounts := map[string]int{}
	for _, item := range respAll.Items {
		statusCounts[item.Status]++
		t.Logf("  inv_id=%d  cert=%s  card=%q  set=%q  status=%s  price=$%.2f  cost=$%.2f  channels=%d  updated=%s",
			item.DHInventoryID, item.CertNumber, item.CardName, item.SetName, item.Status,
			float64(item.ListingPriceCents)/100.0, float64(item.CostBasisCents)/100.0,
			len(item.Channels), item.UpdatedAt)
	}
	t.Log("")
	t.Logf("Status breakdown: %v", statusCounts)

	// 2. "in_stock" filter
	t.Log("")
	t.Log("--- Query 2: status=in_stock ---")
	respInStock, err := c.ListInventory(ctx, dh.InventoryFilters{Status: "in_stock", PerPage: 1})
	if err != nil {
		t.Logf("ListInventory (in_stock): ERROR: %v", err)
	} else {
		t.Logf("TotalCount=%d", respInStock.Meta.TotalCount)
	}

	// 3. "listed" filter
	t.Log("")
	t.Log("--- Query 3: status=listed ---")
	respListed, err := c.ListInventory(ctx, dh.InventoryFilters{Status: "listed", PerPage: 1})
	if err != nil {
		t.Logf("ListInventory (listed): ERROR: %v", err)
	} else {
		t.Logf("TotalCount=%d", respListed.Meta.TotalCount)
	}

	// 4. "active" filter (used in the existing integration test)
	t.Log("")
	t.Log("--- Query 4: status=active ---")
	respActive, err := c.ListInventory(ctx, dh.InventoryFilters{Status: "active", PerPage: 1})
	if err != nil {
		t.Logf("ListInventory (active): ERROR: %v", err)
	} else {
		t.Logf("TotalCount=%d", respActive.Meta.TotalCount)
	}

	// 5. If there are more than 100 total, paginate
	if respAll.Meta.TotalCount > 100 {
		t.Log("")
		t.Logf("--- Query 5: Page 2 (items 101+) ---")
		respPage2, err := c.ListInventory(ctx, dh.InventoryFilters{PerPage: 100, Page: 2})
		if err != nil {
			t.Logf("ListInventory (page 2): ERROR: %v", err)
		} else {
			for _, item := range respPage2.Items {
				statusCounts[item.Status]++
				t.Logf("  inv_id=%d  cert=%s  card=%q  status=%s",
					item.DHInventoryID, item.CertNumber, item.CardName, item.Status)
			}
		}
	}

	t.Log("")
	t.Log("=== SUMMARY ===")
	t.Logf("API reports TotalCount=%d", respAll.Meta.TotalCount)
	t.Logf("Actually returned %d items", len(respAll.Items))
	t.Logf("Status breakdown: %v", statusCounts)

	if len(respAll.Items) == 0 && respAll.Meta.TotalCount > 0 {
		t.Log("")
		t.Log("*** DISCREPANCY: API says TotalCount > 0 but returned 0 items ***")
		t.Log("This means the DH API is reporting phantom inventory.")
	}

	if len(respAll.Items) > 0 {
		t.Log("")
		t.Log("Items DO exist in DH inventory. If they don't show in DH's dashboard,")
		t.Log("check: (1) same account? (2) items need channel sync? (3) DH dashboard filter?")
	}
}
