-- Client-side weekly price trajectory aggregated from DH graded-sales-analytics.
-- One row per (card, Monday-anchored-week) — lets us compute CL-lag slope
-- over the trailing N weeks without waiting on a DH-native endpoint.
CREATE TABLE card_price_trajectory (
    dh_card_id TEXT NOT NULL,
    week_start TEXT NOT NULL,
    sale_count INTEGER NOT NULL,
    avg_price_cents INTEGER NOT NULL,
    median_price_cents INTEGER NOT NULL,
    refreshed_at TIMESTAMP NOT NULL,
    PRIMARY KEY (dh_card_id, week_start)
);

CREATE INDEX idx_card_price_trajectory_card ON card_price_trajectory(dh_card_id, week_start DESC);
