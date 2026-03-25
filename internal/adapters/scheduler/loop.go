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
func RunLoop(ctx context.Context, cfg LoopConfig, workFn func(context.Context)) {
	if cfg.WG != nil {
		cfg.WG.Add(1)
		defer cfg.WG.Done()
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

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
