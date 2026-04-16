package export

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// CrackCandidateProvider is an optional function that returns the set of crack-candidate purchase IDs.
// Returns nil on cold start (before background worker populated it).
type CrackCandidateProvider func(ctx context.Context) map[string]bool

// Service handles sell sheet generation and eBay/Shopify exports.
type Service interface {
	// Sell sheet
	GenerateSellSheet(ctx context.Context, campaignID string, purchaseIDs []string) (*inventory.SellSheet, error)
	GenerateGlobalSellSheet(ctx context.Context) (*inventory.SellSheet, error)
	GenerateSelectedSellSheet(ctx context.Context, purchaseIDs []string) (*inventory.SellSheet, error)
	// eBay export
	ListEbayExportItems(ctx context.Context, flaggedOnly bool) (*inventory.EbayExportListResponse, error)
	GenerateEbayCSV(ctx context.Context, items []inventory.EbayExportGenerateItem) ([]byte, error)
}

// service is the concrete implementation of Service.
type service struct {
	repo      ExportReader
	crackProv CrackCandidateProvider // optional
	logger    observability.Logger   // optional
}

// Option is a functional option for configuring the export service.
type Option func(*service)

// WithCrackCandidateProvider injects the crack candidate provider.
func WithCrackCandidateProvider(p CrackCandidateProvider) Option {
	return func(s *service) { s.crackProv = p }
}

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

// buildCrackCandidateSet returns the crack candidate set if the provider is configured.
func (s *service) buildCrackCandidateSet(ctx context.Context) map[string]bool {
	if s.crackProv == nil {
		return nil
	}
	return s.crackProv(ctx)
}
