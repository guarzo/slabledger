package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/dhlisting"
)

// MockDHListingService is a test double for dhlisting.Service.
// Each method delegates to a function field, allowing per-test configuration.
type MockDHListingService struct {
	ListPurchasesFn func(ctx context.Context, certNumbers []string) dhlisting.DHListingResult
}

var _ dhlisting.Service = (*MockDHListingService)(nil)

func (m *MockDHListingService) ListPurchases(ctx context.Context, certNumbers []string) dhlisting.DHListingResult {
	if m.ListPurchasesFn != nil {
		return m.ListPurchasesFn(ctx, certNumbers)
	}
	return dhlisting.DHListingResult{}
}
