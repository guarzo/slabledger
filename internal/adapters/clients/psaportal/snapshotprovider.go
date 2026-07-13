package psaportal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
	"github.com/guarzo/slabledger/internal/domain/observability"
)

// ErrNoSnapshot indicates no rows snapshot has been stored yet by the
// psa-harvest job.
var ErrNoSnapshot = errors.New("psaportal: no rows snapshot stored")

// ErrStaleSnapshot indicates the stored rows snapshot is older than
// maxSnapshotAge.
var ErrStaleSnapshot = errors.New("psaportal: rows snapshot is stale")

// SnapshotStore reads the most recently harvested rows snapshot.
type SnapshotStore interface {
	CurrentSnapshot(ctx context.Context) (rows []map[string]string, fetchedAt time.Time, err error)
}

// SnapshotWriter persists a harvested rows snapshot (harvester side).
type SnapshotWriter interface {
	SaveSnapshot(ctx context.Context, rows []map[string]string, fetchedAt time.Time) error
}

// maxSnapshotAge is how stale the stored snapshot may be before FetchRows
// refuses it. The harvester refreshes hourly, so exceeding 26h means it has
// been broken for over a day and the sync should fail loudly, not import
// stale data silently.
const maxSnapshotAge = 26 * time.Hour

// SnapshotRowProvider serves PSA export rows from the DB snapshot written by
// the psa-harvest job. It performs no network calls — the Cloudflare-gated
// psacard.com hop lives entirely in the harvester.
type SnapshotRowProvider struct {
	store  SnapshotStore
	logger observability.Logger
	now    func() time.Time // test seam
}

func NewSnapshotRowProvider(store SnapshotStore, logger observability.Logger) *SnapshotRowProvider {
	return &SnapshotRowProvider{store: store, logger: logger, now: time.Now}
}

// FetchRows returns the mapped rows from the stored snapshot.
func (p *SnapshotRowProvider) FetchRows(ctx context.Context) ([]inventory.PSAExportRow, error) {
	raw, fetchedAt, err := p.store.CurrentSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if fetchedAt.IsZero() {
		return nil, fmt.Errorf("%w; psa-harvest job must run first", ErrNoSnapshot)
	}
	if age := p.now().Sub(fetchedAt); age > maxSnapshotAge {
		return nil, fmt.Errorf("%w (fetched %s ago); check the psa-harvest job", ErrStaleSnapshot, age.Round(time.Minute))
	}
	return mapRows(ctx, raw, p.logger)
}
