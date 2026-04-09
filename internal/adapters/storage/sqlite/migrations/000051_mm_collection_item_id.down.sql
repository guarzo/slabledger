-- SQLite does not support DROP COLUMN prior to 3.35.0; recreate without mm_collection_item_id.
CREATE TABLE mm_card_mappings_backup AS SELECT slab_serial, mm_collectible_id, mm_master_id, updated_at, mm_search_title FROM mm_card_mappings;
DROP TABLE mm_card_mappings;
CREATE TABLE mm_card_mappings (
    slab_serial       TEXT    PRIMARY KEY,
    mm_collectible_id INTEGER NOT NULL,
    mm_master_id      INTEGER NOT NULL DEFAULT 0,
    updated_at        TEXT    NOT NULL DEFAULT '',
    mm_search_title   TEXT    NOT NULL DEFAULT ''
);
INSERT INTO mm_card_mappings (slab_serial, mm_collectible_id, mm_master_id, updated_at, mm_search_title)
    SELECT slab_serial, mm_collectible_id, mm_master_id, updated_at, mm_search_title FROM mm_card_mappings_backup;
DROP TABLE mm_card_mappings_backup;
