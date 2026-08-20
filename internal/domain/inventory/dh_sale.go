package inventory

import (
	"context"
	"fmt"
	"time"

	domainerrors "github.com/guarzo/slabledger/internal/domain/errors"
)

// DHSaleRecorder records and voids sales against DH's inventory-sale API
// (docs/specs/2026-08-20-dh-record-sale-design.md). It replaces the earlier
// DHSoldNotifier, which could only express a status PATCH -- and DH's
// inventory vocabulary never accepted the "sold" status that notifier sent.
type DHSaleRecorder interface {
	RecordInventorySale(ctx context.Context, req DHSaleRequest) (*DHSaleResult, error)
	VoidInventorySale(ctx context.Context, dhSaleID, reason string) error
}

// DHSaleRequest is the input to RecordInventorySale. Every field must be a
// pure function of persisted columns (§2 of the design) -- nothing in it may
// read the wall clock -- because it is re-sent byte-identical on any retry
// under the same IdempotencyKey, and DH treats a changed body under a reused
// key as a permanent error (ErrDHIdempotencyKeyReused).
type DHSaleRequest struct {
	DHInventoryID    int
	IdempotencyKey   string
	SalePriceCents   int       // PER UNIT, never derived from RealizedProfitCents
	SoldAt           time.Time // already UTC-normalised and clamped; see DeriveDHSoldAt
	CounterpartyName string
	Notes            string
}

// DHSaleResult is the parsed response from RecordInventorySale. Every field
// is read directly off the DH response rather than inferred, because this
// system's qty-always-1 assumption is a scope simplification, not something
// DH's response shape guarantees (see the design doc's contract-hazards table).
type DHSaleResult struct {
	DHSaleID            string
	SoldInventoryID     *int   // nullable: DH addressed a different row (partial sale) or none
	Delisted            bool   // true means "no live DH ask remains"; never inferred
	ItemStatus          string // open-ended by contract; NOT an enum
	Replayed            bool   // true when this call matched an already-recorded idempotency key
	RealizedProfitCents int    // TOTAL across units, never per-unit
}

// DHIdempotencyKeyPrefix marks a key as server-minted (§2 of the design). The
// key is never derived from a client-controllable value such as Sale.ID: a
// client can supply its own sale id (the HTTP handler decodes client input
// straight into a Sale), including a reused one, which would make a derived
// key a client-triggerable double-disposal.
const DHIdempotencyKeyPrefix = "slabledger-sale-"

// NewDHIdempotencyKey mints a fresh key by calling gen (typically a UUID
// generator) and prefixing it. Callers must persist the result via
// SaleRepository.SetSaleIdempotencyKeyIfAbsent BEFORE issuing the DH call --
// recording first and persisting after leaves a successful remote mutation
// with no key to replay (§2).
func NewDHIdempotencyKey(gen func() string) string {
	return DHIdempotencyKeyPrefix + gen()
}

// retryableDHSaleCodes is EXACTLY the two-member set the design (§3) permits
// re-issuing byte-identical: idempotency_in_progress and lock_contention.
// Every other sentinel represents a permanent outcome.
var retryableDHSaleCodes = []domainerrors.ErrorCode{
	ErrCodeDHIdempotencyInProgress,
	ErrCodeDHLockContention,
}

// IsRetryableDHSaleError reports whether the identical request may be
// re-issued. Only ErrDHIdempotencyInProgress and ErrDHLockContention qualify;
// every other DH sale sentinel (and any non-DH error) is permanent.
func IsRetryableDHSaleError(err error) bool {
	if err == nil {
		return false
	}
	for _, code := range retryableDHSaleCodes {
		if domainerrors.HasErrorCode(err, code) {
			return true
		}
	}
	return false
}

// DHChannelSaleError carries the optional detail DH returns with a 409
// item_sold_on_channel response. Both fields may be nil -- DH's contract
// permits an empty detail body even on this specific error code.
type DHChannelSaleError struct {
	SoldAt  *time.Time
	Channel *string
}

func (e *DHChannelSaleError) Error() string {
	channel := "unknown channel"
	if e.Channel != nil {
		channel = *e.Channel
	}
	if e.SoldAt != nil {
		return fmt.Sprintf("item already sold on %s at %s", channel, e.SoldAt.Format(time.RFC3339))
	}
	return fmt.Sprintf("item already sold on %s", channel)
}

// Unwrap lets errors.Is(err, ErrDHItemSoldOnChannel) match, so callers that
// only care about the sentinel (conflict flagging, retry classification)
// don't need to type-assert *DHChannelSaleError.
func (e *DHChannelSaleError) Unwrap() error {
	return ErrDHItemSoldOnChannel
}
