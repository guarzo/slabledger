package export

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// Service handles sell sheet generation.
type Service interface {
	// Sell sheet
	GenerateGlobalSellSheet(ctx context.Context) (*inventory.SellSheet, error)
}

// service is the concrete implementation of Service.
type service struct {
	repo   ExportReader
	logger observability.Logger // optional
}

// Option is a functional option for configuring the export service.
type Option func(*service)

// WithLogger injects an optional logger.
func WithLogger(logger observability.Logger) Option {
	return func(s *service) { s.logger = logger }
}

// New creates a new export service.
func New(repo ExportReader, opts ...Option) Service {
	s := &service{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
