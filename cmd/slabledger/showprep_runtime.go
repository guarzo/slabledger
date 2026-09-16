package main

import (
	"context"

	cl "github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/scheduler"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/showprep"
)

// This is the production composition, also used by local integration fixtures.
// An absent CL store (no encryption key) is an honest unconfigured worker, not
// an absent coverage endpoint. Operator services never receive this source.
func wireShowPrepScheduler(ctx context.Context, deps schedulerDeps, build *scheduler.BuildDeps) {
	if deps.DB == nil {
		return
	}
	var reader cl.ConfigReader
	if deps.CardLadderStore != nil {
		reader = deps.CardLadderStore
	}
	credentials := cl.NewConfiguredClient(reader, deps.CardLadderClient, deps.Logger, deps.cardLadderAuthOptions...)
	if err := credentials.Refresh(ctx); err != nil {
		deps.Logger.Warn(ctx, "show evidence configuration unavailable at startup")
	}
	build.ShowPrepCredentials = credentials
	build.ShowPrepWorker = showprep.NewEvidenceWorker(postgres.NewShowPrepWorkerStore(deps.DB.DB), credentials.Source, nil, nil)
}

func buildShowPrepWorkerHandler(in handlerInputs) *handlers.ShowPrepWorkerHandler {
	if in.SchedulerResult == nil || in.SchedulerResult.ShowPrepRefresh == nil {
		return nil
	}
	return handlers.NewShowPrepWorkerHandler(in.SchedulerResult.ShowPrepRefresh)
}

func buildCardLadderHandler(in handlerInputs) *handlers.CardLadderHandler {
	if in.CLStore == nil {
		return nil
	}
	h := handlers.NewCardLadderHandler(in.CLStore, in.CLClient, in.Logger)
	wireShowPrepCredentials(h, in.SchedulerResult)
	return h
}

func wireShowPrepCredentials(h *handlers.CardLadderHandler, result *scheduler.BuildResult) {
	if h == nil || result == nil || result.CardLadderCredentials == nil || result.ShowPrepRefresh == nil {
		return
	}
	h.SetConfiguredClient(result.CardLadderCredentials, result.ShowPrepRefresh.CredentialsChanged)
}
