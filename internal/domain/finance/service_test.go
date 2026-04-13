package finance_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/domain/finance"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

// fixedID returns a deterministic ID generator for testing.
func fixedID(id string) func() string { return func() string { return id } }

// newFinanceSvc creates a finance.Service with a FinanceRepositoryMock.
// When GetLatestRevocationFlagFn is not set, it defaults to returning nil, nil
// (no recent flag) so FlagForRevocation tests proceed without a rate-limit error.
func newFinanceSvc(repo *mocks.FinanceRepositoryMock) finance.Service {
	if repo.GetLatestRevocationFlagFn == nil {
		repo.GetLatestRevocationFlagFn = func(_ context.Context) (*inventory.RevocationFlag, error) {
			return nil, nil
		}
	}
	return finance.New(repo, fixedID("generated-id"))
}

// --- ListInvoices ---

func TestFinanceService_ListInvoices(t *testing.T) {
	tests := []struct {
		name        string
		listFn      func(context.Context) ([]inventory.Invoice, error)
		pendingFn   func(context.Context, []string) (map[string]int, error)
		wantLen     int
		wantErr     bool
		wantPending int
	}{
		{
			name: "success — single invoice with pending receipt",
			listFn: func(_ context.Context) ([]inventory.Invoice, error) {
				return []inventory.Invoice{
					{ID: "inv-1", InvoiceDate: "2026-01-01", TotalCents: 10000},
				}, nil
			},
			pendingFn: func(_ context.Context, _ []string) (map[string]int, error) {
				return map[string]int{"2026-01-01": 500}, nil
			},
			wantLen:     1,
			wantPending: 500,
		},
		{
			name: "success — empty list",
			listFn: func(_ context.Context) ([]inventory.Invoice, error) {
				return []inventory.Invoice{}, nil
			},
			wantLen: 0,
		},
		{
			name: "repo error on ListInvoices",
			listFn: func(_ context.Context) ([]inventory.Invoice, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
		{
			name: "repo error on GetPendingReceiptByInvoiceDate",
			listFn: func(_ context.Context) ([]inventory.Invoice, error) {
				return []inventory.Invoice{
					{ID: "inv-2", InvoiceDate: "2026-02-01", TotalCents: 5000},
				}, nil
			},
			pendingFn: func(_ context.Context, _ []string) (map[string]int, error) {
				return nil, errors.New("pending error")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.FinanceRepositoryMock{}
			repo.ListInvoicesFn = tc.listFn
			if tc.pendingFn != nil {
				repo.GetPendingReceiptByInvoiceDateFn = tc.pendingFn
			}
			svc := newFinanceSvc(repo)

			invoices, err := svc.ListInvoices(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(invoices) != tc.wantLen {
				t.Errorf("expected %d invoices, got %d", tc.wantLen, len(invoices))
			}
			if tc.wantPending > 0 && len(invoices) > 0 {
				if invoices[0].PendingReceiptCents != tc.wantPending {
					t.Errorf("expected PendingReceiptCents=%d, got %d", tc.wantPending, invoices[0].PendingReceiptCents)
				}
			}
		})
	}
}

// --- UpdateInvoice ---

func TestFinanceService_UpdateInvoice(t *testing.T) {
	tests := []struct {
		name     string
		updateFn func(context.Context, *inventory.Invoice) error
		wantErr  bool
	}{
		{
			name: "success",
			updateFn: func(_ context.Context, inv *inventory.Invoice) error {
				if inv.ID != "inv-1" {
					return errors.New("unexpected ID")
				}
				return nil
			},
		},
		{
			name: "repo error propagated",
			updateFn: func(_ context.Context, _ *inventory.Invoice) error {
				return errors.New("update failed")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.FinanceRepositoryMock{}
			repo.UpdateInvoiceFn = tc.updateFn
			svc := newFinanceSvc(repo)

			err := svc.UpdateInvoice(context.Background(), &inventory.Invoice{ID: "inv-1"})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- GetCapitalSummary ---

func TestFinanceService_GetCapitalSummary(t *testing.T) {
	tests := []struct {
		name       string
		rawDataFn  func(context.Context) (*inventory.CapitalRawData, error)
		listFn     func(context.Context) ([]inventory.Invoice, error)
		wantErr    bool
		wantNonNil bool
	}{
		{
			name: "success — nil raw data returns safe default",
			rawDataFn: func(_ context.Context) (*inventory.CapitalRawData, error) {
				return nil, nil
			},
			listFn: func(_ context.Context) ([]inventory.Invoice, error) {
				return []inventory.Invoice{}, nil
			},
			wantNonNil: true,
		},
		{
			name: "success — populated raw data",
			rawDataFn: func(_ context.Context) (*inventory.CapitalRawData, error) {
				return &inventory.CapitalRawData{
					OutstandingCents:          50000,
					RecoveryRate30dCents:      10000,
					RecoveryRate30dPriorCents: 8000,
				}, nil
			},
			listFn: func(_ context.Context) ([]inventory.Invoice, error) {
				return []inventory.Invoice{}, nil
			},
			wantNonNil: true,
		},
		{
			name: "error — GetCapitalRawData fails",
			rawDataFn: func(_ context.Context) (*inventory.CapitalRawData, error) {
				return nil, errors.New("raw data error")
			},
			wantErr: true,
		},
		{
			name: "error — ListInvoices fails",
			rawDataFn: func(_ context.Context) (*inventory.CapitalRawData, error) {
				return &inventory.CapitalRawData{}, nil
			},
			listFn: func(_ context.Context) ([]inventory.Invoice, error) {
				return nil, errors.New("list invoices error")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.FinanceRepositoryMock{}
			repo.GetCapitalRawDataFn = tc.rawDataFn
			if tc.listFn != nil {
				repo.ListInvoicesFn = tc.listFn
			}
			svc := newFinanceSvc(repo)

			summary, err := svc.GetCapitalSummary(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantNonNil && summary == nil {
				t.Fatal("expected non-nil summary, got nil")
			}
		})
	}
}

// --- FlagForRevocation ---

func TestFinanceService_FlagForRevocation(t *testing.T) {
	tests := []struct {
		name         string
		latestFlagFn func(context.Context) (*inventory.RevocationFlag, error)
		createFlagFn func(context.Context, *inventory.RevocationFlag) error
		segmentLabel string
		segmentDim   string
		reason       string
		wantErr      bool
		wantErrIs    error
		wantID       string
	}{
		{
			name:         "success — no existing flag",
			segmentLabel: "segment-A",
			segmentDim:   "dimension-X",
			reason:       "over-budget",
			wantID:       "generated-id",
		},
		{
			name: "error — GetLatestRevocationFlag returns error",
			latestFlagFn: func(_ context.Context) (*inventory.RevocationFlag, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
		{
			name: "error — CreateRevocationFlag returns error",
			createFlagFn: func(_ context.Context, _ *inventory.RevocationFlag) error {
				return errors.New("create failed")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedFlag *inventory.RevocationFlag
			repo := &mocks.FinanceRepositoryMock{}

			if tc.latestFlagFn != nil {
				repo.GetLatestRevocationFlagFn = tc.latestFlagFn
			} else {
				repo.GetLatestRevocationFlagFn = func(_ context.Context) (*inventory.RevocationFlag, error) {
					return nil, nil
				}
			}
			if tc.createFlagFn != nil {
				repo.CreateRevocationFlagFn = tc.createFlagFn
			} else {
				repo.CreateRevocationFlagFn = func(_ context.Context, flag *inventory.RevocationFlag) error {
					capturedFlag = flag
					return nil
				}
			}
			svc := finance.New(repo, fixedID("generated-id"))

			flag, err := svc.FlagForRevocation(context.Background(), tc.segmentLabel, tc.segmentDim, tc.reason)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Errorf("expected error %v, got %v", tc.wantErrIs, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantID != "" && flag.ID != tc.wantID {
				t.Errorf("expected flag ID=%q, got %q", tc.wantID, flag.ID)
			}
			if capturedFlag != nil {
				if capturedFlag.SegmentLabel != tc.segmentLabel {
					t.Errorf("expected SegmentLabel=%q, got %q", tc.segmentLabel, capturedFlag.SegmentLabel)
				}
				if capturedFlag.SegmentDimension != tc.segmentDim {
					t.Errorf("expected SegmentDimension=%q, got %q", tc.segmentDim, capturedFlag.SegmentDimension)
				}
				if capturedFlag.Reason != tc.reason {
					t.Errorf("expected Reason=%q, got %q", tc.reason, capturedFlag.Reason)
				}
			}
		})
	}
}

// TestFinanceService_FlagForRevocation_TooSoon verifies ErrRevocationTooSoon is
// returned when a flag was created within the last 7 days.
func TestFinanceService_FlagForRevocation_TooSoon(t *testing.T) {
	repo := &mocks.FinanceRepositoryMock{}
	repo.GetLatestRevocationFlagFn = func(_ context.Context) (*inventory.RevocationFlag, error) {
		return &inventory.RevocationFlag{
			ID:        "existing-flag",
			CreatedAt: time.Now().Add(-1 * time.Hour), // 1 hour ago — within 7-day window
		}, nil
	}
	svc := finance.New(repo, fixedID("new-id"))

	_, err := svc.FlagForRevocation(context.Background(), "seg", "dim", "reason")
	if err == nil {
		t.Fatal("expected ErrRevocationTooSoon, got nil")
	}
	if !errors.Is(err, inventory.ErrRevocationTooSoon) {
		t.Errorf("expected ErrRevocationTooSoon, got %v", err)
	}
}

// --- GenerateRevocationEmail ---

func TestFinanceService_GenerateRevocationEmail(t *testing.T) {
	tests := []struct {
		name       string
		flagByIDFn func(context.Context, string) (*inventory.RevocationFlag, error)
		flagID     string
		wantErr    bool
		wantSubstr string
	}{
		{
			name: "success — valid flag",
			flagByIDFn: func(_ context.Context, id string) (*inventory.RevocationFlag, error) {
				return &inventory.RevocationFlag{
					ID:               id,
					SegmentLabel:     "Vintage Holos",
					SegmentDimension: "set",
					Reason:           "over-budget",
				}, nil
			},
			flagID:     "flag-1",
			wantSubstr: "Vintage Holos",
		},
		{
			name: "error — flag not found (nil return)",
			flagByIDFn: func(_ context.Context, _ string) (*inventory.RevocationFlag, error) {
				return nil, nil
			},
			flagID:  "missing-flag",
			wantErr: true,
		},
		{
			name: "error — repo error propagated",
			flagByIDFn: func(_ context.Context, _ string) (*inventory.RevocationFlag, error) {
				return nil, errors.New("db error")
			},
			flagID:  "flag-2",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.FinanceRepositoryMock{}
			repo.GetRevocationFlagByIDFn = tc.flagByIDFn
			svc := newFinanceSvc(repo)

			email, err := svc.GenerateRevocationEmail(context.Background(), tc.flagID)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantSubstr != "" && !strings.Contains(email, tc.wantSubstr) {
				t.Errorf("expected email to contain %q, got: %s", tc.wantSubstr, email)
			}
		})
	}
}

// --- UpdateCashflowConfig ---

func TestFinanceService_UpdateCashflowConfig(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *inventory.CashflowConfig
		updateFn func(context.Context, *inventory.CashflowConfig) error
		wantErr  bool
	}{
		{
			name: "success",
			cfg:  &inventory.CashflowConfig{CapitalBudgetCents: 100000, CashBufferCents: 5000},
		},
		{
			name:    "error — nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name:    "error — negative CapitalBudgetCents",
			cfg:     &inventory.CashflowConfig{CapitalBudgetCents: -1},
			wantErr: true,
		},
		{
			name:    "error — negative CashBufferCents",
			cfg:     &inventory.CashflowConfig{CashBufferCents: -1},
			wantErr: true,
		},
		{
			name: "error — repo error propagated",
			cfg:  &inventory.CashflowConfig{CapitalBudgetCents: 50000},
			updateFn: func(_ context.Context, _ *inventory.CashflowConfig) error {
				return errors.New("persist failed")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mocks.FinanceRepositoryMock{}
			if tc.updateFn != nil {
				repo.UpdateCashflowConfigFn = tc.updateFn
			}
			svc := newFinanceSvc(repo)

			err := svc.UpdateCashflowConfig(context.Background(), tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
