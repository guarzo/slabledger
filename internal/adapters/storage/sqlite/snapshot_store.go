package sqlite

import (
	"database/sql"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// SnapshotStore provides snapshot persistence for the inventory service.
type SnapshotStore struct {
	base
}

// NewSnapshotStore creates a new Snapshot store.
func NewSnapshotStore(db *sql.DB, logger observability.Logger) *SnapshotStore {
	return &SnapshotStore{base{db: db, logger: logger}}
}
