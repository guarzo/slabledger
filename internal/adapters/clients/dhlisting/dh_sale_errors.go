package dhlisting

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/adapters/clients/httpx"
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// dhSaleErrorBody is the documented DH error shape for the sale/void
// endpoints (design doc §3). DH validates only successful responses against
// its schema, so error bodies are not machine-checked on their side: every
// field here is parsed defensively. A field that is absent, null, malformed,
// or of the wrong type must never panic or hard-fail — it degrades to
// classifying purely by HTTP status.
//
// SoldAt is decoded as a raw string rather than *time.Time: encoding/json
// aborts the entire object decode on a time-parsing failure (*time.ParseError
// is not an UnmarshalTypeError), which would otherwise discard a well-formed
// Code alongside it. It is parsed separately below and left nil on failure.
type dhSaleErrorBody struct {
	Code    string  `json:"code"`
	SoldAt  *string `json:"sold_at"`
	Channel *string `json:"channel"`
}

// parsedSoldAt parses body.SoldAt as RFC3339, returning nil if it is absent
// or malformed rather than failing the whole classification.
func (b dhSaleErrorBody) parsedSoldAt() *time.Time {
	if b.SoldAt == nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *b.SoldAt)
	if err != nil {
		return nil
	}
	return &t
}

// classifyDHSaleError maps an httpx.UpstreamError to an inventory sentinel
// per design doc §3. Errors that are not UpstreamErrors (network failures,
// timeouts, circuit-breaker trips) pass through unchanged.
func classifyDHSaleError(err error) error {
	var ue *httpx.UpstreamError
	if !errors.As(err, &ue) {
		return err
	}

	var body dhSaleErrorBody
	// Ignore the unmarshal error: a malformed or empty body leaves body
	// zero-valued, which falls through to the by-status-class default below.
	_ = json.Unmarshal([]byte(ue.Body), &body)

	switch ue.StatusCode {
	case 409:
		switch body.Code {
		case "item_sold_on_channel":
			return &inventory.DHChannelSaleError{SoldAt: body.parsedSoldAt(), Channel: body.Channel}
		case "item_unavailable":
			return fmt.Errorf("%w: %w", inventory.ErrDHItemUnavailable, err)
		case "idempotency_in_progress":
			return fmt.Errorf("%w: %w", inventory.ErrDHIdempotencyInProgress, err)
		case "reversal_would_collide":
			return fmt.Errorf("%w: %w", inventory.ErrDHReversalWouldCollide, err)
		default:
			return fmt.Errorf("%w: %w", inventory.ErrDHValidation, err)
		}
	case 422:
		if body.Code == "idempotency_key_reused" {
			return fmt.Errorf("%w: %w", inventory.ErrDHIdempotencyKeyReused, err)
		}
		return fmt.Errorf("%w: %w", inventory.ErrDHValidation, err)
	case 503:
		return fmt.Errorf("%w: %w", inventory.ErrDHLockContention, err)
	case 404:
		return fmt.Errorf("%w: %w", inventory.ErrDHSaleNotFound, err)
	default:
		return err
	}
}
