package main

import (
	"github.com/guarzo/slabledger/internal/adapters/httpserver/handlers"
	"github.com/guarzo/slabledger/internal/adapters/storage/postgres"
	"github.com/guarzo/slabledger/internal/domain/showprep"
	"time"
)

func buildShowPrepHandler(in handlerInputs) *handlers.ShowPrepHandler {
	if in.DB == nil {
		return nil
	}
	// Operator reads and list writes cannot acquire provider evidence.
	store := postgres.NewShowPrepStore(in.DB.DB)
	service := showprep.NewService(store, nil, time.Now)
	return handlers.NewShowPrepHandler(service, in.Logger)
}
