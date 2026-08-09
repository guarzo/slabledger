-- Itemized-sale price provenance. In-person sales were previously recorded at
-- a guessed per-card price (cl_value_cents * a fixed pct), which is a
-- tautology: the "observed" comp was derived from the same market signal the
-- sale was supposed to validate. their_comp_cents captures the dealer's
-- actual sticker/offer for the card, independent of any CL/DH signal.
--
-- Both columns are NOT NULL DEFAULT, unlike 000022's sale_reason. That
-- migration needed a rollback-window trigger because a legacy-shaped INSERT
-- (no sale_reason column) would otherwise land as '', which was ambiguous
-- with a real 'discretionary' default. Here there is no such trigger: a
-- rollback-window row landing as their_comp_cents=0, price_source='' is
-- semantically correct under this scheme -- '' already means "unknown", and
-- 0 is never asserted to be a real comp because nothing reads
-- their_comp_cents unless price_source says how it was obtained.
ALTER TABLE campaign_sales
    ADD COLUMN their_comp_cents BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN price_source     TEXT NOT NULL DEFAULT ''
        CHECK (price_source IN ('', 'itemized', 'estimated', 'manual'));

-- Backfill price_source for existing rows from what is already derivable.
-- Does NOT touch their_comp_cents: synthesizing it from cl_value_cents * pct
-- would recreate the exact tautology this migration exists to break, for
-- every historical row.
--
-- order_id is DH's own order identifier (present only for sales DH itemized
-- through its order feed), so its presence is a direct, non-inferred signal.
UPDATE campaign_sales SET price_source = 'itemized'
    WHERE order_id != '';

-- In-person/card-show sales with no order_id were priced from the
-- CL-value-times-pct heuristic (the behavior this ticket removes going
-- forward). Marking them 'estimated' records that provenance honestly rather
-- than leaving them indistinguishable from a channel where the price was
-- never estimated at all.
UPDATE campaign_sales SET price_source = 'estimated'
    WHERE price_source = ''
      AND sale_channel IN ('inperson', 'cardshow');

-- Every other historical row (e.g. ebay/tcgplayer sales with no order_id)
-- keeps price_source = '' (unknown). Do not guess 'manual' for the
-- remainder -- that would manufacture a claim about rows this migration has
-- no evidence for.
