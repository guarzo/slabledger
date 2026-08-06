package mocks

import "github.com/guarzo/slabledger/internal/domain/psacampaign"

// ResolverMock implements psacampaign.Resolver with the Fn-field pattern.
type ResolverMock struct {
	SpecListIDsFn func(languageToken string) ([]string, error)
	SubjectIDFn   func(name string) (int, error)
}

var _ psacampaign.Resolver = (*ResolverMock)(nil)

func (m *ResolverMock) SpecListIDs(languageToken string) ([]string, error) {
	if m.SpecListIDsFn != nil {
		return m.SpecListIDsFn(languageToken)
	}
	return nil, nil
}

func (m *ResolverMock) SubjectID(name string) (int, error) {
	if m.SubjectIDFn != nil {
		return m.SubjectIDFn(name)
	}
	return 0, nil
}
