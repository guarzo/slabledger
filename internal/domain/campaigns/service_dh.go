package campaigns

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/errors"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ApproveDHPush clears the hold on a purchase and re-queues it for DH push.
func (s *service) ApproveDHPush(ctx context.Context, purchaseID string) error {
	p, err := s.repo.GetPurchase(ctx, purchaseID)
	if err != nil {
		return err
	}
	if p.DHPushStatus != DHPushStatusHeld {
		return errors.NewAppError(ErrCodeCampaignValidation, "purchase is not in held status")
	}
	prevStatus := p.DHPushStatus
	holdReasonCleared := p.DHHoldReason
	if err := s.repo.ApproveHeldPurchase(ctx, purchaseID); err != nil {
		return err
	}
	if s.logger != nil {
		s.logger.Info(ctx, "dh push approved",
			observability.String("purchaseID", purchaseID),
			observability.String("previousStatus", prevStatus),
			observability.String("newStatus", DHPushStatusPending),
			observability.String("holdReasonCleared", holdReasonCleared),
			observability.Time("timestamp", time.Now()),
		)
	}
	return nil
}

// GetDHPushConfig returns the current DH push safety configuration.
func (s *service) GetDHPushConfig(ctx context.Context) (*DHPushConfig, error) {
	return s.repo.GetDHPushConfig(ctx)
}

// SaveDHPushConfig persists the DH push safety configuration.
func (s *service) SaveDHPushConfig(ctx context.Context, cfg *DHPushConfig) error {
	if cfg == nil {
		return errors.NewAppError(ErrCodeCampaignValidation, "dh push config cannot be nil")
	}
	if cfg.SwingPctThreshold <= 0 || cfg.SwingMinCents <= 0 ||
		cfg.DisagreementPctThreshold <= 0 || cfg.UnreviewedChangePctThreshold <= 0 ||
		cfg.UnreviewedChangeMinCents <= 0 {
		return errors.NewAppError(ErrCodeCampaignValidation, "all threshold values must be positive")
	}
	return s.repo.SaveDHPushConfig(ctx, cfg)
}
