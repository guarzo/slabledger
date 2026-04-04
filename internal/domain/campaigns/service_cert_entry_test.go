package campaigns

import (
	"context"
	"fmt"
	"testing"
)

type mockCertLookup struct {
	lookupFn func(ctx context.Context, certNumber string) (*CertInfo, error)
}

func (m *mockCertLookup) LookupCert(ctx context.Context, certNumber string) (*CertInfo, error) {
	return m.lookupFn(ctx, certNumber)
}

func TestImportCerts_NewCert(t *testing.T) {
	repo := newMockRepo()
	repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID, Name: ExternalCampaignName}

	certLookup := &mockCertLookup{
		lookupFn: func(_ context.Context, cert string) (*CertInfo, error) {
			return &CertInfo{
				CertNumber: cert, CardName: "Charizard", Grade: 8.0,
				Year: "1999", Category: "BASE SET", CardNumber: "4", Population: 500,
			}, nil
		},
	}

	svc := &service{repo: repo, certLookup: certLookup, idGen: func() string { return "test-id" }}

	result, err := svc.ImportCerts(context.Background(), []string{"12345678"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Imported != 1 {
		t.Errorf("imported = %d, want 1", result.Imported)
	}
	created := repo.purchases["test-id"]
	if created == nil {
		t.Fatal("purchase was not created")
	}
	if created.CertNumber != "12345678" {
		t.Errorf("certNumber = %q, want 12345678", created.CertNumber)
	}
	if created.CardYear != "1999" {
		t.Errorf("cardYear = %q, want 1999", created.CardYear)
	}
	if created.CampaignID != ExternalCampaignID {
		t.Errorf("campaignID = %q, want %q", created.CampaignID, ExternalCampaignID)
	}
	if created.EbayExportFlaggedAt == nil {
		t.Error("expected ebay export flag to be set")
	}
}

func TestImportCerts_ExistingCert(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["existing-id"] = &Purchase{ID: "existing-id", CertNumber: "12345678", Grader: "PSA"}
	repo.certNumbers["12345678"] = true

	svc := &service{repo: repo, idGen: func() string { return "test-id" }}
	result, err := svc.ImportCerts(context.Background(), []string{"12345678"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyExisted != 1 {
		t.Errorf("alreadyExisted = %d, want 1", result.AlreadyExisted)
	}
	if repo.purchases["existing-id"].EbayExportFlaggedAt == nil {
		t.Error("expected ebay export flag to be set")
	}
}

func TestImportCerts_Deduplication(t *testing.T) {
	repo := newMockRepo()
	repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID}

	lookupCount := 0
	idCounter := 0
	certLookup := &mockCertLookup{
		lookupFn: func(_ context.Context, _ string) (*CertInfo, error) {
			lookupCount++
			return &CertInfo{CertNumber: "111", CardName: "Test", Grade: 9}, nil
		},
	}

	svc := &service{
		repo: repo, certLookup: certLookup,
		idGen: func() string { idCounter++; return fmt.Sprintf("id-%d", idCounter) },
	}
	result, _ := svc.ImportCerts(context.Background(), []string{"111", "111", " 111 ", ""})
	if result.Imported != 1 {
		t.Errorf("imported = %d, want 1 (duplicates removed)", result.Imported)
	}
	if lookupCount != 1 {
		t.Errorf("lookup called %d times, want 1", lookupCount)
	}
}

func TestImportCerts_SoldCert(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["sold-id"] = &Purchase{
		ID: "sold-id", CertNumber: "99999999", Grader: "PSA",
		CardName: "Pikachu", CampaignID: "camp-1",
	}
	repo.certNumbers["99999999"] = true
	repo.sales["sale-1"] = &Sale{ID: "sale-1", PurchaseID: "sold-id"}
	repo.purchaseSales["sold-id"] = true

	svc := &service{repo: repo, idGen: func() string { return "test-id" }}
	result, err := svc.ImportCerts(context.Background(), []string{"99999999"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SoldExisting != 1 {
		t.Errorf("soldExisting = %d, want 1", result.SoldExisting)
	}
	if result.AlreadyExisted != 0 {
		t.Errorf("alreadyExisted = %d, want 0", result.AlreadyExisted)
	}
	if len(result.SoldItems) != 1 {
		t.Fatalf("soldItems length = %d, want 1", len(result.SoldItems))
	}
	item := result.SoldItems[0]
	if item.CertNumber != "99999999" {
		t.Errorf("certNumber = %q, want 99999999", item.CertNumber)
	}
	if item.PurchaseID != "sold-id" {
		t.Errorf("purchaseID = %q, want sold-id", item.PurchaseID)
	}
	if item.CardName != "Pikachu" {
		t.Errorf("cardName = %q, want Pikachu", item.CardName)
	}
	if item.CampaignID != "camp-1" {
		t.Errorf("campaignID = %q, want camp-1", item.CampaignID)
	}
	if repo.purchases["sold-id"].EbayExportFlaggedAt != nil {
		t.Error("ebay export flag should not be set on sold item")
	}
}

func TestImportCerts_MixedSoldAndUnsold(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["unsold-id"] = &Purchase{
		ID: "unsold-id", CertNumber: "11111111", Grader: "PSA",
		CardName: "Charizard", CampaignID: "camp-1",
	}
	repo.certNumbers["11111111"] = true

	repo.purchases["sold-id"] = &Purchase{
		ID: "sold-id", CertNumber: "22222222", Grader: "PSA",
		CardName: "Blastoise", CampaignID: "camp-1",
	}
	repo.certNumbers["22222222"] = true
	repo.sales["sale-1"] = &Sale{ID: "sale-1", PurchaseID: "sold-id"}
	repo.purchaseSales["sold-id"] = true

	svc := &service{repo: repo, idGen: func() string { return "test-id" }}
	result, err := svc.ImportCerts(context.Background(), []string{"11111111", "22222222"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyExisted != 1 {
		t.Errorf("alreadyExisted = %d, want 1", result.AlreadyExisted)
	}
	if result.SoldExisting != 1 {
		t.Errorf("soldExisting = %d, want 1", result.SoldExisting)
	}
}
