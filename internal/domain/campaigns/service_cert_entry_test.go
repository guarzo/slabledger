package campaigns

import (
	"context"
	"errors"
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

type expectedSoldItem struct {
	CertNumber string
	PurchaseID string
	CardName   string
	CampaignID string
}

func TestImportCerts_SoldCerts(t *testing.T) {
	tests := []struct {
		name          string
		seed          func(*mockRepo)
		input         []string
		wantAlready   int
		wantSold      int
		wantSoldItems []expectedSoldItem
	}{
		{
			name: "single sold cert",
			seed: func(r *mockRepo) {
				r.purchases["sold-id"] = &Purchase{
					ID: "sold-id", CertNumber: "99999999", Grader: "PSA",
					CardName: "Pikachu", CampaignID: "camp-1",
				}
				r.certNumbers["99999999"] = true
				r.sales["sale-1"] = &Sale{ID: "sale-1", PurchaseID: "sold-id"}
				r.purchaseSales["sold-id"] = true
			},
			input:       []string{"99999999"},
			wantAlready: 0,
			wantSold:    1,
			wantSoldItems: []expectedSoldItem{
				{CertNumber: "99999999", PurchaseID: "sold-id", CardName: "Pikachu", CampaignID: "camp-1"},
			},
		},
		{
			name: "mixed sold and unsold",
			seed: func(r *mockRepo) {
				r.purchases["unsold-id"] = &Purchase{
					ID: "unsold-id", CertNumber: "11111111", Grader: "PSA",
					CardName: "Charizard", CampaignID: "camp-1",
				}
				r.certNumbers["11111111"] = true

				r.purchases["sold-id"] = &Purchase{
					ID: "sold-id", CertNumber: "22222222", Grader: "PSA",
					CardName: "Blastoise", CampaignID: "camp-1",
				}
				r.certNumbers["22222222"] = true
				r.sales["sale-1"] = &Sale{ID: "sale-1", PurchaseID: "sold-id"}
				r.purchaseSales["sold-id"] = true
			},
			input:       []string{"11111111", "22222222"},
			wantAlready: 1,
			wantSold:    1,
			wantSoldItems: []expectedSoldItem{
				{CertNumber: "22222222", PurchaseID: "sold-id", CardName: "Blastoise", CampaignID: "camp-1"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			tc.seed(repo)
			svc := &service{repo: repo, idGen: func() string { return "test-id" }}

			result, err := svc.ImportCerts(context.Background(), tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.AlreadyExisted != tc.wantAlready {
				t.Errorf("alreadyExisted = %d, want %d", result.AlreadyExisted, tc.wantAlready)
			}
			if result.SoldExisting != tc.wantSold {
				t.Errorf("soldExisting = %d, want %d", result.SoldExisting, tc.wantSold)
			}
			if len(result.SoldItems) != len(tc.wantSoldItems) {
				t.Fatalf("soldItems length = %d, want %d", len(result.SoldItems), len(tc.wantSoldItems))
			}
			for i, want := range tc.wantSoldItems {
				got := result.SoldItems[i]
				if got.CertNumber != want.CertNumber {
					t.Errorf("soldItems[%d].certNumber = %q, want %q", i, got.CertNumber, want.CertNumber)
				}
				if got.PurchaseID != want.PurchaseID {
					t.Errorf("soldItems[%d].purchaseID = %q, want %q", i, got.PurchaseID, want.PurchaseID)
				}
				if got.CardName != want.CardName {
					t.Errorf("soldItems[%d].cardName = %q, want %q", i, got.CardName, want.CardName)
				}
				if got.CampaignID != want.CampaignID {
					t.Errorf("soldItems[%d].campaignID = %q, want %q", i, got.CampaignID, want.CampaignID)
				}
			}
		})
	}
}

func TestScanCert(t *testing.T) {
	tests := []struct {
		name       string
		seed       func(*mockRepo)
		certNumber string
		wantStatus string
		wantCard   string
	}{
		{
			name: "existing cert not sold",
			seed: func(r *mockRepo) {
				r.purchases["p1"] = &Purchase{
					ID: "p1", CertNumber: "11111111", Grader: "PSA",
					CardName: "Charizard", CampaignID: "camp-1",
				}
			},
			certNumber: "11111111",
			wantStatus: "existing",
			wantCard:   "Charizard",
		},
		{
			name: "sold cert",
			seed: func(r *mockRepo) {
				r.purchases["p1"] = &Purchase{
					ID: "p1", CertNumber: "22222222", Grader: "PSA",
					CardName: "Pikachu", CampaignID: "camp-1",
				}
				r.sales["s1"] = &Sale{ID: "s1", PurchaseID: "p1"}
				r.purchaseSales["p1"] = true
			},
			certNumber: "22222222",
			wantStatus: "sold",
			wantCard:   "Pikachu",
		},
		{
			name:       "new cert not in DB",
			seed:       func(_ *mockRepo) {},
			certNumber: "33333333",
			wantStatus: "new",
			wantCard:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			tc.seed(repo)
			svc := &service{repo: repo, idGen: func() string { return "test-id" }}

			result, err := svc.ScanCert(context.Background(), tc.certNumber)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", result.Status, tc.wantStatus)
			}
			if result.CardName != tc.wantCard {
				t.Errorf("cardName = %q, want %q", result.CardName, tc.wantCard)
			}
		})
	}
}

func TestScanCert_ExistingSetsExportFlag(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName: "Charizard", CampaignID: "camp-1",
	}

	svc := &service{repo: repo, idGen: func() string { return "test-id" }}

	_, err := svc.ScanCert(context.Background(), "11111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.purchases["p1"].EbayExportFlaggedAt == nil {
		t.Error("expected ebay export flag to be set for existing cert")
	}
}

