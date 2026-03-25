package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/campaigns"
)

// MockCertLookup is a test double for campaigns.CertLookup.
// The LookupCertFn field allows per-test configuration of the lookup behavior.
//
// Example:
//
//	cl := &MockCertLookup{
//	    LookupCertFn: func(ctx context.Context, certNumber string) (*campaigns.CertInfo, error) {
//	        return &campaigns.CertInfo{CertNumber: certNumber, CardName: "Charizard", Grade: 9}, nil
//	    },
//	}
type MockCertLookup struct {
	LookupCertFn func(ctx context.Context, certNumber string) (*campaigns.CertInfo, error)
}

var _ campaigns.CertLookup = (*MockCertLookup)(nil)

func (m *MockCertLookup) LookupCert(ctx context.Context, certNumber string) (*campaigns.CertInfo, error) {
	if m.LookupCertFn != nil {
		return m.LookupCertFn(ctx, certNumber)
	}
	return nil, nil
}
