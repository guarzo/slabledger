package inventory

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestPendingItemRepository_InterfaceDefined(t *testing.T) {
	// Compile-time interface satisfaction check.
	var _ PendingItemRepository = nil
	_ = PendingItem{}
	_ = ErrPendingItemNotFound
	_ = IsPendingItemNotFound(nil)
}

func TestIsPendingItemNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"sentinel error", ErrPendingItemNotFound, true},
		{"wrapped sentinel", fmt.Errorf("wrap: %w", ErrPendingItemNotFound), true},
		{"other error", errors.New("something else"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPendingItemNotFound(tt.err); got != tt.want {
				t.Errorf("IsPendingItemNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestImportSourceFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"default is manual", context.Background(), "manual"},
		{"scheduler from context", WithImportSource(context.Background(), "scheduler"), "scheduler"},
		{"manual from context", WithImportSource(context.Background(), "manual"), "manual"},
		{"empty string defaults to manual", WithImportSource(context.Background(), ""), "manual"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := importSourceFromContext(tt.ctx)
			if got != tt.want {
				t.Errorf("importSourceFromContext() = %q, want %q", got, tt.want)
			}
		})
	}
}
