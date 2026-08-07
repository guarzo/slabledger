package mocks

import "github.com/guarzo/slabledger/internal/domain/psacampaign"

// ResolverMock implements psacampaign.Resolver with the Fn-field pattern.
// Zero-value defaults return the "unknown" sentinel errors rather than a
// silent zero id — a test must opt in to a resolvable name/language, exactly
// as catalogResolver never silently returns 0 for an unresolved name
// (resolver.go:140-143). Callers rely on this: a bogus id headed for the live
// portal is worse than a test failing loudly for forgetting to set the Fn.
type ResolverMock struct {
	SpecListIDsFn func(languageTokens []string) ([]string, error)
	SubjectIDFn   func(name string) (int, error)
}

var _ psacampaign.Resolver = (*ResolverMock)(nil)

func (m *ResolverMock) SpecListIDs(languageTokens []string) ([]string, error) {
	if m.SpecListIDsFn != nil {
		return m.SpecListIDsFn(languageTokens)
	}
	return nil, psacampaign.ErrUnknownSpecList
}

func (m *ResolverMock) SubjectID(name string) (int, error) {
	if m.SubjectIDFn != nil {
		return m.SubjectIDFn(name)
	}
	return 0, psacampaign.ErrUnknownSubject
}
