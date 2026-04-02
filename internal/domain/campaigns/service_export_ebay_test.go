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

func TestListEbayExportItems_IncludesNewPriceFields(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name                string
		buyCostCents        int
		psaSourcingFeeCents int
		lastSoldCents       int
		reviewedPriceCents  int
		reviewedAt          string
		wantCostBasis       int
		wantLastSold        int
		wantReviewedPrice   int
		wantReviewedAt      string
	}{
		{
			name:                "all fields populated",
			buyCostCents:        10000,
			psaSourcingFeeCents: 300,
			lastSoldCents:       26000,
			reviewedPriceCents:  24000,
			reviewedAt:          "2026-03-30T10:00:00Z",
			wantCostBasis:       10300,
			wantLastSold:        26000,
			wantReviewedPrice:   24000,
			wantReviewedAt:      "2026-03-30T10:00:00Z",
		},
		{
			name:                "zero sourcing fee",
			buyCostCents:        15000,
			psaSourcingFeeCents: 0,
			lastSoldCents:       18000,
			reviewedPriceCents:  0,
			reviewedAt:          "",
			wantCostBasis:       15000,
			wantLastSold:        18000,
			wantReviewedPrice:   0,
			wantReviewedAt:      "",
		},
		{
			name:                "no market data",
			buyCostCents:        5000,
			psaSourcingFeeCents: 200,
			lastSoldCents:       0,
			reviewedPriceCents:  6000,
			reviewedAt:          "2026-03-29T08:00:00Z",
			wantCostBasis:       5200,
			wantLastSold:        0,
			wantReviewedPrice:   6000,
			wantReviewedAt:      "2026-03-29T08:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.campaigns["c1"] = &Campaign{ID: "c1", Phase: PhaseActive}
			repo.purchases["p1"] = &Purchase{
				ID: "p1", CampaignID: "c1", CertNumber: "111",
				CardName: "Charizard", SetName: "Base Set",
				CardNumber: "4", CardYear: "1999",
				GradeValue: 8, Grader: "PSA",
				CLValueCents:        25000,
				BuyCostCents:        tt.buyCostCents,
				PSASourcingFeeCents: tt.psaSourcingFeeCents,
				ReviewedPriceCents:  tt.reviewedPriceCents,
				ReviewedAt:          tt.reviewedAt,
				EbayExportFlaggedAt: &now,
				MarketSnapshotData: MarketSnapshotData{
					MedianCents:   27500,
					LastSoldCents: tt.lastSoldCents,
				},
			}

			svc := &service{repo: repo, idGen: func() string { return "id" }}
			resp, err := svc.ListEbayExportItems(context.Background(), true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resp.Items) != 1 {
				t.Fatalf("items = %d, want 1", len(resp.Items))
			}
			item := resp.Items[0]
			if item.CostBasisCents != tt.wantCostBasis {
				t.Errorf("CostBasisCents = %d, want %d", item.CostBasisCents, tt.wantCostBasis)
			}
			if item.LastSoldCents != tt.wantLastSold {
				t.Errorf("LastSoldCents = %d, want %d", item.LastSoldCents, tt.wantLastSold)
			}
			if item.ReviewedPriceCents != tt.wantReviewedPrice {
				t.Errorf("ReviewedPriceCents = %d, want %d", item.ReviewedPriceCents, tt.wantReviewedPrice)
			}
			if item.ReviewedAt != tt.wantReviewedAt {
				t.Errorf("ReviewedAt = %q, want %q", item.ReviewedAt, tt.wantReviewedAt)
			}
		})
	}
}
