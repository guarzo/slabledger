package campaigns

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestListEbayExportItems_FlaggedOnly(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	repo.campaigns["c1"] = &Campaign{ID: "c1", Phase: PhaseActive}
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CampaignID: "c1", CertNumber: "111", CardName: "Charizard", SetName: "Base Set",
		CardNumber: "4", CardYear: "1999", GradeValue: 8, Grader: "PSA",
		CLValueCents: 25000, EbayExportFlaggedAt: &now,
		MarketSnapshotData: MarketSnapshotData{MedianCents: 27500},
	}

	svc := &service{repo: repo, idGen: func() string { return "id" }}
	resp, err := svc.ListEbayExportItems(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].SuggestedPriceCents != 25000 {
		t.Errorf("suggestedPrice = %d, want 25000", resp.Items[0].SuggestedPriceCents)
	}
}

func TestGenerateEbayCSV_Success(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "12345678", CardName: "Charizard",
		SetName: "Base Set", CardNumber: "4", CardYear: "1999",
		GradeValue: 8, Grader: "PSA",
		FrontImageURL: "https://example.com/front.jpg",
		BackImageURL:  "https://example.com/back.jpg",
	}

	svc := &service{repo: repo, idGen: func() string { return "id" }}
	csvBytes, err := svc.GenerateEbayCSV(context.Background(), []EbayExportGenerateItem{
		{PurchaseID: "p1", PriceCents: 25000},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content := string(csvBytes)
	if !strings.Contains(content, "Info,Version=1.0.0") {
		t.Error("missing metadata line")
	}
	if !strings.Contains(content, "PSA-12345678") {
		t.Error("missing CustomLabel")
	}
	if !strings.Contains(content, "250.00") {
		t.Error("missing StartPrice")
	}
}

func TestListEbayExportItems_ExcludesNonPSA(t *testing.T) {
	now := time.Now()
	repo := newMockRepo()
	repo.campaigns["c1"] = &Campaign{ID: "c1", Phase: PhaseActive}
	repo.purchases["psa1"] = &Purchase{
		ID: "psa1", CampaignID: "c1", CertNumber: "111", CardName: "Charizard", SetName: "Base Set",
		CardNumber: "4", GradeValue: 8, Grader: "PSA",
		CLValueCents: 25000, EbayExportFlaggedAt: &now,
	}
	repo.purchases["cgc1"] = &Purchase{
		ID: "cgc1", CampaignID: "c1", CertNumber: "222", CardName: "Pikachu", SetName: "Base Set",
		CardNumber: "58", GradeValue: 9, Grader: "CGC",
		CLValueCents: 10000, EbayExportFlaggedAt: &now,
	}

	svc := &service{repo: repo, idGen: func() string { return "id" }}
	resp, err := svc.ListEbayExportItems(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1 (only PSA)", len(resp.Items))
	}
	if resp.Items[0].CertNumber != "111" {
		t.Errorf("expected PSA cert 111, got %s", resp.Items[0].CertNumber)
	}
}

func TestGenerateEbayCSV_RejectsZeroPrice(t *testing.T) {
	svc := &service{idGen: func() string { return "id" }}
	_, err := svc.GenerateEbayCSV(context.Background(), []EbayExportGenerateItem{
		{PurchaseID: "p1", PriceCents: 0},
	})
	if err == nil {
		t.Fatal("expected error for zero price")
	}
}
