package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/guarzo/slabledger/internal/domain/observability"
)

// LoopConfig configures a standard scheduler run loop.
type LoopConfig struct {
	Name         string
	Interval     time.Duration
	InitialDelay time.Duration   // 0 = run workFn synchronously before the loop (blocks until it returns, delaying ctx.Done handling)
	WG           *sync.WaitGroup // nil = don't track
	StopChan     <-chan struct{}
	Logger       observability.Logger
	LogFields    []observability.Field // extra fields for startup log
}

// RunLoop executes the standard scheduler pattern: optional WaitGroup tracking,
// startup log, initial run (with optional delay), then a tick/stop/context select loop.
//
// The ticker is created AFTER the initial run, so the tick phase is anchored to
// that run rather than to process start. Callers that compute InitialDelay from
// a wall-clock target (timeUntilHour in cardladder_refresh, psa_sync and
// dh_analytics_refresh) depend on this: creating the ticker first made a
// scheduler configured for, say, 04:00 fire its second run one Interval after
// the process booted, drifting off the configured hour for the life of the
// process.
func RunLoop(ctx context.Context, cfg LoopConfig, workFn func(context.Context)) {
	if cfg.WG != nil {
		cfg.WG.Add(1)
		defer cfg.WG.Done()
	}

	fields := append([]observability.Field{observability.Duration("interval", cfg.Interval)}, cfg.LogFields...)
	cfg.Logger.Info(ctx, cfg.Name+" scheduler started", fields...)

	if cfg.InitialDelay > 0 {
		timer := time.NewTimer(cfg.InitialDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-cfg.StopChan:
			timer.Stop()
			return
		case <-timer.C:
			workFn(ctx)
		}
	} else {
		workFn(ctx)
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cfg.Logger.Info(ctx, cfg.Name+" scheduler stopped (context cancelled)")
			return
		case <-cfg.StopChan:
			cfg.Logger.Info(ctx, cfg.Name+" scheduler stopped")
			return
		case <-ticker.C:
			workFn(ctx)
		}
	}
}
