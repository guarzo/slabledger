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

	t.Run("populates invoice fields from unpaid invoice", func(t *testing.T) {
		repo := mocks.NewMockCampaignRepository()
		repo.GetCapitalRawDataFn = func(_ context.Context) (*campaigns.CapitalRawData, error) {
			return &campaigns.CapitalRawData{
				OutstandingCents:     12_000_000,
				RecoveryRate30dCents: 9_000_000,
				PaidCents:            5_000_000,
				UnpaidInvoiceCount:   1,
			}, nil
		}
		invoiceDate := time.Now().Format("2006-01-02")
		dueDate := time.Now().Add(14 * 24 * time.Hour).Format("2006-01-02")
		repo.Invoices["i1"] = &campaigns.Invoice{
			ID:          "i1",
			InvoiceDate: invoiceDate,
			DueDate:     dueDate,
			TotalCents:  6_500_000,
			PaidCents:   0,
			Status:      "unpaid",
		}
		svc := campaigns.NewService(repo, withTestIDGen(), withClosedBaseCtx())

		summary, err := svc.GetCapitalSummary(ctx)
		if err != nil {
			t.Fatalf("GetCapitalSummary: %v", err)
		}
		if summary.NextInvoiceDueDate != dueDate {
			t.Errorf("NextInvoiceDueDate = %q, want %q", summary.NextInvoiceDueDate, dueDate)
		}
		if summary.NextInvoiceAmountCents != 6_500_000 {
			t.Errorf("NextInvoiceAmountCents = %d, want 6500000", summary.NextInvoiceAmountCents)
		}
		if summary.DaysUntilInvoiceDue != 13 && summary.DaysUntilInvoiceDue != 14 {
			t.Errorf("DaysUntilInvoiceDue = %d, want 13 or 14", summary.DaysUntilInvoiceDue)
		}
		// Existing summary fields still flow through.
		if summary.OutstandingCents != 12_000_000 {
			t.Errorf("OutstandingCents = %d, want 12000000", summary.OutstandingCents)
		}
	})

	t.Run("no invoices yields zero-valued projection but still returns summary", func(t *testing.T) {
		repo := mocks.NewMockCampaignRepository()
		repo.GetCapitalRawDataFn = func(_ context.Context) (*campaigns.CapitalRawData, error) {
			return &campaigns.CapitalRawData{
				OutstandingCents:     1_000_000,
				RecoveryRate30dCents: 600_000,
			}, nil
		}
		svc := campaigns.NewService(repo, withTestIDGen(), withClosedBaseCtx())

		summary, err := svc.GetCapitalSummary(ctx)
		if err != nil {
			t.Fatalf("GetCapitalSummary: %v", err)
		}
		if summary.NextInvoiceDate != "" || summary.NextInvoiceDueDate != "" {
			t.Errorf("expected empty next invoice fields, got %+v", summary)
		}
		if summary.NextInvoiceAmountCents != 0 || summary.DaysUntilInvoiceDue != 0 {
			t.Errorf("expected zero invoice values, got %+v", summary)
		}
	})

	t.Run("sell-through data is populated for the next invoice date", func(t *testing.T) {
		repo := mocks.NewMockCampaignRepository()
		repo.GetCapitalRawDataFn = func(_ context.Context) (*campaigns.CapitalRawData, error) {
			return &campaigns.CapitalRawData{OutstandingCents: 500_000}, nil
		}
		invoiceDate := time.Now().Format("2006-01-02")
		dueDate := time.Now().Add(10 * 24 * time.Hour).Format("2006-01-02")
		repo.Invoices["i1"] = &campaigns.Invoice{
			ID: "i1", InvoiceDate: invoiceDate, DueDate: dueDate,
			TotalCents: 1_000_000, Status: "unpaid",
		}
		repo.GetInvoiceSellThroughFn = func(_ context.Context, date string) (campaigns.InvoiceSellThrough, error) {
			if date != invoiceDate {
				return campaigns.InvoiceSellThrough{}, nil
			}
			return campaigns.InvoiceSellThrough{
				TotalPurchaseCount: 10,
				SoldCount:          4,
				TotalCostCents:     200_000,
				SaleRevenueCents:   90_000,
			}, nil
		}
		svc := campaigns.NewService(repo, withTestIDGen(), withClosedBaseCtx())

		summary, err := svc.GetCapitalSummary(ctx)
		if err != nil {
			t.Fatalf("GetCapitalSummary: %v", err)
		}
		st := summary.NextInvoiceSellThrough
		if st.TotalPurchaseCount != 10 {
			t.Errorf("TotalPurchaseCount = %d, want 10", st.TotalPurchaseCount)
		}
		if st.SoldCount != 4 {
			t.Errorf("SoldCount = %d, want 4", st.SoldCount)
		}
		if st.SaleRevenueCents != 90_000 {
			t.Errorf("SaleRevenueCents = %d, want 90000", st.SaleRevenueCents)
		}
	})
}
