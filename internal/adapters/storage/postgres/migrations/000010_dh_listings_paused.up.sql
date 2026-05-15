-- Add a global pause toggle for DH listing transitions.
-- When TRUE, the listing pipeline keeps items at in_stock instead of flipping
-- them to listed on DoubleHolo. Useful when selling everything at a card show
-- and items should be added to inventory without being listed.
ALTER TABLE dh_push_config
    ADD COLUMN listings_paused BOOLEAN NOT NULL DEFAULT FALSE;