func TestImportCerts_NewCertSetsReceivedAt(t *testing.T) {
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
	if created.ReceivedAt == nil {
		t.Error("expected ReceivedAt to be set for new cert")
	}
}

func TestImportCerts_ExistingCertSetsReceivedAt(t *testing.T) {
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
	if repo.purchases["existing-id"].ReceivedAt == nil {
		t.Error("expected ReceivedAt to be set for existing cert")
	}
}

func TestResolveCert(t *testing.T) {
	tests := []struct {
		name         string
		certNumber   string
		lookupFn     func(ctx context.Context, certNumber string) (*CertInfo, error)
		wantErr      bool
		wantSentinel error
		wantName     string
	}{
		{
			name:       "successful lookup",
			certNumber: "44444444",
			lookupFn: func(_ context.Context, cert string) (*CertInfo, error) {
				return &CertInfo{
					CertNumber: cert, CardName: "Umbreon VMAX", Grade: 10,
					Year: "2022", Category: "EVOLVING SKIES", Subject: "2022 Pokemon Evolving Skies Umbreon VMAX",
				}, nil
			},
			wantErr:  false,
			wantName: "Umbreon VMAX",
		},
		{
			name:       "api returns error",
			certNumber: "00000000",
			lookupFn: func(_ context.Context, _ string) (*CertInfo, error) {
				return nil, fmt.Errorf("cert 00000000 not found")
			},
			wantErr: true,
		},
		{
			name:       "api returns nil info",
			certNumber: "00000001",
			lookupFn: func(_ context.Context, _ string) (*CertInfo, error) {
				return nil, nil
			},
			wantErr:      true,
			wantSentinel: ErrCertNotFound,
		},
		{
			name:       "no cert lookup configured",
			certNumber: "55555555",
			lookupFn:   nil,
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			var certLookup CertLookup
			if tc.lookupFn != nil {
				certLookup = &mockCertLookup{lookupFn: tc.lookupFn}
			}
			svc := &service{repo: repo, certLookup: certLookup, idGen: func() string { return "test-id" }}

			info, err := svc.ResolveCert(context.Background(), tc.certNumber)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantSentinel != nil && !errors.Is(err, tc.wantSentinel) {
					t.Errorf("expected error %v, got %v", tc.wantSentinel, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.CardName != tc.wantName {
				t.Errorf("cardName = %q, want %q", info.CardName, tc.wantName)
			}
		})
	}
}
