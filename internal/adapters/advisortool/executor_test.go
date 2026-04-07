package advisortool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// newTestExecutor creates a CampaignToolExecutor backed by the given mock service.
func newTestExecutor(svc campaigns.Service) *CampaignToolExecutor {
	return NewCampaignToolExecutor(svc)
}

// TestDefinitions_RequiredTools verifies that all required tools are registered.
func TestDefinitions_RequiredTools(t *testing.T) {
	e := newTestExecutor(&mocks.MockCampaignService{})
	defs := e.Definitions()

	requiredTools := []string{
		"list_campaigns", "get_campaign_pnl", "get_pnl_by_channel",
		"get_campaign_tuning", "get_inventory_aging", "get_global_inventory", "get_flagged_inventory",
		"get_sell_sheet", "get_portfolio_health", "get_portfolio_insights",
		"get_capital_summary", "get_weekly_review", "get_capital_timeline",
		"get_channel_velocity", "get_dashboard_summary", "get_expected_values",
		"get_deslab_candidates", "get_campaign_suggestions", "run_projection",
		"suggest_price", "get_cert_lookup", "get_suggestion_stats",
		"evaluate_purchase",
		"get_expected_values_batch", "suggest_price_batch",
		"get_acquisition_targets", "get_deslab_opportunities",
		"get_market_intelligence", "get_dh_suggestions",
		"get_inventory_alerts", "get_data_gap_report",
	}

	registered := make(map[string]bool, len(defs))
	for _, d := range defs {
		registered[d.Name] = true
	}
	for _, name := range requiredTools {
		if !registered[name] {
			t.Errorf("Definitions() is missing required tool %q", name)
		}
	}
}

// TestDefinitionsFor_Subset verifies that only the requested tools are returned.
func TestDefinitionsFor_Subset(t *testing.T) {
	e := newTestExecutor(&mocks.MockCampaignService{})
	names := []string{"list_campaigns", "get_portfolio_health"}
	defs := e.DefinitionsFor(names)
	if len(defs) != 2 {
		t.Fatalf("DefinitionsFor(%v) returned %d tools, want 2", names, len(defs))
	}
	got := map[string]bool{}
	for _, d := range defs {
		got[d.Name] = true
	}
	for _, name := range names {
		if !got[name] {
			t.Errorf("DefinitionsFor result is missing tool %q", name)
		}
	}
}

// TestDefinitionsFor_Empty verifies that an empty name list returns zero tools.
func TestDefinitionsFor_Empty(t *testing.T) {
	e := newTestExecutor(&mocks.MockCampaignService{})
	defs := e.DefinitionsFor([]string{})
	if len(defs) != 0 {
		t.Errorf("DefinitionsFor([]) returned %d tools, want 0", len(defs))
	}
}

// TestExecute_UnknownTool verifies that calling an unregistered tool returns an error.
func TestExecute_UnknownTool(t *testing.T) {
	e := newTestExecutor(&mocks.MockCampaignService{})
	_, err := e.Execute(context.Background(), "does_not_exist", "{}")
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error %q does not mention 'unknown tool'", err.Error())
	}
}

