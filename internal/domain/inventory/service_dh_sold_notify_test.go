package inventory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// Recording a sale locally must also retire the item on DH. Without this, a
// card sold off-platform (card show, local buyer) stays live on the DH
// marketplace and can be sold twice — the SLA-109 production incident, where
// DH reported 89 listed against 57 locally and all 32 extras were in-person
// sales that were never delisted.
func newSoldNotifyFixture(t *testing.T, dhInventoryID int) (
	*mocks.InMemoryCampaignStore, *mocks.DHSoldNotifierMock,
	inventory.Service, *inventory.Campaign, *inventory.Purchase,
) {
	t.Helper()
	repo := mocks.NewInMemoryCampaignStore()
	notifier := &mocks.DHSoldNotifierMock{}
	svc := inventory.NewService(repo, repo, repo, repo, repo, repo, repo,
		withTestIDGen(), inventory.WithDHSoldNotifier(notifier))
	ctx := context.Background()

	c := &inventory.Campaign{Name: "Test", BuyTermsCLPct: 0.78}
	if err := svc.CreateCampaign(ctx, c); err != nil {
		t.Fatalf("setup campaign: %v", err)
	}
	p := &inventory.Purchase{
		CampaignID: c.ID, CardName: "Charizard", CertNumber: "SOLD0001",
		GradeValue: 9, BuyCostCents: 50000, PurchaseDate: "2026-06-10",
		DHInventoryID: dhInventoryID, DHStatus: inventory.DHStatusListed,
	}
	if err := svc.CreatePurchase(ctx, p); err != nil {
		t.Fatalf("setup purchase: %v", err)
	}
	return repo, notifier, svc, c, p
}

func TestService_CreateSale_NotifiesDHSold(t *testing.T) {
	tests := []struct {
		name          string
		dhInventoryID int
		notifyErr     error
		wantMarked    []int
	}{
		{name: "listed on DH is retired", dhInventoryID: 4242, wantMarked: []int{4242}},
		{name: "never pushed to DH is skipped", dhInventoryID: 0, wantMarked: nil},
		{
			name:          "DH failure does not fail the sale",
			dhInventoryID: 4242,
			notifyErr:     errors.New("dh unavailable"),
			wantMarked:    []int{4242},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, notifier, svc, c, p := newSoldNotifyFixture(t, tt.dhInventoryID)
			notifier.MarkInventorySoldFn = func(context.Context, int) error { return tt.notifyErr }

			sale := &inventory.Sale{
				PurchaseID:     p.ID,
				SaleChannel:    inventory.SaleChannelInPerson,
				SalePriceCents: 60000,
				SaleDate:       "2026-07-10",
			}
			// Best-effort: a DH outage must never block recording the sale.
			if err := svc.CreateSale(context.Background(), sale, c, p); err != nil {
				t.Fatalf("CreateSale: %v", err)
			}

			got := notifier.MarkedSold()
			if len(got) != len(tt.wantMarked) {
				t.Fatalf("MarkedSold() = %v, want %v", got, tt.wantMarked)
			}
			for i := range got {
				if got[i] != tt.wantMarked[i] {
					t.Fatalf("MarkedSold() = %v, want %v", got, tt.wantMarked)
				}
			}
		})
	}
}

// CreateBulkSales inlines its own sale creation rather than delegating to
// CreateSale, so it needs its own coverage — the card-show flow lands here.
func TestService_CreateBulkSales_NotifiesDHSold(t *testing.T) {
	_, notifier, svc, c, p := newSoldNotifyFixture(t, 7777)

	result, err := svc.CreateBulkSales(context.Background(), c.ID,
		inventory.SaleChannelInPerson, "2026-07-10",
		[]inventory.BulkSaleInput{{PurchaseID: p.ID, SalePriceCents: 60000}})
	if err != nil {
		t.Fatalf("CreateBulkSales: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("Created = %d, want 1 (errors: %+v)", result.Created, result.Errors)
	}

	got := notifier.MarkedSold()
	if len(got) != 1 || got[0] != 7777 {
		t.Fatalf("MarkedSold() = %v, want [7777]", got)
	}
}
