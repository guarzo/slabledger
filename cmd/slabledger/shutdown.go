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
		dhDone := make(chan struct{})
		go func() {
			hOut.DHHandler.Wait()
			close(dhDone)
		}()
		select {
		case <-dhDone:
		case <-time.After(shutdownTimeout):
			logger.Warn(ctx, "dh handler background shutdown timed out",
				observability.String("timeout", shutdownTimeout.String()))
		}
	}

	// Wait for any in-flight background advisor analyses to finish
	if hOut.AdvisorHandler != nil {
		hOut.AdvisorHandler.Wait()
	}

	// Shut down campaign service background workers
	if campaignsService != nil {
		campaignsService.Close()
	}
}
