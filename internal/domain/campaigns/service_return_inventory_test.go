package campaigns

import (
	"context"
	"testing"
)

func TestDeleteSaleByPurchaseID_Success(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{ID: "p1", CampaignID: "c1", CertNumber: "111", Grader: "PSA"}
	repo.sales["s1"] = &Sale{ID: "s1", PurchaseID: "p1"}
	repo.purchaseSales["p1"] = true

	svc := &service{repo: repo, idGen: func() string { return "test-id" }}

	if err := svc.DeleteSaleByPurchaseID(context.Background(), "p1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := repo.sales["s1"]; ok {
		t.Error("sale should have been deleted")
	}
	if repo.purchaseSales["p1"] {
		t.Error("purchaseSales flag should have been cleared")
	}
	if _, ok := repo.purchases["p1"]; !ok {
		t.Error("purchase should still exist after sale deletion")
	}
}

func TestDeleteSaleByPurchaseID_NoSale(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{ID: "p1", CampaignID: "c1", CertNumber: "111", Grader: "PSA"}

	svc := &service{repo: repo, idGen: func() string { return "test-id" }}

	err := svc.DeleteSaleByPurchaseID(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsSaleNotFound(err) {
		t.Errorf("expected ErrSaleNotFound, got %v", err)
	}
}
