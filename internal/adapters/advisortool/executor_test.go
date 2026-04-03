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
		"get_campaign_tuning", "get_inventory_aging", "get_global_inventory",
		"get_sell_sheet", "get_portfolio_health", "get_portfolio_insights",
		"get_credit_summary", "get_weekly_review", "get_capital_timeline",
		"get_channel_velocity", "get_dashboard_summary", "get_expected_values",
		"get_crack_candidates", "get_campaign_suggestions", "run_projection",
		"suggest_price", "get_cert_lookup", "get_suggestion_stats",
		"evaluate_purchase",
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

func TestExecute_GetExpectedValuesBatch_WithIDs(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetExpectedValuesFn: func(_ context.Context, campaignID string) (*campaigns.EVPortfolio, error) {
			return &campaigns.EVPortfolio{
				TotalEVCents:  1000,
				PositiveCount: 2,
				Items: []campaigns.ExpectedValue{
					{CardName: "Charizard-" + campaignID, EVCents: 500},
				},
			}, nil
		},
	}
	e := newTestExecutor(svc)
	result, err := e.Execute(context.Background(), "get_expected_values_batch", `{"campaignIds":["camp-1","camp-2"]}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
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
}

func TestExecute_GetExpectedValuesBatch_AllActive(t *testing.T) {
	svc := &mocks.MockCampaignService{
		ListCampaignsFn: func(_ context.Context, activeOnly bool) ([]campaigns.Campaign, error) {
			if !activeOnly {
				t.Error("expected activeOnly=true")
			}
			return []campaigns.Campaign{
				{ID: "a1", Name: "Alpha"},
				{ID: "a2", Name: "Beta"},
			}, nil
		},
		GetExpectedValuesFn: func(_ context.Context, campaignID string) (*campaigns.EVPortfolio, error) {
			return &campaigns.EVPortfolio{TotalEVCents: 100}, nil
		},
	}
	e := newTestExecutor(svc)
	result, err := e.Execute(context.Background(), "get_expected_values_batch", `{}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
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
}

func TestExecute_GetExpectedValuesBatch_PartialFailure(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetExpectedValuesFn: func(_ context.Context, campaignID string) (*campaigns.EVPortfolio, error) {
			if campaignID == "bad" {
				return nil, errors.New("not found")
			}
			return &campaigns.EVPortfolio{TotalEVCents: 200}, nil
		},
	}
	e := newTestExecutor(svc)
	result, err := e.Execute(context.Background(), "get_expected_values_batch", `{"campaignIds":["good","bad"]}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, `"good"`) {
		t.Errorf("result missing 'good' campaign: %s", result)
	}
	if !strings.Contains(result, `"error"`) {
		t.Errorf("result missing error for 'bad' campaign: %s", result)
	}
}

func TestExecute_GetDashboardSummary(t *testing.T) {
	svc := &mocks.MockCampaignService{
		GetWeeklyReviewSummaryFn: func(_ context.Context) (*campaigns.WeeklyReviewSummary, error) {
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
		GetCreditSummaryFn: func(_ context.Context) (*campaigns.CreditSummary, error) {
			return &campaigns.CreditSummary{
				CreditLimitCents:  5000000,
				OutstandingCents:  2500000,
				UtilizationPct:    50.0,
				AlertLevel:        "ok",
				DaysToNextInvoice: 7,
			}, nil
		},
		GetPortfolioHealthFn: func(_ context.Context) (*campaigns.PortfolioHealth, error) {
			return &campaigns.PortfolioHealth{
				Campaigns: []campaigns.CampaignHealth{
					{CampaignName: "Test", HealthStatus: "healthy", HealthReason: "good", CapitalAtRisk: 1000},
				},
			}, nil
		},
		GetPortfolioChannelVelocityFn: func(_ context.Context) ([]campaigns.ChannelVelocity, error) {
			return []campaigns.ChannelVelocity{
				{Channel: "ebay", AvgDaysToSell: 14.5, SaleCount: 5},
			}, nil
		},
	}
	e := newTestExecutor(svc)

	result, err := e.Execute(context.Background(), "get_dashboard_summary", "{}")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(result, `"purchaseCount":10`) {
		t.Errorf("missing purchaseCount in result: %s", result)
	}
	if !strings.Contains(result, `"alertLevel":"ok"`) {
		t.Errorf("missing alertLevel in result: %s", result)
	}
	if !strings.Contains(result, `"purchaseCountWoW":2`) {
		t.Errorf("missing WoW delta in result: %s", result)
	}
}
