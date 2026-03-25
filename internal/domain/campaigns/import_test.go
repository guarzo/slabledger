package campaigns

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

func internalTestIDGen() func() string {
	var counter atomic.Int64
	return func() string { return fmt.Sprintf("test-id-%d", counter.Add(1)) }
}

func TestExtractGrade(t *testing.T) {
	tests := []struct {
		title string
		want  float64
	}{
		{"2021 Pokemon Celebrations Charizard PSA 9", 9},
		{"PSA 10 Pikachu VMAX", 10},
		{"psa 8 Blastoise Base Set", 8},
		{"PSA10 Gold Star Umbreon", 10},
		{"No grade mentioned", 0},
		{"Some PSA Card", 0},
		{"PSA 0 invalid", 0},
		{"PSA 11 out of range", 0},
		// Half-grade support
		{"2024 Pokemon Prismatic Umbreon PSA 8.5", 8.5},
		{"PSA 9.5 Charizard ex", 9.5},
		{"psa 1.5 Fair Card", 1.5},
	}

	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			got := ExtractGrade(tc.title)
			if got != tc.want {
				t.Errorf("ExtractGrade(%q) = %v, want %v", tc.title, got, tc.want)
			}
		})
	}
}

func TestParseCLDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
		err   bool
	}{
		{"03/09/2026", "2026-03-09", false},
		{"5/5/2023", "2023-05-05", false},
		{"12/31/2024", "2024-12-31", false},
		{"7/30/2021", "2021-07-30", false},
		{"invalid", "", true},
		{"2026-03-09", "", true}, // Wrong format
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseCLDate(tc.input)
			if tc.err && err == nil {
				t.Error("expected error")
			}
			if !tc.err && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ParseCLDate(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCLCardName(t *testing.T) {
	tests := []struct {
		name   string
		row    CLExportRow
		expect string
	}{
		{"prefers Player field", CLExportRow{Player: "Umbreon Ex", Card: "2025 Pokemon Svp Umbreon Ex PSA 10"}, "Umbreon Ex"},
		{"falls back to Card when Player empty", CLExportRow{Card: "Charizard PSA 9"}, "Charizard PSA 9"},
		{"both empty", CLExportRow{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CLCardName(tc.row)
			if got != tc.expect {
				t.Errorf("CLCardName() = %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestExtractCardNumberFromPSATitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"2021 Pokemon Celebrations #25/25 Charizard PSA 9", "25/25"},
		{"Pokemon Base Set Charizard 4/102 PSA 8", "4/102"},
		{"2023 Pokemon Svp #68 Umbreon Ex PSA 10", "68"},
		{"Pokemon 151 Charizard 6 PSA 10", "6"},
		{"No card number PSA 9", ""},
		{"Just a random title", ""},
		{"Pokemon #123 PSA 7 Mint", "123"},
		{"2024 Pokemon #TG23/TG30 Card PSA 10", "TG23/TG30"},
		{"Pokemon Base GG44/GG70 PSA 9", "GG44/GG70"},
		{"Pokemon SWSH123 PSA 10", "SWSH123"},
		{"#TG23 Trainer Gallery Card", "TG23"},
	}
	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			got := ExtractCardNumberFromPSATitle(tc.title)
			if got != tc.want {
				t.Errorf("ExtractCardNumberFromPSATitle(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}
}

func TestIsGenericSetName(t *testing.T) {
	generics := []string{"", "POKEMON CARDS", "pokemon cards", "TCG Cards", "tcg cards", "Cards", "Pokemon", "Other"}
	for _, s := range generics {
		if !isGenericSetName(s) {
			t.Errorf("isGenericSetName(%q) = false, want true", s)
		}
	}
	specifics := []string{"Pokemon Expedition", "Svp En-Sv Black Star Promo", "Base Set"}
	for _, s := range specifics {
		if isGenericSetName(s) {
			t.Errorf("isGenericSetName(%q) = true, want false", s)
		}
	}
}

func TestParsePSAListingTitle(t *testing.T) {
	tests := []struct {
		title   string
		wantSet string
		wantNum string
	}{
		{
			"2002 POKEMON EXPEDITION 56 MEWTWO-REVERSE FOIL PSA 9",
			"EXPEDITION", "56",
		},
		{
			"2025 POKEMON SVP EN-SV BLACK STAR PROMO 176 UMBREON EX PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION PSA 10",
			"SVP EN-SV BLACK STAR PROMO", "176",
		},
		{
			"2024 POKEMON SVP EN-SV BLACK STAR PROMO 161 CHARIZARD EX CHARIZARD ex SUPER-PREMIUM COLLECTION PSA 10",
			"SVP EN-SV BLACK STAR PROMO", "161",
		},
		{
			"2023 Pokemon Svp #68 Umbreon Ex PSA 10",
			"", "", // Has #-prefixed number, not matching the YYYY POKEMON SET NUM NAME pattern
		},
		{
			"No card number PSA 9",
			"", "",
		},
		{
			"2021 POKEMON CELEBRATIONS 25/25 CHARIZARD PSA 9",
			"CELEBRATIONS", "25/25",
		},
		{
			"2024 POKEMON PRISMATIC 56 UMBREON PSA 8.5",
			"PRISMATIC", "56",
		},
	}
	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			gotSet, gotNum := ParsePSAListingTitle(tc.title)
			if gotSet != tc.wantSet {
				t.Errorf("ParsePSAListingTitle(%q) set = %q, want %q", tc.title, gotSet, tc.wantSet)
			}
			if gotNum != tc.wantNum {
				t.Errorf("ParsePSAListingTitle(%q) num = %q, want %q", tc.title, gotNum, tc.wantNum)
			}
		})
	}
}

func TestParseShopifyTags(t *testing.T) {
	tests := []struct {
		name       string
		tags       string
		wantName   string
		wantNumber string
		wantSet    string
		wantSport  string
		wantErr    bool
	}{
		{"empty string", "", "", "", "", "", true},
		{"whitespace only", "   ", "", "", "", "", true},
		{"single tag (card name only)", "pokemon", "pokemon", "", "", "", false},
		{"all four fields", "Charizard,4/102,Base Set,Pokemon", "Charizard", "4/102", "Base Set", "Pokemon", false},
		{"tags with whitespace", " Umbreon Ex , 176 , SVP Promo , Pokemon ", "Umbreon Ex", "176", "SVP Promo", "Pokemon", false},
		{"two fields only", "Pikachu,25/25", "Pikachu", "25/25", "", "", false},
		{"three fields only", "Mewtwo,56,Expedition", "Mewtwo", "56", "Expedition", "", false},
		{"too many parts", "a,b,c,d,e", "", "", "", "", true},
		{"empty card name", ",56,Base Set", "", "", "", "", true},
		{"invalid card number format", "Charizard,not a number!,Base Set", "", "", "", "", true},
		{"alphanumeric card number", "Charizard,TG23/TG30,Crown Zenith", "Charizard", "TG23/TG30", "Crown Zenith", "", false},
		{"card name with empty number", "Charizard,,Base Set", "Charizard", "", "Base Set", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotNumber, gotSet, gotSport, err := ParseShopifyTags(tc.tags)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseShopifyTags(%q) expected error, got nil", tc.tags)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseShopifyTags(%q) unexpected error: %v", tc.tags, err)
			}
			if gotName != tc.wantName {
				t.Errorf("cardName = %q, want %q", gotName, tc.wantName)
			}
			if gotNumber != tc.wantNumber {
				t.Errorf("cardNumber = %q, want %q", gotNumber, tc.wantNumber)
			}
			if gotSet != tc.wantSet {
				t.Errorf("setName = %q, want %q", gotSet, tc.wantSet)
			}
			if gotSport != tc.wantSport {
				t.Errorf("sport = %q, want %q", gotSport, tc.wantSport)
			}
		})
	}
}

func TestExtractCardNameFromTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"plain card name", "Charizard", "Charizard"},
		{"strips PSA grade", "Charizard PSA 10", "Charizard"},
		{"strips BGS grade", "Umbreon Ex BGS 9.5", "Umbreon Ex"},
		{"strips CGC grade", "Pikachu CGC 8", "Pikachu"},
		{"strips SGC grade", "Mewtwo SGC 9", "Mewtwo"},
		{"strips condition suffix", "Charizard - Near Mint", "Charizard"},
		{"strips grade and condition", "Pikachu VMAX PSA 10 - Mint", "Pikachu VMAX"},
		{"preserves non-grading content", "2024 Pokemon Prismatic Umbreon", "2024 Pokemon Prismatic Umbreon"},
		{"returns original on empty result", "PSA 10", "PSA 10"},
		{"collapses whitespace", "Charizard   Base   Set  PSA 9", "Charizard Base Set"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractCardNameFromTitle(tc.title)
			if got != tc.want {
				t.Errorf("ExtractCardNameFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}
}

func TestExtractGraderAndGrade(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		wantGrader string
		wantGrade  float64
	}{
		{"PSA10 no space", "PSA10", "PSA", 10.0},
		{"PSA 10 with space", "PSA 10", "PSA", 10.0},
		{"BGS9.5 no space", "BGS9.5", "BGS", 9.5},
		{"BGS 9.5 with space", "BGS 9.5", "BGS", 9.5},
		{"CGC 8", "CGC 8", "CGC", 8.0},
		{"SGC 7", "SGC 7", "SGC", 7.0},
		{"empty string", "", "", 0},
		{"no grader keyword", "Grade 10 Charizard", "", 0},
		{"grader in full title", "2024 Pokemon Prismatic Umbreon PSA 9 Holo", "PSA", 9.0},
		{"case insensitive", "psa 10 Pikachu", "PSA", 10.0},
		{"half grade in title", "Charizard BGS 8.5 Base Set", "BGS", 8.5},
		{"grade out of range", "PSA 11 invalid", "", 0},
		{"grade zero", "PSA 0 invalid", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotGrader, gotGrade := ExtractGraderAndGrade(tc.title)
			if gotGrader != tc.wantGrader {
				t.Errorf("ExtractGraderAndGrade(%q) grader = %q, want %q", tc.title, gotGrader, tc.wantGrader)
			}
			if gotGrade != tc.wantGrade {
				t.Errorf("ExtractGraderAndGrade(%q) grade = %v, want %v", tc.title, gotGrade, tc.wantGrade)
			}
		})
	}
}

// --- Global Operations Tests ---

func TestService_RefreshCLValuesGlobal(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Create two campaigns
	c1 := &Campaign{Name: "Campaign A", BuyTermsCLPct: 0.78, PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c1)
	c2 := &Campaign{Name: "Campaign B", BuyTermsCLPct: 0.80, PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, c2)

	// Add purchases to each campaign
	p1 := &Purchase{CampaignID: c1.ID, CardName: "Charizard", CertNumber: "CERT001", GradeValue: 9, BuyCostCents: 10000, CLValueCents: 20000, Population: 100, PurchaseDate: "2026-01-01"}
	_ = svc.CreatePurchase(ctx, p1)
	p2 := &Purchase{CampaignID: c2.ID, CardName: "Pikachu", CertNumber: "CERT002", GradeValue: 10, BuyCostCents: 5000, CLValueCents: 10000, Population: 200, PurchaseDate: "2026-01-02"}
	_ = svc.CreatePurchase(ctx, p2)

	// Global refresh with a CSV that touches both campaigns and has one unknown cert
	rows := []CLExportRow{
		{SlabSerial: "CERT001", CurrentValue: 250.00, Population: 150},
		{SlabSerial: "CERT002", CurrentValue: 120.00, Population: 0}, // 0 population should keep old
		{SlabSerial: "CERT999", CurrentValue: 100.00},                // not found
	}

	result, err := svc.RefreshCLValuesGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("RefreshCLValuesGlobal: %v", err)
	}
	if result.Updated != 2 {
		t.Errorf("Updated = %d, want 2", result.Updated)
	}
	if result.NotFound != 1 {
		t.Errorf("NotFound = %d, want 1", result.NotFound)
	}

	// Verify per-campaign breakdown
	if len(result.ByCampaign) != 2 {
		t.Errorf("ByCampaign count = %d, want 2", len(result.ByCampaign))
	}
	if summary, ok := result.ByCampaign[c1.ID]; !ok || summary.Updated != 1 {
		t.Errorf("Campaign A summary: %+v", summary)
	}

	// Verify actual values updated
	updated1, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "CERT001")
	if updated1.CLValueCents != 25000 {
		t.Errorf("CERT001 CLValueCents = %d, want 25000", updated1.CLValueCents)
	}
	if updated1.Population != 150 {
		t.Errorf("CERT001 Population = %d, want 150", updated1.Population)
	}

	updated2, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "CERT002")
	if updated2.CLValueCents != 12000 {
		t.Errorf("CERT002 CLValueCents = %d, want 12000", updated2.CLValueCents)
	}
	if updated2.Population != 200 { // should keep original since CSV had 0
		t.Errorf("CERT002 Population = %d, want 200 (kept original)", updated2.Population)
	}
}

