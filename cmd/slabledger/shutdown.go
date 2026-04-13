package main

import (
	"context"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
	"github.com/guarzo/slabledger/internal/domain/social"
)

// shutdownGracefully stops schedulers and waits for in-flight background
// goroutines before the database is closed.
func shutdownGracefully(
	ctx context.Context,
	logger observability.Logger,
	cancelScheduler context.CancelFunc,
	schedulerResult *scheduler.BuildResult,
	hOut handlerOutputs,
	socialService social.Service,
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

	// Wait for in-flight background DH bulk match to finish
	if hOut.DHHandler != nil {
		hOut.DHHandler.Wait()
	}

	// Wait for any in-flight background advisor analyses to finish
	if hOut.AdvisorHandler != nil {
		hOut.AdvisorHandler.Wait()
	}

	// Wait for in-flight social handler background goroutines (HandleGenerate)
	if hOut.SocialHandler != nil {
		hOut.SocialHandler.Wait()
	}

	// Wait for in-flight social caption generation goroutines
	socialService.Wait()

	// Shut down campaign service background workers
	if campaignsService != nil {
		campaignsService.Close()
	}
}
