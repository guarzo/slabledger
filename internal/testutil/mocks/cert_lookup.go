package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// MockCertLookup is a test double for inventory.CertLookup.
// The LookupCertFn field allows per-test configuration of the lookup behavior.
//
// Example:
//
//	cl := &MockCertLookup{
//	    LookupCertFn: func(ctx context.Context, certNumber string) (*inventory.CertInfo, error) {
//	        return &inventory.CertInfo{CertNumber: certNumber, CardName: "Charizard", Grade: 9}, nil
//	    },
//	}
type MockCertLookup struct {
	LookupCertFn func(ctx context.Context, certNumber string) (*inventory.CertInfo, error)
}

var _ inventory.CertLookup = (*MockCertLookup)(nil)

func (m *MockCertLookup) LookupCert(ctx context.Context, certNumber string) (*inventory.CertInfo, error) {
	if m.LookupCertFn != nil {
		return m.LookupCertFn(ctx, certNumber)
	}
	return nil, nil
}
