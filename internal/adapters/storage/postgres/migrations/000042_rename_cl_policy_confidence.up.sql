-- cl_confidence_at_purchase (added by migration 000022) does not hold what its
-- name implies: it is the campaign's configured policy minimum
-- (ParseCLConfidenceMin(campaign.CLConfidence) in service_crud.go, a targeting
-- parameter set at CreatePurchase), not an observation about the card. The
-- real per-card CardLadder confidence lives in a separate column,
-- cl_card_confidence_at_purchase (migration 000041). Keeping both a genuine
-- card-confidence column and a misleadingly-named policy column side by side
-- is the hazard this migration removes, by giving the policy column a name
-- that says what it is.
--
-- This is the EXPAND half of an expand/contract rename, not a plain RENAME
-- COLUMN. Migrations run automatically on startup, before the instance serves
-- traffic, so under a rolling deploy the first new instance to come up would
-- otherwise rename the column while old instances are still SELECTing the old
-- name -- every purchase read on an old instance would fail from the moment
-- this migration commits until the last old instance cycles out. Adding a new
-- column and dual-writing (the Go changes shipping with this migration) keeps both names valid for
-- the duration of the rollout. Deploy N+1 (SLA-106) is code-only: it removes
-- the last Go/TS reference to the old column but leaves the column in place,
-- because deploy-N instances are still SELECTing it by name for the whole of
-- that rollout. Deploy N+2 (SLA-106) drops the column, once no instance
-- referencing it remains -- see docs/superpowers/specs/
-- 2026-08-10-buy-decision-provenance-design.md, "Sequencing".
ALTER TABLE campaign_purchases
    ADD COLUMN cl_policy_confidence_min_at_purchase SMALLINT;

-- Backfill from the existing column so rows written before this deploy are
-- immediately readable under the new name. NULL stays NULL -- no legacy row
-- had a policy-confidence value backfilled from anywhere else.
UPDATE campaign_purchases
    SET cl_policy_confidence_min_at_purchase = cl_confidence_at_purchase
    WHERE cl_confidence_at_purchase IS NOT NULL;
