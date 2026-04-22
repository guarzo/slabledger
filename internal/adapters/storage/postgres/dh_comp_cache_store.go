package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/guarzo/slabledger/internal/domain/inventory"
)

// DHCompCacheRecord holds pre-aggregated DH sales analytics for a card+grade.
type DHCompCacheRecord struct {
	DHCardID          int
	Grade             string
	TotalSales        int
	RecentCount90d    int
	MedianCents       int
	AvgCents          int
	MinCents          int
	MaxCents          int
	PriceChange30dPct *float64
}

// DHCompCacheStore manages DH comp analytics cache and implements CompSummaryProvider.
type DHCompCacheStore struct {
	db *sql.DB
}

// NewDHCompCacheStore creates a new DH comp cache store.
func NewDHCompCacheStore(db *sql.DB) *DHCompCacheStore {
	return &DHCompCacheStore{db: db}
}

// UpsertCache inserts or updates a cached DH comp record.
func (s *DHCompCacheStore) UpsertCache(ctx context.Context, rec DHCompCacheRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO dh_comp_cache (dh_card_id, grade, total_sales, recent_count_90d, median_cents, avg_cents, min_cents, max_cents, price_change_30d_pct, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT(dh_card_id, grade) DO UPDATE SET
		   total_sales = excluded.total_sales,
		   recent_count_90d = excluded.recent_count_90d,
		   median_cents = excluded.median_cents,
		   avg_cents = excluded.avg_cents,
		   min_cents = excluded.min_cents,
		   max_cents = excluded.max_cents,
		   price_change_30d_pct = excluded.price_change_30d_pct,
		   updated_at = excluded.updated_at`,
		rec.DHCardID, rec.Grade, rec.TotalSales, rec.RecentCount90d,
		rec.MedianCents, rec.AvgCents, rec.MinCents, rec.MaxCents,
		rec.PriceChange30dPct, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert DH comp cache: %w", err)
	}
	return nil
}

// dhPurchaseInfo holds the DH card ID and grade for a purchase, keyed by cert.
type dhPurchaseInfo struct {
	dhCardID   int
	gradeValue float64
}

// lookupDHInfoBatch resolves DH card IDs and grades from campaign_purchases by cert.
func (s *DHCompCacheStore) lookupDHInfoBatch(ctx context.Context, certs []string) (map[string]dhPurchaseInfo, error) {
	out := make(map[string]dhPurchaseInfo, len(certs))
	if len(certs) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT cert_number, dh_card_id, grade_value FROM campaign_purchases
		 WHERE cert_number = ANY($1::text[]) AND dh_card_id > 0`,
		certs)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cert string
		var info dhPurchaseInfo
		if err := rows.Scan(&cert, &info.dhCardID, &info.gradeValue); err != nil {
			return nil, err
		}
		out[cert] = info
	}
	return out, rows.Err()
}

// gradeString formats a grade value to match the DH cache key format.
func gradeString(gradeValue float64) string {
	return fmt.Sprintf("%.0f", gradeValue)
}

// GetCompSummary returns a comp summary from the DH cache for one cert.
func (s *DHCompCacheStore) GetCompSummary(ctx context.Context, _ string, certNumber string) (*inventory.CompSummary, error) {
	if certNumber == "" {
		return nil, nil
	}
	var dhCardID int
	var gradeValue float64
	err := s.db.QueryRowContext(ctx,
		`SELECT dh_card_id, grade_value FROM campaign_purchases WHERE cert_number = $1 AND dh_card_id > 0`,
		certNumber,
	).Scan(&dhCardID, &gradeValue)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.fetchCached(ctx, dhCardID, gradeString(gradeValue))
}