func TestService_ImportCLExportGlobal_AutoAllocate(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Campaign A: high grade, high price
	cA := &Campaign{Name: "High Grade", Phase: PhaseActive, GradeRange: "9-10", PriceRange: "100-500", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, cA)

	// Campaign B: low grade, low price
	cB := &Campaign{Name: "Low Grade", Phase: PhaseActive, GradeRange: "7-8", PriceRange: "10-100", PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, cB)

	rows := []CLExportRow{
		// Should match Campaign A (grade 9, $150)
		{SlabSerial: "NEW001", Card: "Charizard PSA 9", Investment: 150, CurrentValue: 300, DatePurchased: "2026-01-01"},
		// Should match Campaign B (grade 8, $50)
		{SlabSerial: "NEW002", Card: "Pikachu PSA 8", Investment: 50, CurrentValue: 80, DatePurchased: "2026-01-02"},
		// Grade 5 matches neither
		{SlabSerial: "NEW003", Card: "Blastoise PSA 5", Condition: "PSA 5", Investment: 30, CurrentValue: 40, DatePurchased: "2026-01-03"},
	}

	result, err := svc.ImportCLExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportCLExportGlobal: %v", err)
	}
	if result.Allocated != 2 {
		t.Errorf("Allocated = %d, want 2", result.Allocated)
	}
	if result.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", result.Unmatched)
	}

	// Verify allocations
	for _, item := range result.Results {
		switch item.CertNumber {
		case "NEW001":
			if item.Status != "allocated" || item.CampaignID != cA.ID {
				t.Errorf("NEW001: status=%s, campaignID=%s (want allocated to %s)", item.Status, item.CampaignID, cA.ID)
			}
		case "NEW002":
			if item.Status != "allocated" || item.CampaignID != cB.ID {
				t.Errorf("NEW002: status=%s, campaignID=%s (want allocated to %s)", item.Status, item.CampaignID, cB.ID)
			}
		case "NEW003":
			if item.Status != "unmatched" {
				t.Errorf("NEW003: status=%s, want unmatched", item.Status)
			}
		}
	}

	// Verify sourcing fees stamped correctly
	p1, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "NEW001")
	if p1.PSASourcingFeeCents != 300 {
		t.Errorf("NEW001 PSASourcingFeeCents = %d, want 300", p1.PSASourcingFeeCents)
	}
	p2, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "NEW002")
	if p2.PSASourcingFeeCents != 500 {
		t.Errorf("NEW002 PSASourcingFeeCents = %d, want 500", p2.PSASourcingFeeCents)
	}
}

