package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/guarzo/slabledger/internal/domain/psaexchange"
)

// PSAExchangePolicyStore persists the single-row PSA-Exchange policy.
// The id column is locked to 1 by a CHECK constraint in migration 000008,
// so the table holds at most one row — Get/Set always operate on it.
type PSAExchangePolicyStore struct {
	db *sql.DB
}

// NewPSAExchangePolicyStore constructs a PSAExchangePolicyStore.
func NewPSAExchangePolicyStore(db *sql.DB) *PSAExchangePolicyStore {
	return &PSAExchangePolicyStore{db: db}
}

// Compile-time interface check.
var _ psaexchange.PolicyStore = (*PSAExchangePolicyStore)(nil)

// Get returns the persisted policy. The bool is false when no row exists yet
// — callers should fall back to their seed/default policy in that case.
func (s *PSAExchangePolicyStore) Get(ctx context.Context) (psaexchange.Policy, bool, error) {
	const q = `
		SELECT high_liquidity_velocity, high_liquidity_confidence,
		       high_liquidity_offer_pct, default_offer_pct,
		       min_confidence, min_quarter_velocity
		  FROM psa_exchange_policy
		 WHERE id = 1
	`
	var p psaexchange.Policy
	err := s.db.QueryRowContext(ctx, q).Scan(
		&p.HighLiquidityVelocity,
		&p.HighLiquidityConfidence,
		&p.HighLiquidityOfferPct,
		&p.DefaultOfferPct,
		&p.MinConfidence,
		&p.MinQuarterVelocity,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return psaexchange.Policy{}, false, nil
	}
	if err != nil {
		return psaexchange.Policy{}, false, fmt.Errorf("psaexchange policy: get: %w", err)
	}
	return p, true, nil
}

// Set upserts the single policy row.
func (s *PSAExchangePolicyStore) Set(ctx context.Context, p psaexchange.Policy) error {
	const q = `
		INSERT INTO psa_exchange_policy (
		    id, high_liquidity_velocity, high_liquidity_confidence,
		    high_liquidity_offer_pct, default_offer_pct,
		    min_confidence, min_quarter_velocity, updated_at
		) VALUES (1, $1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (id) DO UPDATE SET
		    high_liquidity_velocity   = EXCLUDED.high_liquidity_velocity,
		    high_liquidity_confidence = EXCLUDED.high_liquidity_confidence,
		    high_liquidity_offer_pct  = EXCLUDED.high_liquidity_offer_pct,
		    default_offer_pct         = EXCLUDED.default_offer_pct,
		    min_confidence            = EXCLUDED.min_confidence,
		    min_quarter_velocity      = EXCLUDED.min_quarter_velocity,
		    updated_at                = now()
	`
	if _, err := s.db.ExecContext(ctx, q,
		p.HighLiquidityVelocity,
		p.HighLiquidityConfidence,
		p.HighLiquidityOfferPct,
		p.DefaultOfferPct,
		p.MinConfidence,
		p.MinQuarterVelocity,
	); err != nil {
		return fmt.Errorf("psaexchange policy: set: %w", err)
	}
	return nil
}
