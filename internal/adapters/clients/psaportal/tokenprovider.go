package psaportal

import (
	"context"
	"fmt"
	"time"
)

// TokenStore returns the most recently harvested portal access token.
// A "" token (no row yet) is not an error — the provider treats it as "needs harvest".
type TokenStore interface {
	CurrentToken(ctx context.Context) (token string, expiresAt time.Time, err error)
}

// StoredTokenProvider reads access tokens harvested into a TokenStore.
type StoredTokenProvider struct {
	store TokenStore
}

func NewStoredTokenProvider(store TokenStore) *StoredTokenProvider {
	return &StoredTokenProvider{store: store}
}

// AccessToken returns the stored token if present and unexpired.
func (p *StoredTokenProvider) AccessToken(ctx context.Context) (string, error) {
	tok, exp, err := p.store.CurrentToken(ctx)
	if err != nil {
		return "", err
	}
	if tok == "" {
		return "", fmt.Errorf("psaportal: no stored access token; harvester must run")
	}
	if time.Now().After(exp) {
		return "", fmt.Errorf("psaportal: stored access token expired at %s; harvester must run", exp.Format(time.RFC3339))
	}
	return tok, nil
}