func TestService_ImportCLExportGlobal_RefreshExisting(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c := &Campaign{Name: "Test", Phase: PhaseActive, GradeRange: "9-10", PriceRange: "50-500", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c)

	// Pre-existing purchase
	p := &Purchase{CampaignID: c.ID, CardName: "Charizard", CertNumber: "EXIST001", GradeValue: 9, BuyCostCents: 15000, CLValueCents: 20000, Population: 100, PurchaseDate: "2026-01-01"}
	_ = svc.CreatePurchase(ctx, p)

	rows := []CLExportRow{
		// Existing cert → should refresh
		{SlabSerial: "EXIST001", Card: "Charizard PSA 9", Investment: 150, CurrentValue: 350, Population: 200},
		// New cert → should allocate
		{SlabSerial: "NEW001", Card: "Pikachu PSA 10", Investment: 200, CurrentValue: 400, DatePurchased: "2026-02-01"},
	}

	result, err := svc.ImportCLExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportCLExportGlobal: %v", err)
	}
	if result.Refreshed != 1 {
		t.Errorf("Refreshed = %d, want 1", result.Refreshed)
	}
	if result.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1", result.Allocated)
	}

	// Verify refresh updated CL value
	refreshed, _ := repo.GetPurchaseByCertNumber(ctx, "PSA", "EXIST001")
	if refreshed.CLValueCents != 35000 {
		t.Errorf("EXIST001 CLValueCents = %d, want 35000", refreshed.CLValueCents)
	}
}

