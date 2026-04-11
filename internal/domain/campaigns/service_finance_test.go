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
			repoErr: errors.New("database boom"),
			wantErr: errors.New("database boom"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mocks.NewMockCampaignRepository()
			if tt.repoErr != nil {
				repoErr := tt.repoErr
				repo.UpdateCashflowConfigFn = func(_ *campaigns.CashflowConfig) error {
					return repoErr
				}
			}
			svc := campaigns.NewService(repo, withTestIDGen(), withClosedBaseCtx())

			err := svc.UpdateCashflowConfig(ctx, tt.cfg)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if errors.Is(tt.wantErr, campaigns.ErrInvalidCashflowConfig) {
					if !errors.Is(err, campaigns.ErrInvalidCashflowConfig) {
						t.Fatalf("expected ErrInvalidCashflowConfig, got %v", err)
					}
				} else if err.Error() != tt.wantErr.Error() {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err)
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
		name                 string
		invoices             []campaigns.Invoice
		recoveryRate30dCents int
		cashBufferCents      int
		want                 campaigns.InvoiceProjection
	}{
		{
			name:                 "no invoices returns zero projection",
			invoices:             nil,
			recoveryRate30dCents: 3_000_000, // $30k / 30d
			cashBufferCents:      500_000,
			want:                 campaigns.InvoiceProjection{},
		},
		{
			name: "all invoices paid returns zero projection",
			invoices: []campaigns.Invoice{
				{ID: "i1", InvoiceDate: "2026-03-31", DueDate: "2026-04-14", TotalCents: 5_000_000, PaidCents: 5_000_000, Status: "paid"},
				{ID: "i2", InvoiceDate: "2026-03-15", DueDate: "2026-03-29", TotalCents: 4_000_000, PaidCents: 4_000_000, Status: "paid"},
			},
			recoveryRate30dCents: 3_000_000,
			cashBufferCents:      500_000,
			want:                 campaigns.InvoiceProjection{},
		},
		{
			name: "gap fully covered by recovery alone",
			invoices: []campaigns.Invoice{
				// Due in 14 days. amount = 4_000_000.
				// daily = 9_000_000/30 = 300_000; projected = 300_000*14 = 4_200_000.
				// gap = max(0, 4_000_000 - 4_200_000 - 0) = 0.
				{ID: "i1", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 4_000_000, PaidCents: 0, Status: "unpaid"},
			},
			recoveryRate30dCents: 9_000_000,
			cashBufferCents:      0,
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 4_000_000,
				DaysUntilInvoiceDue:    14,
				ProjectedRecoveryCents: 4_200_000,
				ProjectedCashGapCents:  0,
			},
		},
		{
			name: "gap covered by recovery plus buffer",
			invoices: []campaigns.Invoice{
				// Due in 10 days. amount = 6_000_000.
				// daily = 9_000_000/30 = 300_000; projected = 3_000_000.
				// recovery + buffer = 3_000_000 + 4_000_000 = 7_000_000 >= 6_000_000.
				// gap = 0.
				{ID: "i1", InvoiceDate: "2026-04-11", DueDate: "2026-04-21", TotalCents: 6_000_000, PaidCents: 0, Status: "unpaid"},
			},
			recoveryRate30dCents: 9_000_000,
			cashBufferCents:      4_000_000,
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-21",
				NextInvoiceAmountCents: 6_000_000,
				DaysUntilInvoiceDue:    10,
				ProjectedRecoveryCents: 3_000_000,
				ProjectedCashGapCents:  0,
			},
		},
		{
			name: "real gap exists when recovery and buffer are not enough",
			invoices: []campaigns.Invoice{
				// Due in 14 days. amount = 6_500_000.
				// daily = 3_000_000/30 = 100_000; projected = 1_400_000.
				// gap = 6_500_000 - 1_400_000 - 500_000 = 4_600_000.
				{ID: "i1", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 6_500_000, PaidCents: 0, Status: "unpaid"},
			},
			recoveryRate30dCents: 3_000_000,
			cashBufferCents:      500_000,
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 6_500_000,
				DaysUntilInvoiceDue:    14,
				ProjectedRecoveryCents: 1_400_000,
				ProjectedCashGapCents:  4_600_000,
			},
		},
		{
			name: "overdue invoice picked with zero projected recovery",
			invoices: []campaigns.Invoice{
				// Due 5 days ago. amount = 2_000_000. daysUntil = 0, projected = 0.
				// gap = max(0, 2_000_000 - 0 - 250_000) = 1_750_000.
				{ID: "i1", InvoiceDate: "2026-03-22", DueDate: "2026-04-06", TotalCents: 2_000_000, PaidCents: 0, Status: "unpaid"},
			},
			recoveryRate30dCents: 9_000_000,
			cashBufferCents:      250_000,
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-03-22",
				NextInvoiceDueDate:     "2026-04-06",
				NextInvoiceAmountCents: 2_000_000,
				DaysUntilInvoiceDue:    0,
				ProjectedRecoveryCents: 0,
				ProjectedCashGapCents:  1_750_000,
			},
		},
		{
			name: "empty due date is skipped, falls through to next candidate",
			invoices: []campaigns.Invoice{
				{ID: "i1", InvoiceDate: "2026-03-31", DueDate: "", TotalCents: 5_000_000, PaidCents: 0, Status: "unpaid"},
				{ID: "i2", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 1_500_000, PaidCents: 0, Status: "unpaid"},
			},
			recoveryRate30dCents: 0,
			cashBufferCents:      0,
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 1_500_000,
				DaysUntilInvoiceDue:    14,
				ProjectedRecoveryCents: 0,
				ProjectedCashGapCents:  1_500_000,
			},
		},
		{
			name: "unparseable due date is skipped, falls through to next candidate",
			invoices: []campaigns.Invoice{
				{ID: "i1", InvoiceDate: "2026-03-31", DueDate: "not-a-date", TotalCents: 5_000_000, PaidCents: 0, Status: "unpaid"},
				{ID: "i2", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 1_200_000, PaidCents: 0, Status: "unpaid"},
			},
			recoveryRate30dCents: 0,
			cashBufferCents:      200_000,
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 1_200_000,
				DaysUntilInvoiceDue:    14,
				ProjectedRecoveryCents: 0,
				ProjectedCashGapCents:  1_000_000,
			},
		},
		{
			name: "earliest unpaid due date wins among multiple candidates",
			invoices: []campaigns.Invoice{
				{ID: "later", InvoiceDate: "2026-04-15", DueDate: "2026-04-29", TotalCents: 8_000_000, PaidCents: 0, Status: "unpaid"},
				{ID: "earlier", InvoiceDate: "2026-04-01", DueDate: "2026-04-15", TotalCents: 3_000_000, PaidCents: 0, Status: "unpaid"},
			},
			recoveryRate30dCents: 0,
			cashBufferCents:      0,
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-01",
				NextInvoiceDueDate:     "2026-04-15",
				NextInvoiceAmountCents: 3_000_000,
				DaysUntilInvoiceDue:    4,
				ProjectedRecoveryCents: 0,
				ProjectedCashGapCents:  3_000_000,
			},
		},
		{
			name: "partial paid_cents reduces amount owed",
			invoices: []campaigns.Invoice{
				// Due in 14 days. paid = 1_000_000, total = 5_000_000 -> owed = 4_000_000.
				// daily = 6_000_000/30 = 200_000; projected = 2_800_000.
				// gap = 4_000_000 - 2_800_000 - 200_000 = 1_000_000.
				{ID: "i1", InvoiceDate: "2026-04-11", DueDate: "2026-04-25", TotalCents: 5_000_000, PaidCents: 1_000_000, Status: "partial"},
			},
			recoveryRate30dCents: 6_000_000,
			cashBufferCents:      200_000,
			want: campaigns.InvoiceProjection{
				NextInvoiceDate:        "2026-04-11",
				NextInvoiceDueDate:     "2026-04-25",
				NextInvoiceAmountCents: 4_000_000,
				DaysUntilInvoiceDue:    14,
				ProjectedRecoveryCents: 2_800_000,
				ProjectedCashGapCents:  1_000_000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := campaigns.ComputeInvoiceProjection(tt.invoices, tt.recoveryRate30dCents, tt.cashBufferCents, now)
			if got != tt.want {
				t.Errorf("ComputeInvoiceProjection() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestService_GetCapitalSummary_WithProjection(t *testing.T) {
	ctx := context.Background()

	t.Run("populates projection fields from invoices and cashflow config", func(t *testing.T) {
		repo := mocks.NewMockCampaignRepository()
		repo.GetCapitalRawDataFn = func(_ context.Context) (*campaigns.CapitalRawData, error) {
			return &campaigns.CapitalRawData{
				OutstandingCents:     12_000_000,
				RecoveryRate30dCents: 9_000_000,
				PaidCents:            5_000_000,
				UnpaidInvoiceCount:   1,
			}, nil
		}
		dueDate := time.Now().Add(14 * 24 * time.Hour).Format("2006-01-02")
		repo.Invoices["i1"] = &campaigns.Invoice{
			ID:          "i1",
			InvoiceDate: time.Now().Format("2006-01-02"),
			DueDate:     dueDate,
			TotalCents:  6_500_000,
			PaidCents:   0,
			Status:      "unpaid",
		}
		repo.CashflowConfig = &campaigns.CashflowConfig{
			CapitalBudgetCents: 20_000_000,
			CashBufferCents:    500_000,
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
		// daysUntilDue can be 13 or 14 depending on time-of-day truncation; both are valid.
		if summary.DaysUntilInvoiceDue != 13 && summary.DaysUntilInvoiceDue != 14 {
			t.Errorf("DaysUntilInvoiceDue = %d, want 13 or 14", summary.DaysUntilInvoiceDue)
		}
		if summary.CashBufferCents != 500_000 {
			t.Errorf("CashBufferCents = %d, want 500000", summary.CashBufferCents)
		}
		if summary.ProjectedRecoveryCents <= 0 {
			t.Errorf("ProjectedRecoveryCents = %d, want > 0", summary.ProjectedRecoveryCents)
		}
		// Existing summary fields still flow through.
		if summary.OutstandingCents != 12_000_000 {
			t.Errorf("OutstandingCents = %d, want 12000000", summary.OutstandingCents)
		}
		if summary.RecoveryRate30dCents != 9_000_000 {
			t.Errorf("RecoveryRate30dCents = %d, want 9000000", summary.RecoveryRate30dCents)
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
		repo.CashflowConfig = &campaigns.CashflowConfig{CashBufferCents: 100_000}
		svc := campaigns.NewService(repo, withTestIDGen(), withClosedBaseCtx())

		summary, err := svc.GetCapitalSummary(ctx)
		if err != nil {
			t.Fatalf("GetCapitalSummary: %v", err)
		}
		if summary.NextInvoiceDate != "" || summary.NextInvoiceDueDate != "" {
			t.Errorf("expected empty next invoice fields, got %+v", summary)
		}
		if summary.NextInvoiceAmountCents != 0 || summary.ProjectedCashGapCents != 0 || summary.DaysUntilInvoiceDue != 0 {
			t.Errorf("expected zero projection values, got %+v", summary)
		}
		if summary.CashBufferCents != 100_000 {
			t.Errorf("CashBufferCents = %d, want 100000", summary.CashBufferCents)
		}
	})
}
