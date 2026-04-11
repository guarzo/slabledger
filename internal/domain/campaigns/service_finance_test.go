package campaigns_test

import (
	"context"
	"errors"
	"testing"

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
