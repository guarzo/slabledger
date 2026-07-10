package mocks

import (
	"context"
	"time"
)

// PSATokenRepositoryMock is a test double for psaportal.TokenRepository.
// It records the last saved token/expiry so tests can assert on them.
type PSATokenRepositoryMock struct {
	CurrentTokenFn func(ctx context.Context) (string, time.Time, error)
	SaveTokenFn    func(ctx context.Context, token string, expiresAt time.Time) error

	SavedToken     string
	SavedExpiresAt time.Time
}

func (m *PSATokenRepositoryMock) CurrentToken(ctx context.Context) (string, time.Time, error) {
	if m.CurrentTokenFn != nil {
		return m.CurrentTokenFn(ctx)
	}
	return "", time.Time{}, nil
}

func (m *PSATokenRepositoryMock) SaveToken(ctx context.Context, token string, expiresAt time.Time) error {
	m.SavedToken = token
	m.SavedExpiresAt = expiresAt
	if m.SaveTokenFn != nil {
		return m.SaveTokenFn(ctx, token, expiresAt)
	}
	return nil
}
