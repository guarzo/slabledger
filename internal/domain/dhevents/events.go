// Package dhevents defines the DH pipeline state-transition event model and
// the Recorder/CountsStore interfaces consumed by domain and adapter code.
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
	TypeChannelSynced  Type = "channel_synced"
	TypeSold           Type = "sold"
	TypeOrphanSale     Type = "orphan_sale"
	TypeAlreadySold    Type = "already_sold"
	TypeHeld           Type = "held"
	TypeDismissed      Type = "dismissed"
	TypeUnmatched      Type = "unmatched"
	TypeCardIDResolved Type = "card_id_resolved"
)

// Source identifies which subsystem produced an event.
type Source string

const (
	SourceDHOrdersPoll    Source = "dh_orders_poll"
	SourceDHInventoryPoll Source = "dh_inventory_poll"
	SourceCertIntake      Source = "cert_intake"
	SourceCLImport        Source = "cl_import"
	SourcePSAImport       Source = "psa_import"
	SourceManualUI        Source = "manual_ui"
	SourceCLRefresh       Source = "cl_refresh"
	SourceDHListing       Source = "dh_listing"
	SourceDHPush          Source = "dh_push"
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

// Recorder writes events to storage. Implementations should be best-effort:
// a failure is logged by the caller but does not abort the calling operation.
type Recorder interface {
	Record(ctx context.Context, e Event) error
}

// CountsStore exposes aggregate reads used by /api/dh/status.
type CountsStore interface {
	CountByTypeSince(ctx context.Context, t Type, since time.Time) (int, error)
}
