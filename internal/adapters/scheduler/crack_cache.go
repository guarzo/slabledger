package scheduler

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

var _ Scheduler = (*CrackCacheRefreshJob)(nil)

const crackCacheRefreshInterval = 15 * time.Minute

// CrackCacheRefreshJob periodically refreshes the crack candidate cache in the
// inventory service. This was previously an embedded goroutine in inventory.service.
type CrackCacheRefreshJob struct {
	StopHandle
	svc    inventory.Service
	logger observability.Logger
}

// NewCrackCacheRefreshJob creates a new crack cache refresh job.
func NewCrackCacheRefreshJob(svc inventory.Service, logger observability.Logger) *CrackCacheRefreshJob {
	return &CrackCacheRefreshJob{
		StopHandle: NewStopHandle(),
		svc:        svc,
		logger:     logger.With(context.Background(), observability.String("component", "crack-cache")),
	}
}

// Start begins the background crack cache refresh loop.
func (j *CrackCacheRefreshJob) Start(ctx context.Context) {
	RunLoop(ctx, LoopConfig{
		Name:     "crack-cache",
		Interval: crackCacheRefreshInterval,
		WG:       j.WG(),
		StopChan: j.Done(),
		Logger:   j.logger,
	}, j.refresh)
}

// refresh performs an initial refresh, then runs periodically via RunLoop.
func (j *CrackCacheRefreshJob) refresh(ctx context.Context) {
	if err := j.svc.RefreshCrackCandidates(ctx); err != nil && j.logger != nil {
		j.logger.Warn(ctx, "crack cache refresh failed", observability.Err(err))
	}
}
