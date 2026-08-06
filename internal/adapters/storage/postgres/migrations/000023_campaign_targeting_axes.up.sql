ALTER TABLE campaigns
  ADD COLUMN target_language      TEXT  NOT NULL DEFAULT '',
  ADD COLUMN subject_filter_mode  TEXT  NOT NULL DEFAULT 'Target',
  ADD COLUMN subjects             JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN denied_specs         JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Backfill: subject_filter_mode mirrors the existing polarity bool.
UPDATE campaigns
SET subject_filter_mode = CASE WHEN exclusion_mode THEN 'Exclude' ELSE 'Target' END;

-- Backfill: subjects from inclusion_list, split on the same rule as
-- inventory.SplitInclusionList (comma-or-whitespace runs, empty entries
-- dropped). Ids are placeholders (0) — the operator resolves them via a
-- baseline pull or the getSubjects resolver; see design doc §7.
UPDATE campaigns c
SET subjects = COALESCE(
    (
        SELECT jsonb_agg(jsonb_build_object('id', 0, 'name', tok) ORDER BY ord)
        FROM unnest(regexp_split_to_array(trim(c.inclusion_list), '[,\s]+')) WITH ORDINALITY AS t(tok, ord)
        WHERE tok <> ''
    ),
    '[]'::jsonb
)
WHERE c.inclusion_list IS NOT NULL AND trim(c.inclusion_list) <> '';
