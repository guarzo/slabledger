package inventory

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

func (s *service) batchResolveCardIDs(ctx context.Context, certs []string) {
	if s.cardIDResolver == nil {
		return
	}
	resolved, err := s.cardIDResolver.ResolveCardIDsByCerts(ctx, certs, "PSA")
	if err != nil {
		if s.logger != nil {
			s.logger.Warn(ctx, "batch cert→card_id resolution failed",
				observability.Int("certs", len(certs)),
				observability.Err(err))
		}
		return
	}
	if s.logger != nil && len(resolved) > 0 {
		s.logger.Info(ctx, "batch cert→card_id resolution complete",
			observability.Int("requested", len(certs)),
			observability.Int("resolved", len(resolved)))
	}
}