// TestExecute_InvalidJSON verifies that passing malformed JSON to a campaign-id tool returns an error.
func TestExecute_InvalidJSON(t *testing.T) {
	e := newTestExecutor(&mocks.MockCampaignService{})
	// get_campaign_pnl requires a campaignId; send malformed JSON.
	_, err := e.Execute(context.Background(), "get_campaign_pnl", "{not valid json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestExecute_ListCampaigns verifies that the list_campaigns tool calls ListCampaigns and returns JSON.
func TestExecute_ListCampaigns(t *testing.T) {
	want := []campaigns.Campaign{
		{ID: "camp-1", Name: "Vintage Test"},
		{ID: "camp-2", Name: "Modern Test"},
	}
	svc := &mocks.MockCampaignService{
		ListCampaignsFn: func(_ context.Context, activeOnly bool) ([]campaigns.Campaign, error) {
			return want, nil
		},
	}
	e := newTestExecutor(svc)
	result, err := e.Execute(context.Background(), "list_campaigns", `{"activeOnly":false}`)
	if err != nil {
		t.Fatalf("Execute list_campaigns: %v", err)
	}

	var got []campaigns.Campaign
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d campaigns, want %d", len(got), len(want))
	}
	if got[0].ID != want[0].ID {
		t.Errorf("got[0].ID = %q, want %q", got[0].ID, want[0].ID)
	}
	if got[1].Name != want[1].Name {
		t.Errorf("got[1].Name = %q, want %q", got[1].Name, want[1].Name)
	}
}

// TestExecute_GetPortfolioHealth verifies that the get_portfolio_health tool calls GetPortfolioHealth and returns JSON.
func TestExecute_GetPortfolioHealth(t *testing.T) {
	want := &campaigns.PortfolioHealth{
		TotalDeployed:  100000,
		TotalRecovered: 80000,
		OverallROI:     -0.2,
	}
	svc := &mocks.MockCampaignService{
		GetPortfolioHealthFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
			return want, nil
		},
	}
	e := newTestExecutor(svc)
	result, err := e.Execute(context.Background(), "get_portfolio_health", "{}")
	if err != nil {
		t.Fatalf("Execute get_portfolio_health: %v", err)
	}

	var got campaigns.PortfolioHealth
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got.TotalDeployed != want.TotalDeployed {
		t.Errorf("TotalDeployed = %d, want %d", got.TotalDeployed, want.TotalDeployed)
	}
	if got.TotalRecovered != want.TotalRecovered {
		t.Errorf("TotalRecovered = %d, want %d", got.TotalRecovered, want.TotalRecovered)
	}
}

