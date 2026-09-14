package showprep

import (
	"context"
	"time"
)

type Service struct {
	store  Store
	source Source
	now    func() time.Time
}

func NewService(store Store, source Source, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store, source, now}
}

type evaluationReader interface {
	PurchaseReader
	PriceAssociationStore
	EvidenceReader
}

func evaluateBatch(ctx context.Context, r evaluationReader, ids []string, now time.Time, failOnStorageError bool) ([]Evaluation, error) {
	purchases, purchaseErr := r.ReadPurchases(ctx, ids)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if purchaseErr != nil && failOnStorageError {
		return nil, purchaseErr
	}
	holds, holdErr := r.ObservePriceAssociations(ctx, ids)
	if holdErr != nil && failOnStorageError {
		return nil, holdErr
	}
	out := make([]Evaluation, 0, len(ids))
	identities := make([]Identity, 0, len(purchases))
	for _, p := range purchases {
		identities = append(identities, p.Identity())
	}
	snapshots, evidenceErr := r.ReadSnapshots(ctx, identities)
	if evidenceErr != nil && failOnStorageError {
		return nil, evidenceErr
	}
	for _, id := range ids {
		p, ok := purchases[id]
		if purchaseErr != nil {
			p = Purchase{ID: id}
		} else if !ok {
			p = Purchase{ID: id, Known: true}
		}
		p.PriceAssociationUnclear = holdErr != nil || holds[id]
		identity := p.Identity()
		snapshot := snapshots[identity]
		if evidenceErr != nil {
			snapshot = &Snapshot{Identity: identity, AttemptError: "Evidence storage unavailable"}
		}
		out = append(out, Evaluate(p, snapshot, now))
	}
	return out, nil
}
func (s *Service) Evaluate(ctx context.Context, ids []string) ([]Evaluation, error) {
	return evaluateBatch(ctx, s.store, ids, s.now(), false)
}
func (s *Service) Evidence(ctx context.Context, id string) (Evidence, error) {
	// Use one payload for both summary and detail, never a second evidence read.
	ps, err := s.store.ReadPurchases(ctx, []string{id})
	if err != nil {
		return Evidence{}, err
	}
	p, ok := ps[id]
	if !ok {
		return Evidence{}, ErrNotFound
	}
	holds, holdErr := s.store.ObservePriceAssociations(ctx, []string{id})
	p.PriceAssociationUnclear = holdErr != nil || holds[id]
	snapshots, err := s.store.ReadSnapshots(ctx, []Identity{p.Identity()})
	snap := snapshots[p.Identity()]
	if err != nil {
		snap = &Snapshot{Identity: p.Identity(), AttemptError: "Evidence storage unavailable"}
	}
	now := s.now()
	e := Evidence{Evaluation: Evaluate(p, snap, now), Sales: []Sale{}}
	if snap != nil {
		start, end := Window(now)
		seen := map[string]bool{}
		for _, sale := range snap.Sales {
			if sale.Date >= start && sale.Date <= end && !seen[sale.ID] {
				e.Sales = append(e.Sales, sale)
				seen[sale.ID] = true
			}
		}
	}
	return e, nil
}
