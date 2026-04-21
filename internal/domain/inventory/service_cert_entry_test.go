package inventory

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/dhevents"
)

// dbLikePurchaseRepo wraps mockRepo so Get* methods return a shallow copy of
// the stored Purchase. This mirrors the SQLite adapter's semantics: a later
// SetReceivedAt writes through to the DB but does NOT mutate the caller's
// in-memory struct. Used to guard against mock-vs-real divergence where the
// shared-pointer mock accidentally masks bugs that require the service to
// update its local copy after a DB write.
type dbLikePurchaseRepo struct {
	*mockRepo
}

func (d *dbLikePurchaseRepo) GetPurchaseByCertNumber(ctx context.Context, grader, certNumber string) (*Purchase, error) {
	p, err := d.mockRepo.GetPurchaseByCertNumber(ctx, grader, certNumber)
	if err != nil || p == nil {
		return p, err
	}
	cp := *p
	return &cp, nil
}

func (d *dbLikePurchaseRepo) GetPurchasesByGraderAndCertNumbers(ctx context.Context, grader string, certNumbers []string) (map[string]*Purchase, error) {
	src, err := d.mockRepo.GetPurchasesByGraderAndCertNumbers(ctx, grader, certNumbers)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*Purchase, len(src))
	for k, p := range src {
		if p == nil {
			out[k] = nil
			continue
		}
		cp := *p
		out[k] = &cp
	}
	return out, nil
}

type mockCertLookup struct {
	lookupFn       func(ctx context.Context, certNumber string) (*CertInfo, error)
	lookupImagesFn func(ctx context.Context, certNumber string) (string, string, error)
}

func (m *mockCertLookup) LookupCert(ctx context.Context, certNumber string) (*CertInfo, error) {
	return m.lookupFn(ctx, certNumber)
}

func (m *mockCertLookup) LookupImages(ctx context.Context, certNumber string) (string, string, error) {
	if m.lookupImagesFn != nil {
		return m.lookupImagesFn(ctx, certNumber)
	}
	return "", "", nil
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

	svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, certLookup: certLookup, idGen: func() string { return "test-id" }}

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

// TestImportCerts_GenericPSACategoryStoresEmptySetName guards against PSA's
// catch-all "TCG Cards" / "Pokemon Cards" categories leaking into set_name on
// older certs. Downstream CL enrichment fills the real set name; persisting
// the generic value here pollutes inventory and breaks price lookups.
func TestImportCerts_GenericPSACategoryStoresEmptySetName(t *testing.T) {
	cases := []struct {
		name     string
		category string
	}{
		{name: "TCG Cards", category: "TCG Cards"},
		{name: "Pokemon Cards", category: "Pokemon Cards"},
		{name: "empty category", category: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID, Name: ExternalCampaignName}
			certLookup := &mockCertLookup{
				lookupFn: func(_ context.Context, cert string) (*CertInfo, error) {
					return &CertInfo{
						CertNumber: cert, CardName: "Houndoom", Grade: 9.0,
						Year: "2004", Category: tc.category, CardNumber: "10", Population: 100,
					}, nil
				},
			}
			svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, certLookup: certLookup, idGen: func() string { return "test-id" }}

			if _, err := svc.ImportCerts(context.Background(), []string{"999"}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			created := repo.purchases["test-id"]
			if created == nil {
				t.Fatal("purchase was not created")
			}
			if created.SetName != "" {
				t.Errorf("setName = %q, want empty for generic category %q", created.SetName, tc.category)
			}
		})
	}
}

