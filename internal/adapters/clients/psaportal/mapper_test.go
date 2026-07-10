package psaportal

import (
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestMapRow(t *testing.T) {
	tests := []struct {
		name    string
		in      map[string]string
		wantErr bool
		check   func(t *testing.T, row inventory.PSAExportRow)
	}{
		{
			name: "full row",
			in: map[string]string{
				colCert:       "12345678",
				colTitle:      "2023 Pokemon Charizard #4",
				colGrade:      "10",
				colPricePaid:  "$1,250.00",
				colSource:     "PSA Vault",
				colDate:       "2026-06-01",
				colShipDate:   "2026-06-03",
				colRefunded:   "false",
				colCategory:   "Pokemon",
				colFrontImage: "https://img/f.jpg",
				colBackImage:  "https://img/b.jpg",
			},
			check: func(t *testing.T, row inventory.PSAExportRow) {
				if row.CertNumber != "12345678" {
					t.Errorf("cert: %q", row.CertNumber)
				}
				if row.ListingTitle != "2023 Pokemon Charizard #4" {
					t.Errorf("title: %q", row.ListingTitle)
				}
				if row.Grade != 10 {
					t.Errorf("grade: %v", row.Grade)
				}
				if row.PricePaid != 1250.00 {
					t.Errorf("price: %v", row.PricePaid)
				}
				if row.PurchaseSource != "PSA Vault" {
					t.Errorf("source: %q", row.PurchaseSource)
				}
				if row.ShipDate != "2026-06-03" {
					t.Errorf("ship: %q", row.ShipDate)
				}
				if row.WasRefunded {
					t.Error("expected not refunded")
				}
			},
		},
		{
			name: "half grade and refund true",
			in: map[string]string{
				colCert:     "999",
				colGrade:    "8.5",
				colRefunded: "true",
			},
			check: func(t *testing.T, row inventory.PSAExportRow) {
				if row.Grade != 8.5 {
					t.Errorf("grade: %v", row.Grade)
				}
				if !row.WasRefunded {
					t.Error("expected refunded")
				}
			},
		},
		{
			name:    "bad price",
			in:      map[string]string{colCert: "1", colPricePaid: "not-a-price"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, err := mapRow(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.check != nil {
				tt.check(t, row)
			}
		})
	}
}
