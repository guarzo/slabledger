-- Drop the five dead blob columns from dh_card_cache (SLA-65).
--
-- SLA-41 narrowed this table's entire read/write path to seven columns: the
-- INSERT ... ON CONFLICT in UpsertCardCache, the SELECT in GetCardCache, and
-- scanCardCacheRow all stop at character_name / demand_score /
-- demand_data_quality / the three timestamps / the key. Nothing in Go has
-- named demand_json, velocity_json, trend_json, saturation_json or
-- price_distribution_json since. This is the contract half of that
-- expand/contract pair.
--
-- Deliberately scoped to dh_card_cache. dh_character_cache has its own
-- demand_json / velocity_json / saturation_json and they are still live --
-- SLA-41 typed their decode (demand.CharacterDemand, CharacterVelocity,
-- CharacterSaturation) but kept reading and writing them. Dropping those would
-- take out the character leaderboard.
--
-- This must NOT ship in the same release as SLA-41. Rolling back the app image
-- does not undo a migration (docs/OPERATIONS.md:87), and this repository
-- auto-deploys on every push to main, so deploy -> migrate -> roll back is the
-- ordinary recovery path rather than an edge case. A pre-SLA-41 binary running
-- against a post-000036 schema would issue SELECT ... demand_json ... against
-- columns that no longer exist and fail the card cache path outright. The one
-- release of separation is what keeps that rollback survivable.
--
-- Data loss here is immaterial, which is why this needs none of the
-- export-before-merge warning 000030 and 000035 carry. dh_card_cache is a
-- regenerable cache fed by the daily DH analytics refresh, and these five
-- columns have not been written since SLA-41 -- whatever they still hold is
-- stale by definition and unreachable by any code path.
--
-- No dependent objects: there are no views over dh_card_cache, and neither
-- surviving index touches these columns (idx_card_cache_demand_score is on
-- demand_score and was dropped in 000003 as unused; 000033's
-- idx_card_cache_character_name is partial on character_name). So no CASCADE is
-- needed, matching the explicit-drop style of 000030 and 000035. RLS is
-- unaffected -- 000028's policy is table-scoped, not column-scoped.

ALTER TABLE dh_card_cache
    DROP COLUMN IF EXISTS demand_json,
    DROP COLUMN IF EXISTS velocity_json,
    DROP COLUMN IF EXISTS trend_json,
    DROP COLUMN IF EXISTS saturation_json,
    DROP COLUMN IF EXISTS price_distribution_json;