func TestService_ImportCLExportGlobal_Ambiguous(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Two campaigns with overlapping criteria
	c1 := &Campaign{Name: "Broad A", Phase: PhaseActive, GradeRange: "9-10", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c1)
	c2 := &Campaign{Name: "Broad B", Phase: PhaseActive, GradeRange: "9-10", PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, c2)

	rows := []CLExportRow{
		{SlabSerial: "AMB001", Card: "Charizard PSA 9", Investment: 150, CurrentValue: 300, DatePurchased: "2026-01-01"},
	}

	result, err := svc.ImportCLExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportCLExportGlobal: %v", err)
	}
	if result.Ambiguous != 1 {
		t.Errorf("Ambiguous = %d, want 1", result.Ambiguous)
	}
	if result.Results[0].Status != "ambiguous" {
		t.Errorf("status = %s, want ambiguous", result.Results[0].Status)
	}
	if len(result.Results[0].Candidates) != 2 {
		t.Errorf("candidates = %d, want 2", len(result.Results[0].Candidates))
	}
}

func TestService_ImportCLExportGlobal_SkipsClosedCampaigns(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Active campaign
	cActive := &Campaign{Name: "Active", Phase: PhaseActive, GradeRange: "9-10", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, cActive)

	// Closed campaign with same criteria — should NOT receive allocations
	cClosed := &Campaign{Name: "Closed", Phase: PhaseClosed, GradeRange: "9-10", PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, cClosed)

	rows := []CLExportRow{
		{SlabSerial: "SKIP001", Card: "Charizard PSA 9", Investment: 150, CurrentValue: 300, DatePurchased: "2026-01-01"},
	}

	result, err := svc.ImportCLExportGlobal(ctx, rows)
	if err != nil {
		t.Fatalf("ImportCLExportGlobal: %v", err)
	}
	if result.Allocated != 1 {
		t.Errorf("Allocated = %d, want 1", result.Allocated)
	}
	if result.Ambiguous != 0 {
		t.Errorf("Ambiguous = %d, want 0 (closed campaign should not cause ambiguity)", result.Ambiguous)
	}
	if result.Results[0].CampaignID != cActive.ID {
		t.Errorf("allocated to %s, want %s (active campaign)", result.Results[0].CampaignID, cActive.ID)
	}
}

