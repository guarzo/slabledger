package inventory

import (
	"context"
	"errors"
	"testing"
)

func TestDeleteSaleByPurchaseID(t *testing.T) {
	tests := []struct {
		name         string
		seed         func(*mockRepo)
		wantErr      bool
		wantSaleGone bool
	}{
		{
			name: "success deletes sale and clears flag",
			seed: func(r *mockRepo) {
				r.purchases["p1"] = &Purchase{ID: "p1", CampaignID: "c1", CertNumber: "111", Grader: "PSA"}
				r.sales["s1"] = &Sale{ID: "s1", PurchaseID: "p1"}
				r.purchaseSales["p1"] = true
			},
			wantErr:      false,
			wantSaleGone: true,
		},
		{
			name: "no sale returns ErrSaleNotFound",
			seed: func(r *mockRepo) {
				r.purchases["p1"] = &Purchase{ID: "p1", CampaignID: "c1", CertNumber: "111", Grader: "PSA"}
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			tc.seed(repo)
			svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, idGen: func() string { return "test-id" }}

			err := svc.DeleteSaleByPurchaseID(context.Background(), "p1")

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, ErrSaleNotFound) {
					t.Errorf("expected ErrSaleNotFound, got %v", err)
				}
				return
			}

			if err != nil {
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
		})
	}
}
