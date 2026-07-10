package psaportal

import (
	"context"
	"testing"
	"time"

	"github.com/guarzo/slabledger/internal/testutil/mocks"
)

func TestStoredTokenProvider(t *testing.T) {
	tests := []struct {
		name    string
		tok     string
		exp     time.Time
		want    string
		wantErr bool
	}{
		{
			name:    "empty token",
			tok:     "",
			wantErr: true,
		},
		{
			name:    "expired token",
			tok:     "x",
			exp:     time.Now().Add(-time.Minute),
			wantErr: true,
		},
		{
			name: "valid token",
			tok:  "good",
			exp:  time.Now().Add(time.Hour),
			want: "good",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mocks.PSATokenRepositoryMock{
				CurrentTokenFn: func(context.Context) (string, time.Time, error) {
					return tt.tok, tt.exp, nil
				},
			}
			got, err := NewStoredTokenProvider(store).AccessToken(ctx)
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