func TestService_ExportCLFormatGlobal(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	// Create two campaigns with purchases
	c1 := &Campaign{Name: "Campaign A", BuyTermsCLPct: 0.78, PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c1)
	c2 := &Campaign{Name: "Campaign B", BuyTermsCLPct: 0.80, PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, c2)

	p1 := &Purchase{CampaignID: c1.ID, CardName: "Charizard", CertNumber: "CERT001", GradeValue: 9, BuyCostCents: 15000, CLValueCents: 20000, PurchaseDate: "2026-03-09"}
	_ = svc.CreatePurchase(ctx, p1)
	p2 := &Purchase{CampaignID: c2.ID, CardName: "Pikachu", CertNumber: "CERT002", GradeValue: 10, BuyCostCents: 5000, CLValueCents: 10000, PurchaseDate: "2026-01-15"}
	_ = svc.CreatePurchase(ctx, p2)

	// Mark p2 as sold so it doesn't appear in export
	s := &Sale{PurchaseID: p2.ID, SaleChannel: SaleChannelEbay, SalePriceCents: 12000, SaleDate: "2026-02-01"}
	_ = svc.CreateSale(ctx, s, c2, p2)

	entries, err := svc.ExportCLFormatGlobal(ctx, false)
	if err != nil {
		t.Fatalf("ExportCLFormatGlobal: %v", err)
	}

	// Only unsold purchase should be returned
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.CertNumber != "CERT001" {
		t.Errorf("CertNumber = %q, want CERT001", e.CertNumber)
	}
	if e.Grader != "PSA" {
		t.Errorf("Grader = %q, want PSA", e.Grader)
	}
	if e.Investment != 150.00 {
		t.Errorf("Investment = %f, want 150.00", e.Investment)
	}
	if e.EstimatedValue != 200.00 {
		t.Errorf("EstimatedValue = %f, want 200.00", e.EstimatedValue)
	}
	// Date should be converted from YYYY-MM-DD to M/D/YYYY
	if e.DatePurchased != "3/9/2026" {
		t.Errorf("DatePurchased = %q, want 3/9/2026", e.DatePurchased)
	}
}

