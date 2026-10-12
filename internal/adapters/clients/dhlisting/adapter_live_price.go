package dhlisting

import (
	"context"
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/dh"
	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// LivePriceAdapter has no listing, PSA, channel or lifecycle capabilities.
type LivePriceAdapter struct {
	client interface {
		UpdateLivePrice(context.Context, int, string, int) (*dh.LivePriceResult, error)
	}
}

func NewLivePriceAdapter(client interface {
	UpdateLivePrice(context.Context, int, string, int) (*dh.LivePriceResult, error)
}) *LivePriceAdapter {
	return &LivePriceAdapter{client: client}
}

func (a *LivePriceAdapter) UpdateLivePrice(ctx context.Context, target int, key string, cents int) (*inventory.DHLivePriceResult, error) {
	result, err := a.client.UpdateLivePrice(ctx, target, key, cents)
	if err != nil {
		return nil, classifyDHLivePriceError(err)
	}
	if result == nil || result.DHInventoryID != target || result.ListingPriceCents != cents {
		return nil, fmt.Errorf("DH live-price receipt does not match captured target and cents")
	}
	return &inventory.DHLivePriceResult{DHInventoryID: result.DHInventoryID,
		ListingPriceCents: result.ListingPriceCents, Replayed: result.Replayed}, nil
}

func classifyDHLivePriceError(err error) error {
	var upstream *httpx.UpstreamError
	if !errors.As(err, &upstream) {
		return err
	}
	if upstream.StatusCode == http.StatusTooManyRequests {
		return &inventory.DHMutationRetryError{
			NotBefore: livePriceRetryNotBefore(upstream.RetryAfter, time.Now()), Cause: err,
		}
	}
	mediaType, _, mediaErr := mime.ParseMediaType(upstream.ContentType)
	if mediaErr != nil || mediaType != "application/json" {
		return err
	}
	// PR2331 at e331f3c3 rechecks receipts inside the key lock before these
	// refusals; a matching older commit would replay instead. This proof is
	// endpoint-specific and must not become a generic DHNonMutationError.
	if (upstream.StatusCode == http.StatusConflict &&
		(upstream.Code == "inventory_not_listed" || upstream.Code == "stale_listing")) ||
		(upstream.StatusCode == http.StatusUnprocessableEntity && upstream.Code == "price_validation_failed") {
		return &inventory.DHLivePriceRejection{Code: upstream.Code, Cause: err}
	}
	return err
}

func livePriceRetryNotBefore(raw string, now time.Time) time.Time {
	// Do not cap valid deadlines: DH requires retries to honor Retry-After.
	raw = strings.TrimSpace(raw)
	// Retry-After delta-seconds are unsigned decimal digits, not signed or
	// fractional durations. Add Unix seconds directly: valid long delays need
	// not fit time.Duration's much smaller nanosecond range.
	if raw != "" && strings.IndexFunc(raw, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
		seconds, err := strconv.ParseUint(raw, 10, 63)
		if err == nil && seconds <= uint64(math.MaxInt64-now.Unix()) {
			return time.Unix(now.Unix()+int64(seconds), int64(now.Nanosecond())).In(now.Location())
		}
	}
	if deadline, err := http.ParseTime(raw); err == nil {
		if deadline.Before(now) {
			return now
		}
		return deadline
	}
	return now.Add(15 * time.Minute)
}

var _ inventory.DHLivePriceUpdater = (*LivePriceAdapter)(nil)
