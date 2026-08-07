ALTER TABLE campaigns
  ADD COLUMN target_languages    JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN subject_filter_mode TEXT  NOT NULL DEFAULT 'Target',
  ADD COLUMN subjects            JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN denied_specs        JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Backfill: subject_filter_mode mirrors the existing polarity bool.
UPDATE campaigns
SET subject_filter_mode = CASE WHEN exclusion_mode THEN 'Exclude' ELSE 'Target' END;

-- Backfill: subjects from inclusion_list, split on comma-or-whitespace runs
-- with empty entries dropped. Ids are the legacy-unreconciled marker (-1,
-- matching inventory.LegacyUnreconciledSubjectID), NOT 0: id 0 already means
-- "operator typed this name, resolve it by name" and psacampaign's
-- translators act on that. Marking these -1 keeps them distinguishable, so
-- translation can refuse the campaign until the operator runs the baseline
-- pull instead of silently swapping live portal ids. See design doc §7.
UPDATE campaigns c
SET subjects = COALESCE(
    (
        SELECT jsonb_agg(jsonb_build_object('id', -1, 'name', tok) ORDER BY ord)
        FROM unnest(regexp_split_to_array(trim(c.inclusion_list), '[,\s]+')) WITH ORDINALITY AS t(tok, ord)
        WHERE tok <> ''
    ),
    '[]'::jsonb
)
WHERE c.inclusion_list IS NOT NULL AND trim(c.inclusion_list) <> '';

-- Constrain subject_filter_mode to the closed set the domain models. Defense in
-- depth against a direct SQL write or a future code path that skips
-- normalization: an unrecognized mode makes SubjectAxisMatches fall through to
-- Target semantics, silently inverting an Exclude campaign's attribution rather
-- than failing. Added after the backfill above so it validates rows the backfill
-- just wrote.
--
-- The empty string is in the set deliberately. It is a valid domain state, not a
-- gap: validSubjectFilterModes (inventory/validation.go) accepts it, and both
-- campaign_store.go's read and write paths run it through
-- normalizeSubjectFilterMode to Target. Excluding it here would make the
-- database reject a campaign ValidateAndNormalizeCampaign considers valid — a
-- constraint stricter than the contract it is meant to defend.
ALTER TABLE campaigns
  ADD CONSTRAINT campaigns_subject_filter_mode_check
  CHECK (subject_filter_mode IN ('Target', 'Exclude', ''));
