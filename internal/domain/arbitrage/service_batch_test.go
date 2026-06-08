package arbitrage

import (
	"context"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestGetAcquisitionTargets_BatchPath(t *testing.T) {
	campaignID := "camp-batch-acq"
	campaign := &inventory.Campaign{
		ID:         campaignID,
		Name:       "Test Campaign",
		EbayFeePct: 0.1235,
	}

	purchase := inventory.Purchase{
		ID:         "p-acq-batch",
		CampaignID: campaignID,
		CardName:   "Pikachu",
		SetName:    "Base Set",
		CardNumber: "58",
		CertNumber: "cert-acq",
		GradeValue: 9.0,
	}

	batchPricer := &stubBatchPricer{
		cardIDs: map[string]int{"Pikachu|Base Set|58": 99},
		distributions: map[int]GradedDistribution{
			99: {ByGrade: map[string]PriceBucket{
				"raw":    {MedianCents: 5000, SampleSize: 10},
				"psa_8":  {MedianCents: 12000, SampleSize: 5},
				"psa_9":  {MedianCents: 25000, SampleSize: 8},
				"psa_10": {MedianCents: 80000, SampleSize: 3},
			}},
		},
	}

	svc := NewService(
		&stubCampaignRepo{campaign: campaign},
		&stubPurchaseRepo{unsold: []inventory.Purchase{purchase}},
		&stubAnalyticsRepo{},
		&stubFinanceRepo{},
		WithBatchPricer(batchPricer),
	)

	results, err := svc.GetAcquisitionTargets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].CardName != "Pikachu" {
		t.Errorf("expected Pikachu, got %s", results[0].CardName)
	}
	if results[0].ProfitCents <= 0 {
		t.Errorf("expected positive profit, got %d", results[0].ProfitCents)
	}
}
