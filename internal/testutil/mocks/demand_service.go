package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/demand"
)

// DemandServiceMock is a Fn-field mock of the demand-service surface that
// downstream handlers/schedulers (T4/T5) will depend on.
//
// demand.Service is a concrete struct; consumers declare their own small
// interface matching this shape (typically just Leaderboard). This mock
// satisfies any such interface.
type DemandServiceMock struct {
	LeaderboardFn func(ctx context.Context, opts demand.LeaderboardOpts) ([]demand.NicheOpportunity, error)
}

func (m *DemandServiceMock) Leaderboard(ctx context.Context, opts demand.LeaderboardOpts) ([]demand.NicheOpportunity, error) {
	if m.LeaderboardFn != nil {
		return m.LeaderboardFn(ctx, opts)
	}
	return nil, nil
}
