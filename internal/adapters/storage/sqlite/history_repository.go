package sqlite

import (
	"github.com/guarzo/slabledger/internal/domain/inventory"
)

var (
	_ inventory.SnapshotHistoryRecorder   = (*SnapshotStore)(nil)
	_ inventory.PopulationHistoryRecorder = (*SnapshotStore)(nil)
	_ inventory.CLValueHistoryRecorder    = (*SnapshotStore)(nil)
)
