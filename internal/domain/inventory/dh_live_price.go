package inventory

import (
	"context"
	"fmt"
	"time"
)

// DHLivePriceResult is the verified price-only receipt for one keyed request.
type DHLivePriceResult struct {
	DHInventoryID     int
	ListingPriceCents int
	Replayed          bool
}

// DHLivePriceUpdater never changes listing status or marketplace channels.
type DHLivePriceUpdater interface {
	UpdateLivePrice(context.Context, int, string, int) (*DHLivePriceResult, error)
}

// DHLivePriceRejection is a source-verified refusal serialized with prior work
// for the same key. It is distinct from a generic pre-mutation refusal.
type DHLivePriceRejection struct {
	Code  string
	Cause error
}

func (e *DHLivePriceRejection) Error() string {
	return fmt.Sprintf("DH live price rejected (%s): %v", e.Code, e.Cause)
}
func (e *DHLivePriceRejection) Unwrap() error { return e.Cause }

// DHMutationRetryError keeps the attempt open until the durable cooldown ends.
type DHMutationRetryError struct {
	NotBefore time.Time
	Cause     error
}

func (e *DHMutationRetryError) Error() string {
	return fmt.Sprintf("DH mutation retry not before %s: %v", e.NotBefore.Format(time.RFC3339Nano), e.Cause)
}
func (e *DHMutationRetryError) Unwrap() error { return e.Cause }

// DHMutationRetryRepository optionally persists cooldown under exact ownership.
type DHMutationRetryRepository interface {
	DeferDHMutation(context.Context, string, *DHMutationAttempt, time.Time) error
}