// TestImportCerts_ConcreteCategoryAdoptsResolvedSetName guards the happy path:
// when PSA returns a real category, the resolved set name is persisted.
func TestImportCerts_ConcreteCategoryAdoptsResolvedSetName(t *testing.T) {
	cases := []struct {
		name         string
		certs        []string
		lookupReturn *CertInfo
	}{
		{
			name:  "BASE SET",
			certs: []string{"42"},
			lookupReturn: &CertInfo{
				CardName: "Charizard", Grade: 10,
				Year: "1999", Category: "BASE SET", CardNumber: "4",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID, Name: ExternalCampaignName}
			certLookup := &mockCertLookup{
				lookupFn: func(_ context.Context, cert string) (*CertInfo, error) {
					out := *tc.lookupReturn
					out.CertNumber = cert
					return &out, nil
				},
			}
			svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, certLookup: certLookup, idGen: func() string { return "test-id" }}

			if _, err := svc.ImportCerts(context.Background(), tc.certs); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			created := repo.purchases["test-id"]
			if created == nil {
				t.Fatal("purchase was not created")
			}
			if created.SetName == "" {
				t.Errorf("setName should be set for non-generic category, got empty")
			}
			if IsGenericSetName(created.SetName) {
				t.Errorf("setName %q is still generic; ResolvePSACategory should have mapped it", created.SetName)
			}
		})
	}
}

func TestImportCerts_ExistingCert(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["existing-id"] = &Purchase{ID: "existing-id", CertNumber: "12345678", Grader: "PSA"}
	repo.certNumbers["12345678"] = true

	svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, idGen: func() string { return "test-id" }}
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
		campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, certLookup: certLookup,
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
			svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, idGen: func() string { return "test-id" }}

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
			svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, idGen: func() string { return "test-id" }}

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

func TestScanCert_ExistingIncludesSearchHelperMetadata(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName:      "Charizard",
		CampaignID:    "camp-1",
		FrontImageURL: "https://example.com/front.jpg",
		SetName:       "Base Set",
		CardNumber:    "4",
		CardYear:      "1999",
		GradeValue:    10,
		Population:    1234,
	}

	svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, idGen: func() string { return "test-id" }}

	result, err := svc.ScanCert(context.Background(), "11111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FrontImageURL != "https://example.com/front.jpg" {
		t.Errorf("FrontImageURL = %q, want the seeded URL", result.FrontImageURL)
	}
	if result.SetName != "Base Set" || result.CardNumber != "4" || result.CardYear != "1999" {
		t.Errorf("set/number/year not propagated: %+v", result)
	}
	if result.GradeValue != 10 || result.Population != 1234 {
		t.Errorf("grade/population not propagated: grade=%v pop=%d", result.GradeValue, result.Population)
	}
}

func TestScanCert_SoldIncludesSearchHelperMetadata(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "22222222", Grader: "PSA",
		CardName:      "Pikachu",
		CampaignID:    "camp-1",
		FrontImageURL: "https://example.com/pika.jpg",
		SetName:       "Jungle",
		CardNumber:    "60",
	}
	repo.sales["s1"] = &Sale{ID: "s1", PurchaseID: "p1"}
	repo.purchaseSales["p1"] = true

	svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, idGen: func() string { return "test-id" }}

	result, err := svc.ScanCert(context.Background(), "22222222")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "sold" {
		t.Fatalf("status = %q, want sold", result.Status)
	}
	if result.FrontImageURL != "https://example.com/pika.jpg" || result.SetName != "Jungle" || result.CardNumber != "60" {
		t.Errorf("metadata not propagated on sold cert: %+v", result)
	}
}

func TestScanCerts_Batch(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName: "Charizard", CampaignID: "camp-1",
	}
	repo.purchases["p2"] = &Purchase{
		ID: "p2", CertNumber: "22222222", Grader: "PSA",
		CardName: "Pikachu", CampaignID: "camp-1",
	}
	repo.sales["s1"] = &Sale{ID: "s1", PurchaseID: "p2"}
	repo.purchaseSales["p2"] = true

	svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, idGen: func() string { return "test-id" }}

	out, err := svc.ScanCerts(context.Background(), []string{"11111111", "22222222", "33333333", "", "11111111"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 3 {
		t.Fatalf("expected 3 results (one per unique non-empty cert), got %d: %+v", len(out.Results), out.Results)
	}
	if out.Results["11111111"].Status != "existing" {
		t.Errorf("11111111 status = %q, want existing", out.Results["11111111"].Status)
	}
	if out.Results["22222222"].Status != "sold" {
		t.Errorf("22222222 status = %q, want sold", out.Results["22222222"].Status)
	}
	if out.Results["33333333"].Status != "new" {
		t.Errorf("33333333 status = %q, want new", out.Results["33333333"].Status)
	}
	if len(out.Errors) != 0 {
		t.Errorf("unexpected errors: %+v", out.Errors)
	}
}

func TestScanCert_ExistingSetsExportFlag(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName: "Charizard", CampaignID: "camp-1",
	}

	svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, idGen: func() string { return "test-id" }}

	_, err := svc.ScanCert(context.Background(), "11111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.purchases["p1"].EbayExportFlaggedAt == nil {
		t.Error("expected ebay export flag to be set for existing cert")
	}
}

