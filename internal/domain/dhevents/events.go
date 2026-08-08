// Package dhevents defines the DH pipeline state-transition event model and the
// Recorder/CountsStore/HistoryStore/Pruner interfaces consumed by domain and
// adapter code.
// Flat sibling of dhlisting/inventory/etc — no cross-imports with other
// domain siblings.
package dhevents

import (
	"context"
	"time"
)

// Type identifies a DH state transition.
type Type string

const (
	TypeEnrolled       Type = "enrolled"
	TypePushed         Type = "pushed"
	TypeListed         Type = "listed"
	TypeListDeferred   Type = "list_deferred" // list attempt deferred due to transient PSA issue (e.g. exhausted keys)
	TypeUnlisted       Type = "unlisted"
	TypeChannelSynced  Type = "channel_synced"
	TypeSold           Type = "sold"
	TypeOrphanSale     Type = "orphan_sale"
	TypeAlreadySold    Type = "already_sold"
	TypeHeld           Type = "held"
	TypeDismissed      Type = "dismissed"
	TypeUnmatched      Type = "unmatched"
	TypeCardIDResolved Type = "card_id_resolved"
	// TypeSkipped records a transient skip in the push scheduler — the purchase
	// stays pending and will be retried on the next cycle. Notes carries the
	// reason (e.g. "api_error", "partner_card_error: <DH msg>").
	TypeSkipped Type = "skipped"
)

// Source identifies which subsystem produced an event.
type Source string

const (
	SourceDHOrdersPoll    Source = "dh_orders_poll"
	SourceDHInventoryPoll Source = "dh_inventory_poll"
	SourceCertIntake      Source = "cert_intake"
	SourcePSAImport       Source = "psa_import"
	SourceManualUI        Source = "manual_ui"
	SourceCLRefresh       Source = "cl_refresh"
	SourceDHListing       Source = "dh_listing"
	SourceDHPush          Source = "dh_push"
	SourceDHReconcile     Source = "dh_reconcile"
)

// Event is one row in the dh_state_events table.
//
// Zero values for optional fields (empty strings, 0 ints) are persisted as
// SQL NULL by the adapter.
type Event struct {
	PurchaseID     string // empty for orphan events
	CertNumber     string
	Type           Type
	PrevPushStatus string
	NewPushStatus  string
	PrevDHStatus   string
	NewDHStatus    string
	DHInventoryID  int
	DHCardID       int
	DHOrderID      string
	SalePriceCents int
	Source         Source
	Notes          string
}

// StoredEvent is an Event read back from storage, carrying the two columns the
// writer does not supply: the row id and the timestamp storage stamped on it.
type StoredEvent struct {
	Event
	ID      int64
	EventAt time.Time
}

// History limits bound a per-subject read. The table is append-only and a
// single purchase accumulates one row per transition, so a bounded read is
// always enough to answer "what happened to this one" without risking an
// unbounded scan.
const (
	// DefaultHistoryLimit applies when a caller passes limit <= 0.
	DefaultHistoryLimit = 100
	// MaxHistoryLimit caps what a caller can ask for.
	MaxHistoryLimit = 500
)

// ClampHistoryLimit normalizes a caller-supplied limit into [1, MaxHistoryLimit],
// substituting DefaultHistoryLimit for non-positive values.
func ClampHistoryLimit(limit int) int {
	if limit <= 0 {
		return DefaultHistoryLimit
	}
	return min(limit, MaxHistoryLimit)
}

// Recorder writes events to storage. Implementations should be best-effort:
// a failure is logged by the caller but does not abort the calling operation.
type Recorder interface {
	Record(ctx context.Context, e Event) error
}

// CountsStore exposes aggregate reads used by /api/dh/status.
type CountsStore interface {
	CountByTypeSince(ctx context.Context, t Type, since time.Time) (int, error)
}

// HistoryStore exposes the per-subject reads that make the trail auditable.
// Aggregate counts answer "is the pipeline healthy"; these answer "why is this
// purchase stuck", which is the question the row-level columns exist for.
// Both return newest-first.
type HistoryStore interface {
	ByPurchase(ctx context.Context, purchaseID string, limit int) ([]StoredEvent, error)
	ByCert(ctx context.Context, certNumber string, limit int) ([]StoredEvent, error)
}

// Pruner enforces retention on the append-only trail. Nothing else in the
// system deletes from it, so without a Pruner the table grows without bound.
type Pruner interface {
	// DeleteOlderThan removes events stamped strictly before cutoff and
	// returns how many rows were deleted.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}
