package campaigns_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestService_UpdateCashflowConfig(t *testing.T) {
	ctx := context.Background()
	// Shared sentinel so the assertion can use errors.Is against the exact
	// error instance the mock returns — errors.Is on distinct errors.New
	// values would fall back to identity comparison and fail.
	repoBoom := errors.New("database boom")

	tests := []struct {
		name      string
		cfg       *campaigns.CashflowConfig
		repoErr   error
		wantErr   error
		checkRepo func(t *testing.T, repo *mocks.MockCampaignRepository)
	}{
		{
			name:    "nil config returns ErrInvalidCashflowConfig",
			cfg:     nil,
			wantErr: campaigns.ErrInvalidCashflowConfig,
		},
		{
			name:    "negative capital budget returns ErrInvalidCashflowConfig",
			cfg:     &campaigns.CashflowConfig{CapitalBudgetCents: -1, CashBufferCents: 0},
			wantErr: campaigns.ErrInvalidCashflowConfig,
		},
		{
			name:    "negative cash buffer returns ErrInvalidCashflowConfig",
			cfg:     &campaigns.CashflowConfig{CapitalBudgetCents: 0, CashBufferCents: -1},
			wantErr: campaigns.ErrInvalidCashflowConfig,
		},
		{
			name: "valid update persists and sets UpdatedAt",
			cfg:  &campaigns.CashflowConfig{CapitalBudgetCents: 5_000_000, CashBufferCents: 500_000},
			checkRepo: func(t *testing.T, repo *mocks.MockCampaignRepository) {
				t.Helper()
				if repo.CashflowConfig == nil {
					t.Fatal("expected CashflowConfig to be set on repo")
				}
				if repo.CashflowConfig.CapitalBudgetCents != 5_000_000 {
					t.Errorf("CapitalBudgetCents = %d, want 5000000", repo.CashflowConfig.CapitalBudgetCents)
				}
				if repo.CashflowConfig.CashBufferCents != 500_000 {
					t.Errorf("CashBufferCents = %d, want 500000", repo.CashflowConfig.CashBufferCents)
				}
				if repo.CashflowConfig.UpdatedAt.IsZero() {
					t.Error("expected UpdatedAt to be set")
				}
			},
		},
		{
			name:    "repo error propagates",
			cfg:     &campaigns.CashflowConfig{CapitalBudgetCents: 1, CashBufferCents: 1},
			repoErr: repoBoom,
			wantErr: repoBoom, // same instance so errors.Is passes even if the service wraps
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewMockCampaignRepository()
			if tt.repoErr != nil {
				repoErr := tt.repoErr
				repo.UpdateCashflowConfigFn = func(_ context.Context, _ *campaigns.CashflowConfig) error {
					return repoErr
				}
			}
			svc := campaigns.NewService(repo, withTestIDGen(), withClosedBaseCtx())

			err := svc.UpdateCashflowConfig(ctx, tt.cfg)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error wrapping %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkRepo != nil {
				tt.checkRepo(t, repo)
			}
		})
	}
}