func TestImportCerts_SetsReceivedAt(t *testing.T) {
	tests := []struct {
		name            string
		seedPurchases   map[string]*Purchase
		seedCertNumbers map[string]bool
		certLookup      *mockCertLookup
		idGen           func() string
		wantImported    int
		wantExisted     int
		wantPurchaseID  string
	}{
		{
			name:            "NewCertSetsReceivedAt",
			seedPurchases:   map[string]*Purchase{},
			seedCertNumbers: map[string]bool{},
			certLookup: &mockCertLookup{
				lookupFn: func(_ context.Context, cert string) (*CertInfo, error) {
					return &CertInfo{
						CertNumber: cert, CardName: "Charizard", Grade: 8.0,
						Year: "1999", Category: "BASE SET", CardNumber: "4", Population: 500,
					}, nil
				},
			},
			idGen:          func() string { return "test-id" },
			wantImported:   1,
			wantExisted:    0,
			wantPurchaseID: "test-id",
		},
		{
			name: "ExistingCertSetsReceivedAt",
			seedPurchases: map[string]*Purchase{
				"existing-id": {ID: "existing-id", CertNumber: "12345678", Grader: "PSA"},
			},
			seedCertNumbers: map[string]bool{"12345678": true},
			certLookup:      nil, // not needed for existing cert
			idGen:           func() string { return "test-id" },
			wantImported:    0,
			wantExisted:     1,
			wantPurchaseID:  "existing-id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID, Name: ExternalCampaignName}
			for k, v := range tc.seedPurchases {
				repo.purchases[k] = v
			}
			for k, v := range tc.seedCertNumbers {
				repo.certNumbers[k] = v
			}

			svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, certLookup: tc.certLookup, idGen: tc.idGen}
			result, err := svc.ImportCerts(context.Background(), []string{"12345678"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Imported != tc.wantImported {
				t.Errorf("imported = %d, want %d", result.Imported, tc.wantImported)
			}
			if result.AlreadyExisted != tc.wantExisted {
				t.Errorf("alreadyExisted = %d, want %d", result.AlreadyExisted, tc.wantExisted)
			}
			p := repo.purchases[tc.wantPurchaseID]
			if p == nil {
				t.Fatalf("purchase %q was not found", tc.wantPurchaseID)
			}
			if p.ReceivedAt == nil {
				t.Errorf("expected ReceivedAt to be set for purchase %q", tc.wantPurchaseID)
			}
		})
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
			svc := &service{campaigns: repo, purchases: repo, sales: repo, analytics: repo, finance: repo, pricing: repo, dh: repo, certLookup: certLookup, idGen: func() string { return "test-id" }}

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

// The following tests cover the DH push pipeline enrollment that ScanCert and
// ImportCerts do so that newly-received inventory actually flows into the DH
// sync pipeline instead of getting stranded with an empty dh_push_status.

func TestImportCerts_NewCertEnrollsForDHPush(t *testing.T) {
	repo := newMockRepo()
	repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID, Name: ExternalCampaignName}
	certLookup := &mockCertLookup{
		lookupFn: func(_ context.Context, cert string) (*CertInfo, error) {
			return &CertInfo{CertNumber: cert, CardName: "Charizard", Grade: 9, Category: "BASE SET", CardNumber: "4"}, nil
		},
	}
	svc := &service{
		campaigns: repo, purchases: repo, sales: repo, analytics: repo,
		finance: repo, pricing: repo, dh: repo, certLookup: certLookup,
		idGen: func() string { return "new-id" },
	}

	if _, err := svc.ImportCerts(context.Background(), []string{"12345678"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	created := repo.purchases["new-id"]
	if created == nil {
		t.Fatal("purchase not created")
	}
	if created.DHPushStatus != DHPushStatusPending {
		t.Errorf("dhPushStatus = %q, want %q", created.DHPushStatus, DHPushStatusPending)
	}
}

func TestImportCerts_ExistingCertEnrollsForDHPush(t *testing.T) {
	// A "matched" row in real life always has DHInventoryID != 0; NeedsDHPush
	// uses the inventory-id check, not the status string, to detect that case.
	tests := []struct {
		name          string
		initialStatus DHPushStatus
		initialInvID  int
		wantStatus    DHPushStatus
	}{
		{"empty status enrolls", "", 0, DHPushStatusPending},
		{"matched is preserved", DHPushStatusMatched, 42, DHPushStatusMatched},
		{"held is preserved", DHPushStatusHeld, 0, DHPushStatusHeld},
		{"unmatched is preserved", DHPushStatusUnmatched, 0, DHPushStatusUnmatched},
		{"dismissed is preserved", DHPushStatusDismissed, 0, DHPushStatusDismissed},
		{"manual is preserved", DHPushStatusManual, 99, DHPushStatusManual},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.purchases["p1"] = &Purchase{
				ID: "p1", CertNumber: "11111111", Grader: "PSA",
				CardName: "Charizard", DHPushStatus: tc.initialStatus,
				DHInventoryID: tc.initialInvID,
			}
			repo.certNumbers["11111111"] = true

			svc := &service{
				campaigns: repo, purchases: repo, sales: repo, analytics: repo,
				finance: repo, pricing: repo, dh: repo,
				idGen: func() string { return "unused" },
			}
			if _, err := svc.ImportCerts(context.Background(), []string{"11111111"}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := repo.purchases["p1"].DHPushStatus; got != tc.wantStatus {
				t.Errorf("dhPushStatus = %q, want %q", got, tc.wantStatus)
			}
		})
	}
}

func TestScanCert_ExistingEnrollsForDHPush(t *testing.T) {
	tests := []struct {
		name          string
		initialStatus DHPushStatus
		initialInvID  int
		wantStatus    DHPushStatus
	}{
		{"empty status enrolls", "", 0, DHPushStatusPending},
		{"matched is preserved", DHPushStatusMatched, 42, DHPushStatusMatched},
		{"held is preserved", DHPushStatusHeld, 0, DHPushStatusHeld},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			repo.purchases["p1"] = &Purchase{
				ID: "p1", CertNumber: "11111111", Grader: "PSA",
				CardName: "Charizard", DHPushStatus: tc.initialStatus,
				DHInventoryID: tc.initialInvID,
			}
			svc := &service{
				campaigns: repo, purchases: repo, sales: repo, analytics: repo,
				finance: repo, pricing: repo, dh: repo,
				idGen: func() string { return "unused" },
			}
			if _, err := svc.ScanCert(context.Background(), "11111111"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := repo.purchases["p1"].DHPushStatus; got != tc.wantStatus {
				t.Errorf("dhPushStatus = %q, want %q", got, tc.wantStatus)
			}
		})
	}
}

func TestScanCert_ResetsExhaustedSnapshot(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName: "Charizard", DHPushStatus: "",
		SnapshotStatus: SnapshotStatusExhausted, SnapshotRetryCount: 5,
	}
	svc := &service{
		campaigns: repo, purchases: repo, sales: repo, analytics: repo,
		finance: repo, pricing: repo, dh: repo,
		idGen: func() string { return "unused" },
	}
	if _, err := svc.ScanCert(context.Background(), "11111111"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := repo.purchases["p1"]
	if p.SnapshotStatus != SnapshotStatusPending {
		t.Errorf("snapshotStatus = %q, want %q", p.SnapshotStatus, SnapshotStatusPending)
	}
	if p.SnapshotRetryCount != 0 {
		t.Errorf("snapshotRetryCount = %d, want 0", p.SnapshotRetryCount)
	}
}

// TestScanCert_FirstReceiptEnrollsAgainstRealDBSemantics is the regression
// guard for the stale-ReceivedAt bug. It uses a repo wrapper that returns a
// COPY from GetPurchaseByCertNumber, mirroring the SQLite adapter. In the
// "first-time receipt" scenario the service must reflect its own SetReceivedAt
// write on the local struct before NeedsDHPush() runs — otherwise enrollment
// is silently skipped. The shared-pointer mockRepo hides this bug.
func TestScanCert_FirstReceiptEnrollsAgainstRealDBSemantics(t *testing.T) {
	base := newMockRepo()
	base.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName: "Charizard",
		// Crucially: ReceivedAt is nil — this is a PSA-sheet-synced row
		// that has never been received before.
		ReceivedAt:   nil,
		DHPushStatus: "",
	}
	wrapped := &dbLikePurchaseRepo{mockRepo: base}

	svc := &service{
		campaigns: base, purchases: wrapped, sales: base, analytics: base,
		finance: base, pricing: base, dh: base,
		idGen: func() string { return "unused" },
	}
	if _, err := svc.ScanCert(context.Background(), "11111111"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The stored row should now be enrolled in the DH push pipeline.
	stored := base.purchases["p1"]
	if stored.DHPushStatus != DHPushStatusPending {
		t.Errorf("dhPushStatus = %q, want %q (enrollment was silently skipped — regression of the stale-ReceivedAt fix)",
			stored.DHPushStatus, DHPushStatusPending)
	}
	if stored.ReceivedAt == nil {
		t.Errorf("receivedAt was never persisted")
	}
}

// TestImportCerts_FirstReceiptExistingEnrollsAgainstRealDBSemantics is the
// ImportCerts counterpart of the above — same stale-ReceivedAt bug, different
// entry point.
func TestImportCerts_FirstReceiptExistingEnrollsAgainstRealDBSemantics(t *testing.T) {
	base := newMockRepo()
	base.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID, Name: ExternalCampaignName}
	base.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName:     "Charizard",
		ReceivedAt:   nil,
		DHPushStatus: "",
	}
	base.certNumbers["11111111"] = true
	wrapped := &dbLikePurchaseRepo{mockRepo: base}

	svc := &service{
		campaigns: base, purchases: wrapped, sales: base, analytics: base,
		finance: base, pricing: base, dh: base,
		idGen: func() string { return "unused" },
	}
	if _, err := svc.ImportCerts(context.Background(), []string{"11111111"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored := base.purchases["p1"]
	if stored.DHPushStatus != DHPushStatusPending {
		t.Errorf("dhPushStatus = %q, want %q", stored.DHPushStatus, DHPushStatusPending)
	}
}

// TestImportCerts_NewCertSetsReceivedAtOnStructLiteral guards the new-cert
// ordering fix: the struct literal carries ReceivedAt so the initial insert
// is atomic (status + received together). A SetReceivedAt failure after that
// no longer strands the row with pending status and no receipt.
func TestImportCerts_NewCertSetsReceivedAtOnStructLiteral(t *testing.T) {
	repo := newMockRepo()
	repo.campaigns[ExternalCampaignID] = &Campaign{ID: ExternalCampaignID, Name: ExternalCampaignName}
	certLookup := &mockCertLookup{
		lookupFn: func(_ context.Context, cert string) (*CertInfo, error) {
			return &CertInfo{CertNumber: cert, CardName: "Charizard", Grade: 9, Category: "BASE SET", CardNumber: "4"}, nil
		},
	}
	svc := &service{
		campaigns: repo, purchases: repo, sales: repo, analytics: repo,
		finance: repo, pricing: repo, dh: repo, certLookup: certLookup,
		idGen: func() string { return "new-id" },
	}
	if _, err := svc.ImportCerts(context.Background(), []string{"12345678"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	created := repo.purchases["new-id"]
	if created == nil {
		t.Fatal("purchase not created")
	}
	if created.ReceivedAt == nil {
		t.Error("ReceivedAt is nil on the created row; struct-literal fix not in place")
	}
	if created.DHPushStatus != DHPushStatusPending {
		t.Errorf("dhPushStatus = %q, want %q", created.DHPushStatus, DHPushStatusPending)
	}
	// Sanity: the parsed ReceivedAt is close to now.
	if created.ReceivedAt != nil {
		if _, err := time.Parse(time.RFC3339, *created.ReceivedAt); err != nil {
			t.Errorf("ReceivedAt not in RFC3339 form: %v", err)
		}
	}
}

// TestEnrollExistingInDHPushPipeline_RecordsEvent verifies that a successful
// enrollment emits a TypeEnrolled event with SourceCertIntake.
func TestEnrollExistingInDHPushPipeline_RecordsEvent(t *testing.T) {
	repo := newMockRepo()
	receivedAt := time.Now().Format(time.RFC3339)
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName: "Charizard", DHPushStatus: "",
		ReceivedAt: &receivedAt,
	}
	rec := &stubEventRecorder{}
	svc := &service{
		campaigns: repo, purchases: repo, sales: repo, analytics: repo,
		finance: repo, pricing: repo, dh: repo, eventRec: rec,
		idGen: func() string { return "unused" },
	}

	svc.enrollExistingInDHPushPipeline(context.Background(), repo.purchases["p1"], "11111111", "cert import")

	events := rec.snapshot()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Type != dhevents.TypeEnrolled {
		t.Errorf("type = %q, want %q", e.Type, dhevents.TypeEnrolled)
	}
	if e.PurchaseID != "p1" {
		t.Errorf("purchaseID = %q, want p1", e.PurchaseID)
	}
	if e.CertNumber != "11111111" {
		t.Errorf("certNumber = %q, want 11111111", e.CertNumber)
	}
	if e.NewPushStatus != DHPushStatusPending {
		t.Errorf("newPushStatus = %q, want %q", e.NewPushStatus, DHPushStatusPending)
	}
	if e.Source != dhevents.SourceCertIntake {
		t.Errorf("source = %q, want %q", e.Source, dhevents.SourceCertIntake)
	}
}

// TestEnrollExistingInDHPushPipeline_SkipsWhenNotNeeded verifies no event is
// recorded when NeedsDHPush() is false (e.g. already matched).
func TestEnrollExistingInDHPushPipeline_SkipsWhenNotNeeded(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		DHPushStatus: DHPushStatusMatched, DHInventoryID: 42,
	}
	rec := &stubEventRecorder{}
	svc := &service{
		campaigns: repo, purchases: repo, sales: repo, analytics: repo,
		finance: repo, pricing: repo, dh: repo, eventRec: rec,
		idGen: func() string { return "unused" },
	}

	svc.enrollExistingInDHPushPipeline(context.Background(), repo.purchases["p1"], "11111111", "cert import")

	if got := len(rec.snapshot()); got != 0 {
		t.Errorf("events = %d, want 0 (matched rows should not re-enroll)", got)
	}
}

func TestScanCert_DoesNotResetHealthySnapshot(t *testing.T) {
	repo := newMockRepo()
	repo.purchases["p1"] = &Purchase{
		ID: "p1", CertNumber: "11111111", Grader: "PSA",
		CardName: "Charizard", DHPushStatus: "",
		SnapshotStatus: SnapshotStatusNone, SnapshotRetryCount: 0,
	}
	svc := &service{
		campaigns: repo, purchases: repo, sales: repo, analytics: repo,
		finance: repo, pricing: repo, dh: repo,
		idGen: func() string { return "unused" },
	}
	if _, err := svc.ScanCert(context.Background(), "11111111"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p := repo.purchases["p1"]; p.SnapshotStatus != SnapshotStatusNone {
		t.Errorf("snapshotStatus = %q, want empty/none", p.SnapshotStatus)
	}
}
