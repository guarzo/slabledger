package psaportal

import (
	"context"
	"testing"
	"time"
)

type fakeStore struct {
	tok string
	exp time.Time
	err error
}

func (f fakeStore) CurrentToken(ctx context.Context) (string, time.Time, error) {
	return f.tok, f.exp, f.err
}

func TestStoredTokenProvider(t *testing.T) {
	ctx := context.Background()
	if _, err := NewStoredTokenProvider(fakeStore{tok: ""}).AccessToken(ctx); err == nil {
		t.Error("expected error for empty token")
	}
	if _, err := NewStoredTokenProvider(fakeStore{tok: "x", exp: time.Now().Add(-time.Minute)}).AccessToken(ctx); err == nil {
		t.Error("expected error for expired token")
	}
	got, err := NewStoredTokenProvider(fakeStore{tok: "good", exp: time.Now().Add(time.Hour)}).AccessToken(ctx)
	if err != nil || got != "good" {
		t.Fatalf("got %q err=%v", got, err)
	}
}
