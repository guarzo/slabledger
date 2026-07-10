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
	tests := []struct {
		name    string
		store   fakeStore
		want    string
		wantErr bool
	}{
		{
			name:    "empty token",
			store:   fakeStore{tok: ""},
			wantErr: true,
		},
		{
			name:    "expired token",
			store:   fakeStore{tok: "x", exp: time.Now().Add(-time.Minute)},
			wantErr: true,
		},
		{
			name:  "valid token",
			store: fakeStore{tok: "good", exp: time.Now().Add(time.Hour)},
			want:  "good",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewStoredTokenProvider(tt.store).AccessToken(ctx)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
