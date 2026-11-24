package inventory

import (
	"context"
	"errors"
	"testing"
)

func TestGetSaleStatusByCert(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{ID: "p1", CertNumber: "11111111", Grader: "PSA", CardName: "Charizard"}
	repo.purchases["p2"] = &Purchase{ID: "p2", CertNumber: "22222222", Grader: "PSA", CardName: "Blastoise"}
	repo.sales["p2"] = &Sale{ID: "s2", PurchaseID: "p2", SalePriceCents: 5000}
	svc := &service{purchases: repo, sales: repo}

	tests := []struct {
		name     string
		cert     string
		wantErr  error
		wantSale string
	}{
		{"unsold", "11111111", nil, ""},
		{"sold", "22222222", nil, "s2"},
		{"not found", "99999999", ErrPurchaseNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.GetSaleStatusByCert(context.Background(), tt.cert)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			switch {
			case tt.wantSale == "" && got.Sale != nil:
				t.Fatalf("sale = %+v, want nil", got.Sale)
			case tt.wantSale != "" && (got.Sale == nil || got.Sale.ID != tt.wantSale):
				t.Fatalf("sale = %+v, want %s", got.Sale, tt.wantSale)
			}
		})
	}
}

func TestDeleteExpectedSale(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		wantErr  error
		wantGone bool
	}{
		{"matching sale is deleted", "s1", nil, true},
		{"different sale is kept", "s-old", ErrReturnConflict, false},
		{"no expectation deletes current sale", "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.purchases["p1"] = &Purchase{ID: "p1", CertNumber: "11111111", Grader: "PSA"}
			repo.sales["p1"] = &Sale{ID: "s1", PurchaseID: "p1", SalePriceCents: 5000}
			svc := &service{purchases: repo, sales: repo}

			err := svc.DeleteExpectedSale(context.Background(), "p1", tt.expected)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if _, kept := repo.sales["p1"]; kept == tt.wantGone {
				t.Fatalf("sale kept = %v, want gone = %v", kept, tt.wantGone)
			}
		})
	}
}
