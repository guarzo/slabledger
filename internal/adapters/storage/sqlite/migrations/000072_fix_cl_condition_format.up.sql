-- Rewrite cl_card_mappings.cl_condition rows stored in the Firestore "g9" form
-- into the display "PSA 9" form. The pricing path keys the catalog lookup by
-- cl_condition (see catalogValueKey in scheduler/cardladder_helpers.go) and the
-- catalog returns condition in the "PSA 9" form, so any row in the "g9" form
-- misses every catalog hit and never gets a cl_value_cents applied.
--
-- Root cause: pushSingleCard (scheduler/cardladder_sync.go + handler
-- cardladder_sync.go) was passing result.GemRateCondition ("g9") instead of
-- result.Condition ("PSA 9") into SaveMapping, overwriting the correct value
-- written by the pricing refresh Phase 1 (SaveMappingPricing). That writer is
-- fixed in this same change; this migration backfills rows corrupted before
-- the fix.
--
-- Grader is PSA in this codebase (enforced by the campaign_purchases.grader
-- CHECK constraint + the default). The mapping table does not track grader,
-- but every "g*" row joins to a PSA purchase row.

UPDATE cl_card_mappings
SET cl_condition = CASE cl_condition
    WHEN 'g10'  THEN 'PSA 10'
    WHEN 'g9'   THEN 'PSA 9'
    WHEN 'g8_5' THEN 'PSA 8.5'
    WHEN 'g8'   THEN 'PSA 8'
    WHEN 'g7_5' THEN 'PSA 7.5'
    WHEN 'g7'   THEN 'PSA 7'
    WHEN 'g6_5' THEN 'PSA 6.5'
    WHEN 'g6'   THEN 'PSA 6'
    WHEN 'g5_5' THEN 'PSA 5.5'
    WHEN 'g5'   THEN 'PSA 5'
    WHEN 'g4_5' THEN 'PSA 4.5'
    WHEN 'g4'   THEN 'PSA 4'
    WHEN 'g3'   THEN 'PSA 3'
    WHEN 'g2'   THEN 'PSA 2'
    WHEN 'g1'   THEN 'PSA 1'
    ELSE cl_condition
END,
updated_at = datetime('now')
WHERE cl_condition LIKE 'g%';
