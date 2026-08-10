-- Buy-decision provenance, part 2 (see docs/superpowers/specs/2026-08-10-
-- buy-decision-provenance-design.md, defects D3/D5). Migration 000022 froze
-- cl_value_at_purchase_cents and cl_value_at_sale_cents but recorded neither
-- WHEN the value was observed relative to the purchase/sale, nor WHERE it
-- came from. Two different writers can land a value in cl_value_at_purchase_cents
-- -- the create-time copy of whatever CLValueCents the intake carried, and
-- CardLadder's own refresh sweep via UpdatePurchaseCLValue -- and today's
-- schema cannot tell which happened. The same ambiguity exists on the sale
-- side, where cl_value_at_sale_cents is copied from the purchase's current
-- CLValueCents with no record of whether CardLadder ever produced it.
--
-- These columns are purely additive: no existing column is touched, no
-- existing write path stops being valid mid-deploy. They give the write
-- paths (Task 2/3 of this change) somewhere to record source and timing;
-- this migration itself does not classify a single existing row.
--
-- NO BACKFILL. This is the governing principle of the whole design: a column
-- named `*_at_purchase` must contain a value observed near the purchase, or
-- nothing at all. It is better to leave a hole than to fill it with today's
-- number wearing a purchase-time label, because the second is undetectable
-- afterwards and silently corrupts every later analysis. The ~859 existing
-- frozen rows (both purchase- and sale-side) have unrecoverable provenance --
-- there is no way to know, after the fact, whether a given historical value
-- came from intake or from CardLadder, or how stale it was when frozen. They
-- are left at the '' default, which is the machine-checkable "unknown
-- provenance" marker: any study reading this data filters on
-- `cl_value_at_purchase_source <> ''` (or the sale-side equivalent) to
-- exclude this backlog rather than trusting the column name.
ALTER TABLE campaign_purchases
    ADD COLUMN cl_value_at_purchase_observed_at TEXT NOT NULL DEFAULT '',
    ADD COLUMN cl_value_at_purchase_source      TEXT NOT NULL DEFAULT ''
        CHECK (cl_value_at_purchase_source IN ('', 'intake', 'cardladder')),
    ADD COLUMN cl_card_confidence_at_purchase   SMALLINT;

ALTER TABLE campaign_sales
    ADD COLUMN cl_value_at_sale_observed_at TEXT NOT NULL DEFAULT '',
    ADD COLUMN cl_value_at_sale_source      TEXT NOT NULL DEFAULT ''
        CHECK (cl_value_at_sale_source IN ('', 'intake', 'cardladder'));

-- No rollback-window trigger, unlike 000022's campaign_sales_derive_reason_trg.
-- That trigger existed because a legacy-image INSERT omitting sale_reason
-- would land '', which was AMBIGUOUS with a real 'discretionary' default --
-- the trigger disambiguated by deriving the true reason from the boolean
-- column that both images write. Here there is no such ambiguity to resolve:
-- a legacy-image INSERT (the previous binary, mid-rollback, has no notion of
-- these columns at all) lands cl_value_at_purchase_source = '', and '' means
-- exactly "unknown provenance" -- which is precisely the correct, honest
-- outcome for a row written by code that predates this column existing. There
-- is nothing to derive it into. Migration 000040 made the identical call for
-- price_source, for the identical reason.
--
-- cl_card_confidence_at_purchase is nullable rather than NOT NULL DEFAULT '',
-- unlike the two TEXT columns: it is a SMALLINT, and 0 is a value CardLadder
-- can legitimately report (a real, if low, comp confidence), so it cannot
-- double as its own "unknown" sentinel the way '' does for TEXT. NULL is the
-- only encoding that does not collide with a real answer.
