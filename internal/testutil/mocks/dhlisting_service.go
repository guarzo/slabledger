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

// MockDHPSAImporter implements dhlisting.DHPSAImporter with the Fn-field
// pattern and also satisfies dhlisting.PSAKeyRotator so tests that exercise
// the rate-limit / key-rotation path can type-assert successfully.
//
// Two usage modes:
//
//  1. Override PSAImportFn for bespoke behavior.
//  2. Queue responses via Results and errors via Errs; leave PSAImportFn
//     nil. Each call consumes one entry from each queue (FIFO). An empty
//     queue returns (nil, nil). Convenient for rate-limit retry tests
//     where the first call returns RateLimited and the second succeeds.
type MockDHPSAImporter struct {
	// Handlers override the default queue-based behavior when set.
	PSAImportFn           func(ctx context.Context, items []dhlisting.DHPSAImportItem) ([]dhlisting.DHPSAImportResult, error)
	RotatePSAKeyFn        func() bool
	ResetPSAKeyRotationFn func()

	// Response queue consumed by the default PSAImport path.
	Results [][]dhlisting.DHPSAImportResult
	Errs    []error

	// Call counters — populated in both modes.
	Calls            int
	RotateCalls      int
	ResetRotateCalls int
}

func (m *MockDHPSAImporter) PSAImport(ctx context.Context, items []dhlisting.DHPSAImportItem) ([]dhlisting.DHPSAImportResult, error) {
	m.Calls++
	if m.PSAImportFn != nil {
		return m.PSAImportFn(ctx, items)
	}
	if len(m.Errs) > 0 {
		e := m.Errs[0]
		m.Errs = m.Errs[1:]
		if e != nil {
			return nil, e
		}
	}
	if len(m.Results) == 0 {
		return nil, nil
	}
	r := m.Results[0]
	m.Results = m.Results[1:]
	return r, nil
}

func (m *MockDHPSAImporter) RotatePSAKey() bool {
	m.RotateCalls++
	if m.RotatePSAKeyFn != nil {
		return m.RotatePSAKeyFn()
	}
	return false
}

func (m *MockDHPSAImporter) ResetPSAKeyRotation() {
	m.ResetRotateCalls++
	if m.ResetPSAKeyRotationFn != nil {
		m.ResetPSAKeyRotationFn()
	}
}

var _ dhlisting.PSAKeyRotator = (*MockDHPSAImporter)(nil)

// DHPushStatusCall records one UpdatePurchaseDHPushStatus invocation.
type DHPushStatusCall struct {
	ID     string
	Status string
}

// MockDHPushStatusUpdater is a focused mock for the dh_push_status update
// method. Satisfies both scheduler.DHPushStatusUpdater and
// dhlisting.DHListingPushStatusUpdater (identical signatures).
type MockDHPushStatusUpdater struct {
	UpdatePurchaseDHPushStatusFn func(ctx context.Context, id, status string) error
	Calls                        []DHPushStatusCall
}

func (m *MockDHPushStatusUpdater) UpdatePurchaseDHPushStatus(ctx context.Context, id, status string) error {
	m.Calls = append(m.Calls, DHPushStatusCall{ID: id, Status: status})
	if m.UpdatePurchaseDHPushStatusFn != nil {
		return m.UpdatePurchaseDHPushStatusFn(ctx, id, status)
	}
	return nil
}
