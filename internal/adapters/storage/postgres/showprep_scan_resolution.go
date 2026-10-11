package postgres

import "context"

// Resolution deliberately does not filter by availability or grader: even a sold,
// refunded or closed duplicate prevents unique exact-cert identification. The
// domain locks these candidates, re-queries, and compares identities before consent.
func (s *showPrepSession) ReadCertPurchaseIDs(ctx context.Context, cert string) ([]string, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id FROM campaign_purchases WHERE cert_number=$1 ORDER BY id`, cert)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
