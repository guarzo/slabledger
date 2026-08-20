package inventory

import (
	"errors"
	"testing"
	"time"
)

func TestIsRetryableDHSaleError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"item sold on channel is not retryable", ErrDHItemSoldOnChannel, false},
		{"item unavailable is not retryable", ErrDHItemUnavailable, false},
		{"idempotency in progress IS retryable", ErrDHIdempotencyInProgress, true},
		{"reversal would collide is not retryable", ErrDHReversalWouldCollide, false},
		{"idempotency key reused is not retryable", ErrDHIdempotencyKeyReused, false},
		{"validation error is not retryable", ErrDHValidation, false},
		{"lock contention IS retryable", ErrDHLockContention, true},
		{"sale not found is not retryable", ErrDHSaleNotFound, false},
		{"unrelated error is not retryable", ErrPurchaseNotFound, false},
		{"nil is not retryable", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryableDHSaleError(tt.err); got != tt.want {
				t.Errorf("IsRetryableDHSaleError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestDHChannelSaleError(t *testing.T) {
	soldAt := time.Date(2026, 8, 15, 14, 22, 0, 0, time.UTC)
	channel := "ebay"

	tests := []struct {
		name string
		err  *DHChannelSaleError
	}{
		{"both fields populated", &DHChannelSaleError{SoldAt: &soldAt, Channel: &channel}},
		// Both fields may be nil per the DH contract (§3): DH can return a
		// 409 item_sold_on_channel with neither field populated.
		{"both fields nil", &DHChannelSaleError{}},
		{"only channel", &DHChannelSaleError{Channel: &channel}},
		{"only soldAt", &DHChannelSaleError{SoldAt: &soldAt}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, ErrDHItemSoldOnChannel) {
				t.Error("expected errors.Is to match ErrDHItemSoldOnChannel via Unwrap")
			}
			if tt.err.Error() == "" {
				t.Error("expected a non-empty error message")
			}
			if IsRetryableDHSaleError(tt.err) {
				t.Error("a channel sale is a permanent conflict, never retryable")
			}
		})
	}
}

func TestNewDHIdempotencyKey(t *testing.T) {
	got := NewDHIdempotencyKey(func() string { return "abc-123" })
	want := DHIdempotencyKeyPrefix + "abc-123"
	if got != want {
		t.Errorf("NewDHIdempotencyKey() = %q, want %q", got, want)
	}
	if len(got) > 255 {
		t.Errorf("key length %d exceeds DH's 255-char limit", len(got))
	}
}
