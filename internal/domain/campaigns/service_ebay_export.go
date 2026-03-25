package campaigns

import (
	"context"
	"errors"
)

// ImportCerts imports purchases by PSA cert numbers.
// TODO: full implementation in a later task.
func (s *service) ImportCerts(_ context.Context, _ []string) (*CertImportResult, error) {
	return nil, errors.New("not implemented")
}

// ListEbayExportItems returns items eligible for eBay export.
// TODO: full implementation in a later task.
func (s *service) ListEbayExportItems(_ context.Context, _ bool) (*EbayExportListResponse, error) {
	return nil, errors.New("not implemented")
}

// GenerateEbayCSV generates a CSV file for eBay bulk upload.
// TODO: full implementation in a later task.
func (s *service) GenerateEbayCSV(_ context.Context, _ []EbayExportGenerateItem) ([]byte, error) {
	return nil, errors.New("not implemented")
}