func TestService_ExportCLFormatGlobal_Empty(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	entries, err := svc.ExportCLFormatGlobal(ctx, false)
	if err != nil {
		t.Fatalf("ExportCLFormatGlobal: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestService_ExportCLFormatGlobal_MissingCLOnly(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c := &Campaign{Name: "Campaign A", BuyTermsCLPct: 0.78, PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c)

	// p1 has CL data, p2 does not
	p1 := &Purchase{CampaignID: c.ID, CardName: "Charizard", CertNumber: "CERT001", GradeValue: 9, BuyCostCents: 15000, CLValueCents: 20000, PurchaseDate: "2026-03-09"}
	_ = svc.CreatePurchase(ctx, p1)
	p2 := &Purchase{CampaignID: c.ID, CardName: "Pikachu", CertNumber: "CERT002", GradeValue: 10, BuyCostCents: 5000, CLValueCents: 0, PurchaseDate: "2026-01-15"}
	_ = svc.CreatePurchase(ctx, p2)

	// Without filter: both returned
	all, err := svc.ExportCLFormatGlobal(ctx, false)
	if err != nil {
		t.Fatalf("ExportCLFormatGlobal(false): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}

	// With filter: only p2 (missing CL data)
	missing, err := svc.ExportCLFormatGlobal(ctx, true)
	if err != nil {
		t.Fatalf("ExportCLFormatGlobal(true): %v", err)
	}
	if len(missing) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(missing))
	}
	if missing[0].CertNumber != "CERT002" {
		t.Errorf("CertNumber = %q, want CERT002", missing[0].CertNumber)
	}
}

func TestService_ReassignPurchase(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c1 := &Campaign{Name: "Source", PSASourcingFeeCents: 300}
	_ = svc.CreateCampaign(ctx, c1)
	c2 := &Campaign{Name: "Target", PSASourcingFeeCents: 500}
	_ = svc.CreateCampaign(ctx, c2)

	p := &Purchase{CampaignID: c1.ID, CardName: "Charizard", CertNumber: "MOVE001", GradeValue: 9, BuyCostCents: 15000, PSASourcingFeeCents: 300, PurchaseDate: "2026-01-01"}
	_ = svc.CreatePurchase(ctx, p)

	if err := svc.ReassignPurchase(ctx, p.ID, c2.ID); err != nil {
		t.Fatalf("ReassignPurchase: %v", err)
	}

	// Verify purchase moved
	moved, _ := repo.GetPurchase(ctx, p.ID)
	if moved.CampaignID != c2.ID {
		t.Errorf("CampaignID = %s, want %s", moved.CampaignID, c2.ID)
	}
	if moved.PSASourcingFeeCents != 500 {
		t.Errorf("PSASourcingFeeCents = %d, want 500", moved.PSASourcingFeeCents)
	}
}

func TestLooksLikeCollectorNumber(t *testing.T) {
	tests := []struct {
		token string
		want  bool
	}{
		// Starts with digit — always a collector number
		{"123", true},
		{"25/25", true},
		{"4", true},
		// Short alpha prefix (at or under maxCollectorNumberAlphaPrefix = 4)
		{"TG23", true},    // 2 alpha chars
		{"BW93", true},    // 2 alpha chars
		{"SWSH123", true}, // 4 alpha chars — exactly at boundary
		// Long alpha prefix (exceeds boundary) — Pokémon names
		{"PORYGON2", false}, // 7 alpha chars
		{"DEOXYS1", false},  // 6 alpha chars
		{"ABCDE5", false},   // 5 alpha chars — just over boundary
		// Edge cases
		{"", false},
		{"ABCD5", true}, // exactly 4 alpha chars
	}

	for _, tc := range tests {
		t.Run(tc.token, func(t *testing.T) {
			got := looksLikeCollectorNumber(tc.token)
			if got != tc.want {
				t.Errorf("looksLikeCollectorNumber(%q) = %v, want %v", tc.token, got, tc.want)
			}
		})
	}
}

func TestService_ReassignPurchase_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, WithIDGenerator(internalTestIDGen()))
	ctx := context.Background()

	c := &Campaign{Name: "Target"}
	_ = svc.CreateCampaign(ctx, c)

	err := svc.ReassignPurchase(ctx, "nonexistent", c.ID)
	if err == nil {
		t.Error("expected error for nonexistent purchase")
	}
}

// TestCollectionSuffixRegistryExamples verifies that each Example in the
// collectionSuffixRegistry actually triggers the suffix stripping it documents.
func TestCollectionSuffixRegistryExamples(t *testing.T) {
	for _, cs := range collectionSuffixRegistry {
		if cs.Example == "" {
			continue
		}
		t.Run(cs.Pattern, func(t *testing.T) {
			result := stripCollectionSuffix(cs.Example)
			if result == cs.Example {
				t.Errorf("stripCollectionSuffix(%q) returned unchanged — pattern %q did not match", cs.Example, cs.Pattern)
			}
			if result == "" {
				t.Errorf("stripCollectionSuffix(%q) returned empty string", cs.Example)
			}
		})
	}
}

