package showprep

import (
	"context"
	"errors"
	"time"
)

// Refresh reserves a longer-lived request child for attempt/result persistence.
// A cancellation cannot cause detached work; the committed start remains visible.
func (s *Service) Refresh(ctx context.Context, ids []string, sourceDeadline, persistenceDeadline time.Time) ([]Evaluation, error) {
	persist, cancelPersist := context.WithDeadline(ctx, persistenceDeadline)
	defer cancelPersist()
	source, cancelSource := context.WithDeadline(ctx, sourceDeadline)
	defer cancelSource()
	purchases, err := s.store.ReadPurchases(persist, ids)
	if err != nil {
		return nil, err
	}
	seen := map[Identity]bool{}
	for _, id := range ids {
		p, ok := purchases[id]
		if !ok || !p.Identity().Valid() {
			continue
		}
		identity := p.Identity()
		if seen[identity] {
			continue
		}
		seen[identity] = true
		now := s.now()
		attempt, err := s.store.BeginAttempt(persist, identity, now)
		if err != nil {
			return nil, err
		}
		var snap Snapshot
		switch {
		case source.Err() != nil:
			err = source.Err()
		case s.source == nil:
			err = errors.New("CardLadder credentials unavailable")
		default:
			snap, err = s.source.Fetch(source, identity, now)
		}
		snap.Identity = identity
		if err != nil {
			snap.Complete = false
			// Never persist provider errors containing request URLs or credentials.
			if snap.AttemptError == "" {
				snap.AttemptError = "CardLadder refresh failed"
			}
			if errors.Is(err, context.DeadlineExceeded) || source.Err() == context.DeadlineExceeded {
				snap.AttemptError = "CardLadder source timeout"
			}
			if s.source == nil {
				snap.AttemptError = "CardLadder credentials unavailable"
			}
		}
		if !snap.Complete && snap.AttemptError == "" {
			snap.AttemptError = "Incomplete CardLadder window"
		}
		if persist.Err() != nil {
			return nil, persist.Err()
		}
		if err := s.store.FinishAttempt(persist, identity, attempt, snap); err != nil {
			return nil, err
		}
	}
	return evaluateBatch(persist, s.store, ids, s.now(), true)
}