// TestExecute_GetCampaignPNL verifies that campaignId is extracted from args and forwarded to GetCampaignPNL.
func TestExecute_GetCampaignPNL(t *testing.T) {
	const wantID = "camp-42"
	var capturedID string

	svc := &mocks.MockCampaignService{
		GetCampaignPNLFn: func(_ context.Context, campaignID string) (*campaigns.CampaignPNL, error) {
			capturedID = campaignID
			return &campaigns.CampaignPNL{CampaignID: campaignID, TotalSpendCents: 5000}, nil
		},
	}
	e := newTestExecutor(svc)
	result, err := e.Execute(context.Background(), "get_campaign_pnl", `{"campaignId":"camp-42"}`)
	if err != nil {
		t.Fatalf("Execute get_campaign_pnl: %v", err)
	}
	if capturedID != wantID {
		t.Errorf("capturedID = %q, want %q", capturedID, wantID)
	}

	var got campaigns.CampaignPNL
	if err := json.Unmarshal([]byte(result), &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got.TotalSpendCents != 5000 {
		t.Errorf("TotalSpendCents = %d, want 5000", got.TotalSpendCents)
	}
}

// TestExecute_ServiceError verifies that a service error is propagated by Execute.
func TestExecute_ServiceError(t *testing.T) {
	serviceErr := errors.New("database unavailable")
	svc := &mocks.MockCampaignService{
		ListCampaignsFn: func(_ context.Context, _ bool) ([]campaigns.Campaign, error) {
			return nil, serviceErr
		},
	}
	e := newTestExecutor(svc)
	_, err := e.Execute(context.Background(), "list_campaigns", "{}")
	if err == nil {
		t.Fatal("expected error from service, got nil")
	}
	if !errors.Is(err, serviceErr) {
		t.Errorf("err = %v, want to wrap %v", err, serviceErr)
	}
}

// TestToJSON_TruncatesAt15KB verifies that toJSON output is limited to 15KB.
func TestToJSON_TruncatesAt15KB(t *testing.T) {
	// Build a slice that marshals to >15000 bytes
	padding := strings.Repeat("x", 100)
	items := make([]map[string]string, 300)
	for i := range items {
		items[i] = map[string]string{"id": fmt.Sprintf("item-%04d", i), "data": padding}
	}
	result := toJSON(items)
	if len(result) > 15000 {
		t.Errorf("toJSON output = %d bytes, want <= 15000", len(result))
	}
}

func TestExecute_GetExpectedValuesBatch(t *testing.T) {
	tests := []struct {
		name      string
		mockSetup func(*mocks.MockCampaignService)
		payload   string
		wantErr   bool
		validate  func(t *testing.T, result string)
	}{
		{
			name: "WithIDs",
			mockSetup: func(svc *mocks.MockCampaignService) {
				svc.GetExpectedValuesFn = func(_ context.Context, campaignID string) (*campaigns.EVPortfolio, error) {
					return &campaigns.EVPortfolio{
						TotalEVCents:  1000,
						PositiveCount: 2,
						Items: []campaigns.ExpectedValue{
							{CardName: "Charizard-" + campaignID, EVCents: 500},
						},
					}, nil
				}
			},
			payload: `{"campaignIds":["camp-1","camp-2"]}`,
			validate: func(t *testing.T, result string) {
				var got map[string]*campaigns.EVPortfolio
				if err := json.Unmarshal([]byte(result), &got); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(got) != 2 {
					t.Fatalf("got %d campaigns, want 2", len(got))
				}
				if got["camp-1"].Items[0].CardName != "Charizard-camp-1" {
					t.Errorf("camp-1 card = %q, want Charizard-camp-1", got["camp-1"].Items[0].CardName)
				}
				if got["camp-2"].Items[0].CardName != "Charizard-camp-2" {
					t.Errorf("camp-2 card = %q, want Charizard-camp-2", got["camp-2"].Items[0].CardName)
				}
			},
		},
		{
			name: "AllActive",
			mockSetup: func(svc *mocks.MockCampaignService) {
				svc.ListCampaignsFn = func(_ context.Context, activeOnly bool) ([]campaigns.Campaign, error) {
					if !activeOnly {
						return nil, fmt.Errorf("expected activeOnly=true, got false")
					}
					return []campaigns.Campaign{
						{ID: "a1", Name: "Alpha"},
						{ID: "a2", Name: "Beta"},
					}, nil
				}
				svc.GetExpectedValuesFn = func(_ context.Context, campaignID string) (*campaigns.EVPortfolio, error) {
					return &campaigns.EVPortfolio{TotalEVCents: 100}, nil
				}
			},
			payload: `{}`,
			validate: func(t *testing.T, result string) {
				var got map[string]*campaigns.EVPortfolio
				if err := json.Unmarshal([]byte(result), &got); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(got) != 2 {
					t.Fatalf("got %d campaigns, want 2", len(got))
				}
				if _, ok := got["a1"]; !ok {
					t.Error("missing campaign a1")
				}
				if _, ok := got["a2"]; !ok {
					t.Error("missing campaign a2")
				}
			},
		},
		{
			name: "PartialFailure",
			mockSetup: func(svc *mocks.MockCampaignService) {
				svc.GetExpectedValuesFn = func(_ context.Context, campaignID string) (*campaigns.EVPortfolio, error) {
					if campaignID == "bad" {
						return nil, errors.New("not found")
					}
					return &campaigns.EVPortfolio{TotalEVCents: 200}, nil
				}
			},
			payload: `{"campaignIds":["good","bad"]}`,
			validate: func(t *testing.T, result string) {
				var got map[string]json.RawMessage
				if err := json.Unmarshal([]byte(result), &got); err != nil {
					t.Fatalf("unmarshal top-level: %v", err)
				}
				// Verify "good" has TotalEVCents
				goodRaw, ok := got["good"]
				if !ok {
					t.Fatal("result missing 'good' campaign")
				}
				var goodObj map[string]json.RawMessage
				if err := json.Unmarshal(goodRaw, &goodObj); err != nil {
					t.Fatalf("unmarshal good: %v", err)
				}
				if _, hasTotalEV := goodObj["totalEvCents"]; !hasTotalEV {
					t.Error("good campaign missing totalEvCents")
				}
				// Verify "bad" has an "error" key
				badRaw, ok := got["bad"]
				if !ok {
					t.Fatal("result missing 'bad' campaign")
				}
				var badObj map[string]json.RawMessage
				if err := json.Unmarshal(badRaw, &badObj); err != nil {
					t.Fatalf("unmarshal bad: %v", err)
				}
				if _, hasError := badObj["error"]; !hasError {
					t.Error("bad campaign missing error key")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{}
			tc.mockSetup(svc)
			e := newTestExecutor(svc)
			result, err := e.Execute(context.Background(), "get_expected_values_batch", tc.payload)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if tc.validate != nil {
				tc.validate(t, result)
			}
		})
	}
}

func TestExecute_SuggestPriceBatch(t *testing.T) {
	tests := []struct {
		name      string
		mockSetup func(*mocks.MockCampaignService)
		payload   string
		wantErr   bool
		validate  func(t *testing.T, result string)
	}{
		{
			name: "AllOK",
			mockSetup: func(svc *mocks.MockCampaignService) {
				svc.SetAISuggestedPriceFn = func(_ context.Context, purchaseID string, priceCents int) error {
					return nil
				}
			},
			payload: `{"suggestions":[{"purchaseId":"p1","priceCents":1500},{"purchaseId":"p2","priceCents":2000}]}`,
			validate: func(t *testing.T, result string) {
				var got struct {
					Results []struct {
						PurchaseID string `json:"purchaseId"`
						Status     string `json:"status"`
					} `json:"results"`
				}
				if err := json.Unmarshal([]byte(result), &got); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(got.Results) != 2 {
					t.Fatalf("got %d results, want 2", len(got.Results))
				}
				for _, r := range got.Results {
					if r.Status != "ok" {
						t.Errorf("purchaseId %s: status = %q, want ok", r.PurchaseID, r.Status)
					}
				}
			},
		},
		{
			name: "PartialFailure",
			mockSetup: func(svc *mocks.MockCampaignService) {
				svc.SetAISuggestedPriceFn = func(_ context.Context, purchaseID string, priceCents int) error {
					if purchaseID == "bad" {
						return errors.New("purchase not found")
					}
					return nil
				}
			},
			payload: `{"suggestions":[{"purchaseId":"good","priceCents":1500},{"purchaseId":"bad","priceCents":2000}]}`,
			validate: func(t *testing.T, result string) {
				var got struct {
					Results []struct {
						PurchaseID string `json:"purchaseId"`
						Status     string `json:"status"`
						Error      string `json:"error,omitempty"`
					} `json:"results"`
				}
				if err := json.Unmarshal([]byte(result), &got); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(got.Results) != 2 {
					t.Fatalf("got %d results, want 2", len(got.Results))
				}
				if got.Results[0].Status != "ok" {
					t.Errorf("good: status = %q, want ok", got.Results[0].Status)
				}
				if got.Results[1].Status != "error" {
					t.Errorf("bad: status = %q, want error", got.Results[1].Status)
				}
				if got.Results[1].Error == "" {
					t.Error("bad: expected error message")
				}
			},
		},
		{
			name:      "Empty",
			mockSetup: func(svc *mocks.MockCampaignService) {},
			payload:   `{"suggestions":[]}`,
			wantErr:   true,
		},
		{
			name:      "InvalidItem",
			mockSetup: func(svc *mocks.MockCampaignService) {},
			payload:   `{"suggestions":[{"purchaseId":"","priceCents":1500}]}`,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{}
			tc.mockSetup(svc)
			e := newTestExecutor(svc)
			result, err := e.Execute(context.Background(), "suggest_price_batch", tc.payload)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if tc.validate != nil {
				tc.validate(t, result)
			}
		})
	}
}

func TestExecute_GetDashboardSummary(t *testing.T) {
	tests := []struct {
		name               string
		weeklyReviewFn     func(_ context.Context) (*campaigns.WeeklyReviewSummary, error)
		capitalSummaryFn   func(_ context.Context) (*campaigns.CapitalSummary, error)
		portfolioHealthFn  func(_ context.Context) (*campaigns.PortfolioHealth, error)
		channelVelocityFn  func(_ context.Context) ([]campaigns.ChannelVelocity, error)
		wantSubstrings     []string
		wantMissingStrings []string
	}{
		{
			name: "full dashboard with critical capital",
			weeklyReviewFn: func(_ context.Context) (*campaigns.WeeklyReviewSummary, error) {
				return &campaigns.WeeklyReviewSummary{
					PurchasesThisWeek:    10,
					PurchasesLastWeek:    8,
					SpendThisWeekCents:   50000,
					SalesThisWeek:        5,
					SalesLastWeek:        3,
					RevenueThisWeekCents: 30000,
					ProfitThisWeekCents:  5000,
					ProfitLastWeekCents:  3000,
				}, nil
			},
			capitalSummaryFn: func(_ context.Context) (*campaigns.CapitalSummary, error) {
				return &campaigns.CapitalSummary{
					OutstandingCents:     2500000,
					RecoveryRate30dCents: 500000,
					WeeksToCover:         21.5,
					RecoveryTrend:        campaigns.TrendStable,
					AlertLevel:           campaigns.AlertCritical,
				}, nil
			},
			portfolioHealthFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
				return &campaigns.PortfolioHealth{
					Campaigns: []campaigns.CampaignHealth{
						{CampaignName: "Test", HealthStatus: "healthy", HealthReason: "good", CapitalAtRisk: 1000},
					},
				}, nil
			},
			channelVelocityFn: func(_ context.Context) ([]campaigns.ChannelVelocity, error) {
				return []campaigns.ChannelVelocity{
					{Channel: "ebay", AvgDaysToSell: 14.5, SaleCount: 5},
				}, nil
			},
			wantSubstrings: []string{
				`"purchaseCount":10`,
				`"alertLevel":"critical"`,
				`"purchaseCountWoW":2`,
				`"recoveryTrend":"stable"`,
			},
		},
		{
			name: "healthy capital with ok alert",
			weeklyReviewFn: func(_ context.Context) (*campaigns.WeeklyReviewSummary, error) {
				return &campaigns.WeeklyReviewSummary{
					PurchasesThisWeek:   3,
					PurchasesLastWeek:   3,
					SalesThisWeek:       2,
					SalesLastWeek:       2,
					ProfitThisWeekCents: 1000,
					ProfitLastWeekCents: 1000,
				}, nil
			},
			capitalSummaryFn: func(_ context.Context) (*campaigns.CapitalSummary, error) {
				return &campaigns.CapitalSummary{
					OutstandingCents:     50000,
					RecoveryRate30dCents: 100000,
					WeeksToCover:         2.15,
					RecoveryTrend:        campaigns.TrendImproving,
					AlertLevel:           campaigns.AlertOK,
				}, nil
			},
			portfolioHealthFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
				return &campaigns.PortfolioHealth{}, nil
			},
			channelVelocityFn: func(_ context.Context) ([]campaigns.ChannelVelocity, error) {
				return nil, nil
			},
			wantSubstrings: []string{
				`"alertLevel":"ok"`,
				`"recoveryTrend":"improving"`,
				`"purchaseCount":3`,
			},
		},
		{
			name: "partial errors still returns available data",
			weeklyReviewFn: func(_ context.Context) (*campaigns.WeeklyReviewSummary, error) {
				return nil, fmt.Errorf("weekly review unavailable")
			},
			capitalSummaryFn: func(_ context.Context) (*campaigns.CapitalSummary, error) {
				return &campaigns.CapitalSummary{
					OutstandingCents: 100000,
					AlertLevel:       campaigns.AlertWarning,
					WeeksToCover:     8.0,
				}, nil
			},
			portfolioHealthFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
				return nil, fmt.Errorf("health unavailable")
			},
			channelVelocityFn: func(_ context.Context) ([]campaigns.ChannelVelocity, error) {
				return nil, nil
			},
			wantSubstrings: []string{
				`"alertLevel":"warning"`,
				`weeklyReview`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mocks.MockCampaignService{
				GetWeeklyReviewSummaryFn:      tt.weeklyReviewFn,
				GetCapitalSummaryFn:           tt.capitalSummaryFn,
				GetPortfolioHealthFn:          tt.portfolioHealthFn,
				GetPortfolioChannelVelocityFn: tt.channelVelocityFn,
			}
			e := newTestExecutor(svc)

			result, err := e.Execute(context.Background(), "get_dashboard_summary", "{}")
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			for _, want := range tt.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("missing %q in result: %s", want, result)
				}
			}
			for _, unwanted := range tt.wantMissingStrings {
				if strings.Contains(result, unwanted) {
					t.Errorf("unexpected %q in result: %s", unwanted, result)
				}
			}
		})
	}
}
