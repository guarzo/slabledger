package inventory

// SnapshotRepository is a marker interface grouping snapshot-related operations.
// Snapshot persistence is actually delegated to PurchaseRepository
// (ListSnapshotPurchasesByStatus, UpdatePurchaseSnapshotStatus).
// This interface exists for future decomposition (Phase 2).
type SnapshotRepository interface {
	// Snapshot recording is handled by SnapshotHistoryRecorder (see history.go)
}