func TestStripCollectionSuffix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"prismatic figure collection", "UMBREON EX PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION", "UMBREON EX"},
		{"super premium collection", "CHARIZARD EX SUPER PREMIUM COLLECTION", "CHARIZARD EX"},
		{"trailing crown zenith", "RAYQUAZA-HOLO CRZ CROWN ZENITH", "RAYQUAZA-HOLO CRZ"},
		{"trailing special art rare", "MEGA GARDEVOIR EX SPECIAL ART RARE", "MEGA GARDEVOIR EX"},
		{"no suffix", "PIKACHU VMAX", "PIKACHU VMAX"},
		{"empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripCollectionSuffix(tc.input)
			if got != tc.want {
				t.Errorf("stripCollectionSuffix(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseCardMetadataFromTitle(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		category   string
		wantCard   string
		wantNumber string
		wantSet    string
	}{
		{
			"standard PSA title with specific category",
			"2002 POKEMON EXPEDITION 56 MEWTWO-REVERSE FOIL PSA 9",
			"GAME",
			"MEWTWO-REVERSE FOIL",
			"56",
			// "GAME" category maps to "Base Set" (not generic), so title-parsed set isn't used
			"Base Set",
		},
		{
			"standard PSA title with generic category",
			"2002 POKEMON EXPEDITION 56 MEWTWO-REVERSE FOIL PSA 9",
			"Other",
			"MEWTWO-REVERSE FOIL",
			"56",
			// "Other" is generic, so title-parsed set "EXPEDITION" is used
			"EXPEDITION",
		},
		{
			"ancient mew no number",
			"2000 POKEMON GAME MOVIE ANCIENT MEW POKEMON 2000 MOVIE PSA 9",
			"GAME MOVIE",
			"ANCIENT MEW",
			"",
			"Promo",
		},
		{
			"promo card with collection suffix",
			"2025 POKEMON SVP EN-SV BLACK STAR PROMO 176 UMBREON EX PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION PSA 10",
			"Other",
			"UMBREON EX",
			"176",
			"SVP EN-SV BLACK STAR PROMO",
		},
		{
			"GAME category maps to Base Set",
			"2001 POKEMON BASE SET 4/102 CHARIZARD PSA 8",
			"GAME",
			"CHARIZARD",
			"4/102",
			// "GAME" category maps directly to "Base Set"
			"Base Set",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta := parseCardMetadataFromTitle(tc.title, tc.category)
			if meta.CardName != tc.wantCard {
				t.Errorf("cardName = %q, want %q", meta.CardName, tc.wantCard)
			}
			if meta.CardNumber != tc.wantNumber {
				t.Errorf("cardNumber = %q, want %q", meta.CardNumber, tc.wantNumber)
			}
			if meta.SetName != tc.wantSet {
				t.Errorf("setName = %q, want %q", meta.SetName, tc.wantSet)
			}
		})
	}
}

func TestExtractVariantFromTitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"2024 POKEMON PRISMATIC 56 UMBREON REVERSE HOLO PSA 10", "REVERSE HOLO"},
		{"2023 POKEMON BASE SET 4 CHARIZARD HOLO PSA 9", "HOLO"},
		{"2024 POKEMON SV 1ST EDITION 100 PIKACHU PSA 10", "1ST EDITION"},
		{"2023 POKEMON BASE SET 4 CHARIZARD SHADOWLESS PSA 8", "SHADOWLESS"},
		{"2023 POKEMON BASE SET 4 CHARIZARD PSA 9", ""},
		// "HOLO" inside "HOLON" should NOT match — word-boundary regex prevents it
		{"2006 POKEMON HOLON PHANTOMS 42 PIKACHU PSA 9", ""},
		// Empty title
		{"", ""},
		// "1ST" inside "1ST EDITION" — longest match wins
		{"2024 POKEMON 1ST EDITION CHARIZARD PSA 10", "1ST EDITION"},
	}
	for _, tc := range tests {
		t.Run(tc.title, func(t *testing.T) {
			got := extractVariantFromTitle(tc.title)
			if got != tc.want {
				t.Errorf("extractVariantFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}
}

func TestParseNoNumberTitle(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		wantCard string
		wantSet  string
	}{
		{
			"Ancient Mew standard",
			"2000 POKEMON GAME MOVIE ANCIENT MEW POKEMON 2000 MOVIE PSA 9",
			"ANCIENT MEW", "GAME MOVIE",
		},
		{
			"Dark Mewtwo with descriptor",
			"2016 POKEMON DARK MEWTWO POKKEN TOURNAMENT PSA 8",
			"DARK MEWTWO POKKEN TOURNAMENT", "",
		},
		{
			"no match — normal card",
			"2024 POKEMON PRISMATIC 56 UMBREON PSA 10",
			"", "",
		},
		{
			"empty title",
			"",
			"", "",
		},
		{
			"MEW inside MEWTWO should not match ANCIENT MEW",
			"2024 POKEMON MEWTWO VMAX PSA 10",
			"", "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotCard, gotSet := parseNoNumberTitle(tc.title)
			if gotCard != tc.wantCard {
				t.Errorf("cardName = %q, want %q", gotCard, tc.wantCard)
			}
			if gotSet != tc.wantSet {
				t.Errorf("setName = %q, want %q", gotSet, tc.wantSet)
			}
		})
	}
}

func TestStripCollectionSuffix_Stacked(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"single trailing suffix",
			"RAYQUAZA-HOLO CRZ CROWN ZENITH",
			"RAYQUAZA-HOLO CRZ",
		},
		{
			"anywhere suffix",
			"UMBREON EX PRISMATIC EVOLUTIONS PREMIUM FIGURE COLLECTION",
			"UMBREON EX",
		},
		{
			"no suffix",
			"CHARIZARD EX",
			"CHARIZARD EX",
		},
		{
			"stacked trailing — both stripped",
			"MEGA GARDEVOIR EX SPECIAL ART RARE CROWN ZENITH",
			"MEGA GARDEVOIR EX",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripCollectionSuffix(tc.input)
			if got != tc.want {
				t.Errorf("stripCollectionSuffix(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseCardMetadataFromTitle_ParseWarning(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		category    string
		wantWarning bool
	}{
		{
			"normal parse — no warning",
			"2002 POKEMON EXPEDITION 56 MEWTWO-REVERSE FOIL PSA 9",
			"GAME",
			false,
		},
		{
			"generic set remains — warning",
			"2024 POKEMON 56 UMBREON PSA 10",
			"Other",
			// title parser extracts set "" (only "POKEMON" before card number, stripped),
			// category "Other" is generic with no title set to replace → warning
			true,
		},
		{
			"raw title fallback — warning",
			"JUST SOME RANDOM TEXT",
			"Other",
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta := parseCardMetadataFromTitle(tc.title, tc.category)
			hasWarning := meta.ParseWarning != ""
			if hasWarning != tc.wantWarning {
				t.Errorf("ParseWarning = %q, wantWarning = %v", meta.ParseWarning, tc.wantWarning)
			}
		})
	}
}

func TestCollectionSuffixRegistryNoDuplicates(t *testing.T) {
	seen := make(map[string]bool)
	for _, cs := range collectionSuffixRegistry {
		if seen[cs.Pattern] {
			t.Errorf("duplicate collection suffix pattern: %q", cs.Pattern)
		}
		seen[cs.Pattern] = true
	}
}

func TestResolveSetName(t *testing.T) {
	tests := []struct {
		name        string
		titleSet    string
		category    string
		wantSet     string
		wantWarning bool
	}{
		{"GAME maps to Base Set", "", "GAME", "Base Set", false},
		{"generic category uses titleSet", "ROCKET", "POKEMON CARDS", "ROCKET", false},
		{"titleSet GAME also resolved", "GAME", "POKEMON CARDS", "Base Set", false},
		{"non-generic stays", "", "Scarlet Violet", "Scarlet Violet", false},
		{"generic with no titleSet warns", "", "POKEMON CARDS", "POKEMON CARDS", true},
		{"both titleSet and category generic warns", "POKEMON", "POKEMON CARDS", "POKEMON", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotSet, gotWarning := resolveSetName(tc.titleSet, tc.category)
			if gotSet != tc.wantSet {
				t.Errorf("resolveSetName(%q, %q) set = %q, want %q", tc.titleSet, tc.category, gotSet, tc.wantSet)
			}
			hasWarning := gotWarning != ""
			if hasWarning != tc.wantWarning {
				t.Errorf("resolveSetName(%q, %q) warning = %q, wantWarning = %v", tc.titleSet, tc.category, gotWarning, tc.wantWarning)
			}
		})
	}
}
