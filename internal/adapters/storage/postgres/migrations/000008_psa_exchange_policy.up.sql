-- 000008_psa_exchange_policy: persist tunable PSA-Exchange scoring/filter levers
-- so admins can adjust offer percentages and tier thresholds from the UI.
-- Single-row table (id constrained to 1) — the active policy is always row 1.
-- When the row is absent the service falls back to PSA_EXCHANGE_* env vars,
-- which fall back to psaexchange.DefaultPolicy().

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