// GetCompSummariesByKeys is the batch form.
func (s *DHCompCacheStore) GetCompSummariesByKeys(ctx context.Context, keys []inventory.CompKey) (map[inventory.CompKey]*inventory.CompSummary, error) {
	out := make(map[inventory.CompKey]*inventory.CompSummary)
	if len(keys) == 0 {
		return out, nil
	}

	certs := make([]string, 0, len(keys))
	for _, k := range keys {
		if k.CertNumber != "" {
			certs = append(certs, k.CertNumber)
		}
	}
	certToInfo, err := s.lookupDHInfoBatch(ctx, certs)
	if err != nil {
		return nil, fmt.Errorf("batch lookup DH info: %w", err)
	}

	// Group by (dh_card_id, grade) to deduplicate cache lookups.
	type cacheKey struct {
		dhCardID int
		grade    string
	}
	ckToKeys := make(map[cacheKey][]inventory.CompKey)
	var lookupKeys []cacheKey
	for _, k := range keys {
		info, ok := certToInfo[k.CertNumber]
		if !ok {
			continue
		}
		ck := cacheKey{dhCardID: info.dhCardID, grade: gradeString(info.gradeValue)}
		if _, seen := ckToKeys[ck]; !seen {
			lookupKeys = append(lookupKeys, ck)
		}
		ckToKeys[ck] = append(ckToKeys[ck], k)
	}
	if len(lookupKeys) == 0 {
		return out, nil
	}

	// Batch query the cache.
	dhIDs := make([]int, len(lookupKeys))
	grades := make([]string, len(lookupKeys))
	for i, lk := range lookupKeys {
		dhIDs[i] = lk.dhCardID
		grades[i] = lk.grade
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT dh_card_id, grade, total_sales, recent_count_90d, median_cents, avg_cents, min_cents, max_cents, price_change_30d_pct
		 FROM dh_comp_cache
		 WHERE (dh_card_id, grade) IN (SELECT * FROM UNNEST($1::int[], $2::text[]))`,
		dhIDs, grades)
	if err != nil {
		return nil, fmt.Errorf("DH cache batch: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var rec DHCompCacheRecord
		var pctChange sql.NullFloat64
		if err := rows.Scan(&rec.DHCardID, &rec.Grade, &rec.TotalSales, &rec.RecentCount90d,
			&rec.MedianCents, &rec.AvgCents, &rec.MinCents, &rec.MaxCents, &pctChange); err != nil {
			return nil, fmt.Errorf("scan DH cache: %w", err)
		}
		if rec.TotalSales == 0 {
			continue
		}
		ck := cacheKey{dhCardID: rec.DHCardID, grade: rec.Grade}
		sum := recordToCompSummary(&rec, pctChange)
		for _, k := range ckToKeys[ck] {
			cs := *sum
			out[k] = &cs
		}
	}
	return out, rows.Err()
}

func recordToCompSummary(rec *DHCompCacheRecord, pctChange sql.NullFloat64) *inventory.CompSummary {
	recentComps := rec.RecentCount90d
	if recentComps == 0 {
		recentComps = rec.TotalSales
	}
	var trend float64
	if pctChange.Valid {
		trend = pctChange.Float64 / 100
	}
	return &inventory.CompSummary{
		TotalComps:   rec.TotalSales,
		RecentComps:  recentComps,
		MedianCents:  rec.MedianCents,
		HighestCents: rec.MaxCents,
		LowestCents:  rec.MinCents,
		Trend90d:     trend,
	}
}

func (s *DHCompCacheStore) fetchCached(ctx context.Context, dhCardID int, grade string) (*inventory.CompSummary, error) {
	var rec DHCompCacheRecord
	var pctChange sql.NullFloat64
	err := s.db.QueryRowContext(ctx,
		`SELECT dh_card_id, grade, total_sales, recent_count_90d, median_cents, avg_cents, min_cents, max_cents, price_change_30d_pct
		 FROM dh_comp_cache WHERE dh_card_id = $1 AND grade = $2`,
		dhCardID, grade,
	).Scan(&rec.DHCardID, &rec.Grade, &rec.TotalSales, &rec.RecentCount90d,
		&rec.MedianCents, &rec.AvgCents, &rec.MinCents, &rec.MaxCents, &pctChange)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if rec.TotalSales == 0 {
		return nil, nil
	}
	return recordToCompSummary(&rec, pctChange), nil
}

var _ inventory.CompSummaryProvider = (*DHCompCacheStore)(nil)
