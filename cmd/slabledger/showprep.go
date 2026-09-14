package main

import (
	"github.com/guarzo/slabledger/internal/adapters/clients/cardladder"
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/showprep"
	"time"
)

func buildShowPrepHandler(in handlerInputs) *handlers.ShowPrepHandler {
	if in.DB == nil {
		return nil
	}
	// Retain the configured client even when credentials are not yet available;
	// admin credential updates act on that same client. Lists need only the DB.
	var source showprep.Source
	if in.CLClient != nil {
		source = cardladder.NewShowPrepSource(in.CLClient, in.Logger)
	}
	store := postgres.NewShowPrepStore(in.DB.DB)
	service := showprep.NewService(store, source, time.Now)
	return handlers.NewShowPrepHandler(service, in.Cfg.Server.WriteTimeout, in.Logger)
}
