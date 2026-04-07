package campaigns

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/errors"
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
	if err := s.repo.UpdatePurchaseDHHoldReason(ctx, purchaseID, ""); err != nil {
		return err
	}
	return s.repo.UpdatePurchaseDHPushStatus(ctx, purchaseID, DHPushStatusPending)
}

// GetDHPushConfig returns the current DH push safety configuration.
func (s *service) GetDHPushConfig(ctx context.Context) (*DHPushConfig, error) {
	return s.repo.GetDHPushConfig(ctx)
}

// SaveDHPushConfig persists the DH push safety configuration.
func (s *service) SaveDHPushConfig(ctx context.Context, cfg *DHPushConfig) error {
	return s.repo.SaveDHPushConfig(ctx, cfg)
}
