package psaportal

import "testing"

func TestMapRow(t *testing.T) {
	in := map[string]string{
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
	}
	row, err := mapRow(in)
	if err != nil {
		t.Fatal(err)
	}
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
}

func TestMapRow_HalfGradeAndRefundTrue(t *testing.T) {
	row, err := mapRow(map[string]string{
		colCert:     "999",
		colGrade:    "8.5",
		colRefunded: "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Grade != 8.5 {
		t.Errorf("grade: %v", row.Grade)
	}
	if !row.WasRefunded {
		t.Error("expected refunded")
	}
}

func TestMapRow_BadPrice(t *testing.T) {
	_, err := mapRow(map[string]string{colCert: "1", colPricePaid: "not-a-price"})
	if err == nil {
		t.Fatal("expected error on bad price")
	}
}
