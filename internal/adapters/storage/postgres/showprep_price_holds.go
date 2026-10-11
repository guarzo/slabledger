package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type showPrepPriceHold struct {
	purchaseID, certNumber, grader string
	firstDetectedAt                time.Time
}

func (s *ShowPrepStore) ObservePriceAssociations(ctx context.Context, ids []string) (map[string]bool, error) {
	var holds map[string]bool
	err := s.transaction(ctx, func(tx *showPrepSession) error {
		var err error
		holds, err = tx.ObservePriceAssociations(ctx, ids)
		return err
	})
	return holds, err
}
func (s *showPrepSession) ObservePriceAssociations(ctx context.Context, ids []string) (map[string]bool, error) {
	holds := map[string]bool{}
	if len(ids) == 0 {
		return holds, nil
	}
	if err := s.observePriceCollisions(ctx, ids); err != nil {
		return nil, err
	}
	rows, err := s.q.QueryContext(ctx, `SELECT purchase_id FROM showprep_price_holds WHERE purchase_id=ANY($1::text[])`, ids)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		holds[id] = true
	}
	return holds, rows.Err()
}

func (s *showPrepSession) observePriceCollisions(ctx context.Context, ids []string) error {
	// One SQL snapshot both observes live competitors and persists new holds.
	// Return every live collision, not only INSERT RETURNING: existing historical
	// hold rows must not hide a newly observed live collision in this session.
	rows, err := s.q.QueryContext(ctx, `WITH collisions AS MATERIALIZED (
 SELECT p.id,p.cert_number,p.grader FROM campaign_purchases p
 WHERE p.cert_number<>''
 AND p.cert_number IN (SELECT cert_number FROM campaign_purchases WHERE id=ANY($1::text[]))
 AND EXISTS(SELECT 1 FROM campaign_purchases other WHERE other.cert_number=p.cert_number AND other.id<>p.id)
 ), inserted AS (
 INSERT INTO showprep_price_holds(purchase_id,cert_number,grader)
 SELECT id,cert_number,grader FROM collisions
 ON CONFLICT(purchase_id) DO NOTHING
 RETURNING purchase_id,first_detected_at
 ) SELECT c.id,c.cert_number,c.grader,i.first_detected_at FROM collisions c
 LEFT JOIN inserted i ON i.purchase_id=c.id`, ids)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var hold showPrepPriceHold
		var insertedAt sql.NullTime
		if err := rows.Scan(&hold.purchaseID, &hold.certNumber, &hold.grader, &insertedAt); err != nil {
			return err
		}
		if s.observedCertCollisions == nil {
			s.observedCertCollisions = make(map[string]bool)
		}
		s.observedCertCollisions[hold.certNumber] = true
		if insertedAt.Valid {
			hold.firstDetectedAt = insertedAt.Time
			s.observedHolds = append(s.observedHolds, hold)
		}
	}
	return rows.Err()
}

func (s *showPrepSession) CertCollisionObserved(cert string) bool {
	return s.observedCertCollisions[cert]
}

func (s *showPrepSession) restorePriceHolds(ctx context.Context) error {
	for _, hold := range s.observedHolds {
		_, err := s.q.ExecContext(ctx, `INSERT INTO showprep_price_holds(purchase_id,cert_number,grader,first_detected_at)
 VALUES($1,$2,$3,$4) ON CONFLICT(purchase_id) DO NOTHING`, hold.purchaseID, hold.certNumber, hold.grader, hold.firstDetectedAt)
		if err != nil {
			return fmt.Errorf("preserve observed price ambiguity: %w", err)
		}
	}
	return nil
}
