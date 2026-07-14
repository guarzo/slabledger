package psaportal

import (
	"context"
	"fmt"
	"time"
)

// StoredTokenProvider adapts a TokenStore into the Client's TokenProvider,
// serving the most recently harvested access token for campaign fetch/push.
type StoredTokenProvider struct {
	store TokenStore
}

// NewStoredTokenProvider wraps a TokenStore (e.g. the Postgres token store).
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
