-- SQLite does not support DROP COLUMN; recreate without mm_search_title.
CREATE TABLE mm_card_mappings_backup AS SELECT slab_serial, mm_collectible_id, mm_master_id, updated_at FROM mm_card_mappings;
DROP TABLE mm_card_mappings;
CREATE TABLE mm_card_mappings (
    slab_serial       TEXT    PRIMARY KEY,
    mm_collectible_id INTEGER NOT NULL,
    mm_master_id      INTEGER NOT NULL DEFAULT 0,
    updated_at        TEXT    NOT NULL DEFAULT ''
);
INSERT INTO mm_card_mappings (slab_serial, mm_collectible_id, mm_master_id, updated_at)
    SELECT slab_serial, mm_collectible_id, mm_master_id, updated_at FROM mm_card_mappings_backup;
DROP TABLE mm_card_mappings_backup;
