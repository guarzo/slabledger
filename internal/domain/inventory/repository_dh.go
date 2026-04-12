package inventory

import "context"

// DHRepository handles DH-specific configuration persistence.
type DHRepository interface {
	GetDHPushConfig(ctx context.Context) (*DHPushConfig, error)
	SaveDHPushConfig(ctx context.Context, cfg *DHPushConfig) error
}
