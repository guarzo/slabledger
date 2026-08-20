package dhlisting

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

func TestClassifyDHSaleError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    error
	}{
		{"409 item_sold_on_channel with values", 409, `{"code":"item_sold_on_channel","sold_at":"2026-08-15T14:22:00Z","channel":"ebay"}`, inventory.ErrDHItemSoldOnChannel},
		{"409 item_sold_on_channel with nulls", 409, `{"code":"item_sold_on_channel","sold_at":null,"channel":null}`, inventory.ErrDHItemSoldOnChannel},
		{"409 item_unavailable", 409, `{"code":"item_unavailable"}`, inventory.ErrDHItemUnavailable},
		{"409 idempotency_in_progress", 409, `{"code":"idempotency_in_progress"}`, inventory.ErrDHIdempotencyInProgress},
		{"409 reversal_would_collide", 409, `{"code":"reversal_would_collide"}`, inventory.ErrDHReversalWouldCollide},
		{"409 unknown code degrades by status class", 409, `{"code":"something_new"}`, inventory.ErrDHValidation},
		{"409 absent code degrades by status class", 409, `{}`, inventory.ErrDHValidation},
		{"409 malformed JSON degrades by status class", 409, `not json`, inventory.ErrDHValidation},
		{"409 empty body degrades by status class", 409, ``, inventory.ErrDHValidation},
		{"422 idempotency_key_reused", 422, `{"code":"idempotency_key_reused"}`, inventory.ErrDHIdempotencyKeyReused},
		{"422 code null degrades by status class", 422, `{"code":null}`, inventory.ErrDHValidation},
		{"503 lock contention", 503, `{"code":"lock_contention"}`, inventory.ErrDHLockContention},
		{"503 with no body", 503, ``, inventory.ErrDHLockContention},
		{"404 not found", 404, `{"code":"not_found"}`, inventory.ErrDHSaleNotFound},
		{"extra unexpected fields never hard-fail", 409, `{"code":"item_unavailable","extra":{"nested":true},"more":[1,2,3]}`, inventory.ErrDHItemUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpx.UpstreamError{
				Provider:   "dh",
				Op:         "POST /api/v1/enterprise/inventory/1/sale",
				StatusCode: tt.statusCode,
				Body:       tt.body,
			}
			require.ErrorIs(t, classifyDHSaleError(upstream), tt.wantErr)
		})
	}

	t.Run("item_sold_on_channel carries null SoldAt and Channel", func(t *testing.T) {
		upstream := &httpx.UpstreamError{StatusCode: 409, Body: `{"code":"item_sold_on_channel","sold_at":null,"channel":null}`}
		var channelErr *inventory.DHChannelSaleError
		require.ErrorAs(t, classifyDHSaleError(upstream), &channelErr)
		require.Nil(t, channelErr.SoldAt)
		require.Nil(t, channelErr.Channel)
	})

	t.Run("item_sold_on_channel carries populated SoldAt and Channel", func(t *testing.T) {
		upstream := &httpx.UpstreamError{StatusCode: 409, Body: `{"code":"item_sold_on_channel","sold_at":"2026-08-15T14:22:00Z","channel":"ebay"}`}
		var channelErr *inventory.DHChannelSaleError
		require.ErrorAs(t, classifyDHSaleError(upstream), &channelErr)
		require.Equal(t, time.Date(2026, 8, 15, 14, 22, 0, 0, time.UTC), *channelErr.SoldAt)
		require.Equal(t, "ebay", *channelErr.Channel)
	})

	t.Run("unmapped status passes through unchanged", func(t *testing.T) {
		upstream := &httpx.UpstreamError{StatusCode: 400, Body: `{"code":"whatever"}`}
		require.Same(t, upstream, classifyDHSaleError(upstream))
	})

	t.Run("non-UpstreamError passes through unchanged", func(t *testing.T) {
		wantErr := errors.New("network failure")
		require.Same(t, wantErr, classifyDHSaleError(wantErr))
	})
}
