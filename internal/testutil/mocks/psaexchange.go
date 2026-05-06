package mocks

import (
	"context"

	"github.com/guarzo/slabledger/internal/domain/psaexchange"
)

// MockCatalogClient is a test double for psaexchange.CatalogClient.
// Override behavior by setting any of the *Fn fields.
//
// Example:
//
//	c := &MockCatalogClient{
//	    FetchCatalogFn: func(ctx context.Context) (psaexchange.Catalog, error) {
//	        return psaexchange.Catalog{Cards: []psaexchange.CatalogCard{{Cert: "X"}}}, nil
//	    },
//	}
type MockCatalogClient struct {
	FetchCatalogFn    func(ctx context.Context) (psaexchange.Catalog, error)
	FetchCardLadderFn func(ctx context.Context, cert string) (psaexchange.CardLadder, error)
	CategoryURLFn     func(category string) string
}

var _ psaexchange.CatalogClient = (*MockCatalogClient)(nil)

func (m *MockCatalogClient) FetchCatalog(ctx context.Context) (psaexchange.Catalog, error) {
	if m.FetchCatalogFn != nil {
		return m.FetchCatalogFn(ctx)
	}
	return psaexchange.Catalog{}, nil
}

func (m *MockCatalogClient) FetchCardLadder(ctx context.Context, cert string) (psaexchange.CardLadder, error) {
	if m.FetchCardLadderFn != nil {
		return m.FetchCardLadderFn(ctx, cert)
	}
	return psaexchange.CardLadder{}, nil
}

func (m *MockCatalogClient) CategoryURL(category string) string {
	if m.CategoryURLFn != nil {
		return m.CategoryURLFn(category)
	}
	return ""
}

// MockPSAExchangeService is a test double for psaexchange.Service.
type MockPSAExchangeService struct {
	OpportunitiesFn    func(ctx context.Context) (psaexchange.OpportunitiesResult, error)
	PolicyFn           func() psaexchange.Policy
	EffectivePolicyFn  func(ctx context.Context) psaexchange.Policy
	SetPolicyFn        func(ctx context.Context, p psaexchange.Policy) error
}

var _ psaexchange.Service = (*MockPSAExchangeService)(nil)

func (m *MockPSAExchangeService) Opportunities(ctx context.Context) (psaexchange.OpportunitiesResult, error) {
	if m.OpportunitiesFn != nil {
		return m.OpportunitiesFn(ctx)
	}
	return psaexchange.OpportunitiesResult{}, nil
}

func (m *MockPSAExchangeService) Policy() psaexchange.Policy {
	if m.PolicyFn != nil {
		return m.PolicyFn()
	}
	return psaexchange.DefaultPolicy()
}

func (m *MockPSAExchangeService) EffectivePolicy(ctx context.Context) psaexchange.Policy {
	if m.EffectivePolicyFn != nil {
		return m.EffectivePolicyFn(ctx)
	}
	if m.PolicyFn != nil {
		return m.PolicyFn()
	}
	return psaexchange.DefaultPolicy()
}

func (m *MockPSAExchangeService) SetPolicy(ctx context.Context, p psaexchange.Policy) error {
	if m.SetPolicyFn != nil {
		return m.SetPolicyFn(ctx, p)
	}
	return nil
}

// MockPSAExchangePolicyStore is a test double for psaexchange.PolicyStore.
// Override behavior by setting GetFn / SetFn. Calls to Set are recorded in
// SetCalls so tests can assert how many times the store was written and with
// what payload.
type MockPSAExchangePolicyStore struct {
	GetFn    func(ctx context.Context) (psaexchange.Policy, bool, error)
	SetFn    func(ctx context.Context, p psaexchange.Policy) error
	SetCalls []psaexchange.Policy
}

var _ psaexchange.PolicyStore = (*MockPSAExchangePolicyStore)(nil)

func (m *MockPSAExchangePolicyStore) Get(ctx context.Context) (psaexchange.Policy, bool, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx)
	}
	return psaexchange.Policy{}, false, nil
}

func (m *MockPSAExchangePolicyStore) Set(ctx context.Context, p psaexchange.Policy) error {
	if m.SetFn != nil {
		if err := m.SetFn(ctx, p); err != nil {
			return err
		}
	}
	m.SetCalls = append(m.SetCalls, p)
	return nil
}
