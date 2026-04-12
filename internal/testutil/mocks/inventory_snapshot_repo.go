package mocks

import "github.com/guarzo/slabledger/internal/domain/inventory"

// SnapshotRepositoryMock implements inventory.SnapshotRepository.
// This is a marker interface with no methods (see repository_snapshot.go).
type SnapshotRepositoryMock struct{}

var _ inventory.SnapshotRepository = (*SnapshotRepositoryMock)(nil)
