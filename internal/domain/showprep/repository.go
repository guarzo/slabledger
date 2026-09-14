package showprep

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("show preparation record not found")
	ErrConflict = errors.New("show preparation state changed; reload and review")
	ErrInvalid  = errors.New("invalid show preparation input")
)

type PurchaseReader interface {
	ReadPurchases(context.Context, []string) (map[string]Purchase, error)
}
type PriceAssociationStore interface {
	ObservePriceAssociations(context.Context, []string) (map[string]bool, error)
}
type EvidenceReader interface {
	ReadSnapshots(context.Context, []Identity) (map[Identity]*Snapshot, error)
}
type EvidenceStore interface {
	EvidenceReader
	BeginAttempt(context.Context, Identity, time.Time) (int64, error)
	FinishAttempt(context.Context, Identity, int64, Snapshot) error
}
type Source interface {
	Fetch(context.Context, Identity, time.Time) (Snapshot, error)
}

// Session is used only within a transaction. LockPurchases locks purchases first,
// then their current campaigns in stable order, before fresh reads and item writes.
// Contended rows return ErrConflict rather than waiting into legacy bulk-lock cycles.
type Session interface {
	PurchaseReader
	PriceAssociationStore
	EvidenceReader
	LockPurchases(context.Context, []string) error
	GetList(context.Context, string) (List, error)
	GetItems(context.Context, string) ([]Item, error)
	InsertItem(context.Context, string, Item) error
	SaveItem(context.Context, string, Item) error
}
type ListStore interface {
	Lists(context.Context) ([]List, error)
	CreateList(context.Context, string, string) (List, error)
	RenameList(context.Context, string, string) (List, error)
	GetList(context.Context, string) (List, error)
	GetItems(context.Context, string) ([]Item, error)
	RemoveItem(context.Context, string, string) error
	// Within commits list writes only on success. On rejection it rolls back all
	// list writes but durably retains price-association collisions actually observed.
	Within(context.Context, func(Session) error) error
}
type Store interface {
	PurchaseReader
	PriceAssociationStore
	EvidenceStore
	ListStore
}
