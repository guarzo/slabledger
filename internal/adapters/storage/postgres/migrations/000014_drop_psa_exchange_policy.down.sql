-- Rollback: restore the psa_exchange_policy table to its pre-000014 state,
-- mirroring the original 000008_psa_exchange_policy.up.sql definition
-- (single-row table with CHECK constraints + RLS).

CREATE TABLE IF NOT EXISTS public.psa_exchange_policy (
    id                          SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    high_liquidity_velocity     INTEGER          NOT NULL CHECK (high_liquidity_velocity   >= 0),
    high_liquidity_confidence   INTEGER          NOT NULL CHECK (high_liquidity_confidence >= 0 AND high_liquidity_confidence <= 10),
    high_liquidity_offer_pct    DOUBLE PRECISION NOT NULL CHECK (high_liquidity_offer_pct  >  0 AND high_liquidity_offer_pct  <= 1),
    default_offer_pct           DOUBLE PRECISION NOT NULL CHECK (default_offer_pct         >  0 AND default_offer_pct         <= 1),
    min_confidence              INTEGER          NOT NULL CHECK (min_confidence            >= 0 AND min_confidence            <= 10),
    min_quarter_velocity        INTEGER          NOT NULL CHECK (min_quarter_velocity      >= 0),
    updated_at                  TIMESTAMPTZ      NOT NULL DEFAULT now()
);

ALTER TABLE public.psa_exchange_policy ENABLE ROW LEVEL SECURITY;
