package main

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// shutdownGracefully stops schedulers and waits for in-flight background
// goroutines before the database is closed.
func shutdownGracefully(
	ctx context.Context,
	logger observability.Logger,
	cancelScheduler context.CancelFunc,
	schedulerResult *scheduler.BuildResult,
	hOut handlerOutputs,
	campaignsService inventory.Service,
	shutdownTimeout time.Duration,
) {
	logger.Info(ctx, "shutting down schedulers")
	cancelScheduler()
	schedulerResult.Group.StopAll()

	waitDone := make(chan struct{})
	go func() {
		schedulerResult.Group.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		// Schedulers shut down cleanly
	case <-time.After(shutdownTimeout):
		logger.Warn(ctx, "scheduler shutdown timed out",
			observability.String("timeout", shutdownTimeout.String()))
	}

	// Wait for in-flight background DH work (bulk match + best-effort
	// follow-ups like ConfirmMatch/DelistChannels) to finish. Bounded so a
	// hung DH roundtrip can't block process shutdown indefinitely.
	if hOut.DHHandler != nil {
		waitBounded(ctx, logger, "dh handler", hOut.DHHandler.Wait, shutdownTimeout)
	}

	// Wait for any in-flight background advisor analyses to finish. Bounded
	// for the same reason — a slow LLM call can't stall process shutdown.
	if hOut.AdvisorHandler != nil {
		waitBounded(ctx, logger, "advisor handler", hOut.AdvisorHandler.Wait, shutdownTimeout)
	}

	// Shut down campaign service background workers
	if campaignsService != nil {
		campaignsService.Close()
	}
}

// waitBounded runs wait on a goroutine and returns when it completes or when
// shutdownTimeout elapses. Used so a hung background handler can't block
// process shutdown indefinitely; on timeout it logs a warning and returns,
// leaving the goroutine to complete on its own.
func waitBounded(
	ctx context.Context,
	logger observability.Logger,
	name string,
	wait func(),
	shutdownTimeout time.Duration,
) {
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		logger.Warn(ctx, "background shutdown timed out",
			observability.String("handler", name),
			observability.String("timeout", shutdownTimeout.String()))
	}
}