func TestComputeInvoiceProjection(t *testing.T) {
	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		invoices []campaigns.Invoice
		want     campaigns.InvoiceProjection
	}{
		{
			name:     "no invoices returns zero projection",
			invoices: nil,
			want:     campaigns.InvoiceProjection{},
		},
		{
			name: "all invoices paid returns zero projection",
			invoices: []campaigns.Invoice{
				{ID: "i1", InvoiceDate: "2026-03-31", DueDate: "2026-04-14", TotalCents: 5_000_000, PaidCents: 5_000_000, Status: "paid"},
				{ID: "i2", InvoiceDate: "2026-03-15", DueDate: "2026-03-29", TotalCents: 4_000_000, PaidCents: 4_000_000, Status: "paid"},
			},
			want: campaigns.InvoiceProjection{},
		},
		{
			name: "picks first unpaid invoice with parseable due date",
			invoices: []campaigns.Invoice{
				// Due in 14 days. amount = 4_000_000.
				{ID: "i1", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 4_000_000, PaidCents: 0, Status: "unpaid"},
			},
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 4_000_000,
				DaysUntilInvoiceDue:    14,
			},
		},
		{
			name: "picks unpaid invoice over paid one",
			invoices: []campaigns.Invoice{
				// Due in 10 days. amount = 6_000_000.
				{ID: "i1", InvoiceDate: "2026-04-11", DueDate: "2026-04-21", TotalCents: 6_000_000, PaidCents: 0, Status: "unpaid"},
			},
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-21",
				NextInvoiceAmountCents: 6_000_000,
				DaysUntilInvoiceDue:    10,
			},
		},
		{
			name: "handles amount owed calculation when fully unpaid",
			invoices: []campaigns.Invoice{
				// Due in 14 days. amount = 6_500_000.
				{ID: "i1", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 6_500_000, PaidCents: 0, Status: "unpaid"},
			},
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 6_500_000,
				DaysUntilInvoiceDue:    14,
			},
		},
		{
			name: "overdue invoice reports negative daysUntilDue",
			invoices: []campaigns.Invoice{
				// Due 5 days ago (now=2026-04-11, due=2026-04-06). amount = 2_000_000.
				// daysUntil = -5 (UI renders "5 days overdue").
				{ID: "i1", InvoiceDate: "2026-03-22", DueDate: "2026-04-06", TotalCents: 2_000_000, PaidCents: 0, Status: "unpaid"},
			},
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-03-22",
				NextInvoiceDueDate:     "2026-04-06",
				NextInvoiceAmountCents: 2_000_000,
				DaysUntilInvoiceDue:    -5,
			},
		},
		{
			name: "empty due date is skipped, falls through to next candidate",
			invoices: []campaigns.Invoice{
				{ID: "i1", InvoiceDate: "2026-03-31", DueDate: "", TotalCents: 5_000_000, PaidCents: 0, Status: "unpaid"},
				{ID: "i2", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 1_500_000, PaidCents: 0, Status: "unpaid"},
			},
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 1_500_000,
				DaysUntilInvoiceDue:    14,
			},
		},
		{
			name: "unparseable due date is skipped, falls through to next candidate",
			invoices: []campaigns.Invoice{
				{ID: "i1", InvoiceDate: "2026-03-31", DueDate: "not-a-date", TotalCents: 5_000_000, PaidCents: 0, Status: "unpaid"},
				{ID: "i2", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 1_200_000, PaidCents: 0, Status: "unpaid"},
			},
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 1_200_000,
				DaysUntilInvoiceDue:    14,
			},
		},
		{
			name: "earliest unpaid due date wins among multiple candidates",
			invoices: []campaigns.Invoice{
				{ID: "later", InvoiceDate: "2026-04-15", DueDate: "2026-04-29", TotalCents: 8_000_000, PaidCents: 0, Status: "unpaid"},
				{ID: "earlier", InvoiceDate: "2026-04-01", DueDate: "2026-04-15", TotalCents: 3_000_000, PaidCents: 0, Status: "unpaid"},
			},
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-01",
				NextInvoiceDueDate:     "2026-04-15",
				NextInvoiceAmountCents: 3_000_000,
				DaysUntilInvoiceDue:    4,
			},
		},
		{
			name: "partial paid_cents reduces amount owed",
			invoices: []campaigns.Invoice{
				// Due in 14 days. paid = 1_000_000, total = 5_000_000 -> owed = 4_000_000.
				{ID: "i1", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 5_000_000, PaidCents: 1_000_000, Status: "partial"},
			},
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 4_000_000,
				DaysUntilInvoiceDue:    14,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := campaigns.ComputeInvoiceProjection(tt.invoices, now)
			if got != tt.want {
				t.Errorf("ComputeInvoiceProjection() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestService_GetCapitalSummary_WithProjection(t *testing.T) {
	ctx := context.Background()

	// Fixed UTC reference date so DaysUntilInvoiceDue assertions are deterministic.
	// The service calls ComputeInvoiceProjection(invoices, time.Now()), so we cannot
	// inject a clock; instead we use dates relative to today-UTC and assert exact values.
	todayUTC := time.Now().UTC().Truncate(24 * time.Hour)
	fmtDate := func(d time.Time) string { return d.Format("2006-01-02") }

	tests := []struct {
		name string
		// Per-test repo setup
		rawData    *campaigns.CapitalRawData
		invoice    *campaigns.Invoice // nil = no invoice added
		sellThruFn func(context.Context, string) (campaigns.InvoiceSellThrough, error)
		// Expected fields on the returned summary.
		// checkNextInvoiceDate: true = assert NextInvoiceDate == wantNextInvoiceDate
		// (allows distinguishing "expect empty" from "don't care").
		checkNextInvoiceDate   bool
		wantNextInvoiceDate    string
		wantNextInvoiceDueDate string
		wantAmountCents        int
		wantDaysUntilDue       int
		wantOutstandingCents   int
		wantSellThru           campaigns.InvoiceSellThrough
	}{
		{
			name: "populates invoice fields from unpaid invoice",
			rawData: &campaigns.CapitalRawData{
				OutstandingCents:     12_000_000,
				RecoveryRate30dCents: 9_000_000,
				PaidCents:            5_000_000,
				UnpaidInvoiceCount:   1,
			},
			invoice: &campaigns.Invoice{
				ID:          "i1",
				InvoiceDate: fmtDate(todayUTC),
				DueDate:     fmtDate(todayUTC.Add(14 * 24 * time.Hour)),
				TotalCents:  6_500_000,
				PaidCents:   0,
				Status:      "unpaid",
			},
			checkNextInvoiceDate:   true,
			wantNextInvoiceDate:    fmtDate(todayUTC),
			wantNextInvoiceDueDate: fmtDate(todayUTC.Add(14 * 24 * time.Hour)),
			wantAmountCents:        6_500_000,
			wantDaysUntilDue:       14,
			wantOutstandingCents:   12_000_000,
		},
		{
			name: "no invoices yields zero-valued projection but still returns summary",
			rawData: &campaigns.CapitalRawData{
				OutstandingCents:     1_000_000,
				RecoveryRate30dCents: 600_000,
			},
			invoice:              nil,
			checkNextInvoiceDate: true,
			wantNextInvoiceDate:  "",
			wantAmountCents:      0,
			wantDaysUntilDue:     0,
			wantOutstandingCents: 1_000_000,
		},
		{
			name:    "sell-through data is populated for the next invoice date",
			rawData: &campaigns.CapitalRawData{OutstandingCents: 500_000},
			invoice: &campaigns.Invoice{
				ID:          "i1",
				InvoiceDate: fmtDate(todayUTC),
				DueDate:     fmtDate(todayUTC.Add(10 * 24 * time.Hour)),
				TotalCents:  1_000_000,
				Status:      "unpaid",
			},
			sellThruFn: func(_ context.Context, date string) (campaigns.InvoiceSellThrough, error) {
				if date != fmtDate(todayUTC) {
					return campaigns.InvoiceSellThrough{}, nil
				}
				return campaigns.InvoiceSellThrough{
					TotalPurchaseCount: 10,
					SoldCount:          4,
					TotalCostCents:     200_000,
					SaleRevenueCents:   90_000,
				}, nil
			},
			wantDaysUntilDue: 10,
			wantAmountCents:  1_000_000,
			wantSellThru: campaigns.InvoiceSellThrough{
				TotalPurchaseCount: 10,
				SoldCount:          4,
				TotalCostCents:     200_000,
				SaleRevenueCents:   90_000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewMockCampaignRepository()
			rawData := tt.rawData
			repo.GetCapitalRawDataFn = func(_ context.Context) (*campaigns.CapitalRawData, error) {
				return rawData, nil
			}
			if tt.invoice != nil {
				repo.Invoices[tt.invoice.ID] = tt.invoice
			}
			if tt.sellThruFn != nil {
				repo.GetInvoiceSellThroughFn = tt.sellThruFn
			}
			svc := campaigns.NewService(repo, withTestIDGen(), withClosedBaseCtx())

			summary, err := svc.GetCapitalSummary(ctx)
			if err != nil {
				t.Fatalf("GetCapitalSummary: %v", err)
			}
			if tt.checkNextInvoiceDate && summary.NextInvoiceDate != tt.wantNextInvoiceDate {
				t.Errorf("NextInvoiceDate = %q, want %q", summary.NextInvoiceDate, tt.wantNextInvoiceDate)
			}
			if tt.wantNextInvoiceDueDate != "" && summary.NextInvoiceDueDate != tt.wantNextInvoiceDueDate {
				t.Errorf("NextInvoiceDueDate = %q, want %q", summary.NextInvoiceDueDate, tt.wantNextInvoiceDueDate)
			}
			if summary.NextInvoiceAmountCents != tt.wantAmountCents {
				t.Errorf("NextInvoiceAmountCents = %d, want %d", summary.NextInvoiceAmountCents, tt.wantAmountCents)
			}
			if tt.wantDaysUntilDue != 0 && summary.DaysUntilInvoiceDue != tt.wantDaysUntilDue {
				t.Errorf("DaysUntilInvoiceDue = %d, want %d", summary.DaysUntilInvoiceDue, tt.wantDaysUntilDue)
			}
			if tt.wantOutstandingCents != 0 && summary.OutstandingCents != tt.wantOutstandingCents {
				t.Errorf("OutstandingCents = %d, want %d", summary.OutstandingCents, tt.wantOutstandingCents)
			}
			if tt.wantSellThru != (campaigns.InvoiceSellThrough{}) {
				st := summary.NextInvoiceSellThrough
				if st != tt.wantSellThru {
					t.Errorf("NextInvoiceSellThrough = %+v, want %+v", st, tt.wantSellThru)
				}
			}
		})
	}
}
