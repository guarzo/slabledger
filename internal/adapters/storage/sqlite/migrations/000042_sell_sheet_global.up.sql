-- Make sell_sheet_items global (remove user_id scoping).
-- Preserve existing items by picking user_id=0 rows (API token) or any row on conflict.
CREATE TABLE sell_sheet_items_new (
    purchase_id TEXT     NOT NULL PRIMARY KEY,
    added_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO sell_sheet_items_new (purchase_id, added_at)
    SELECT purchase_id, added_at FROM sell_sheet_items;

DROP TABLE sell_sheet_items;
ALTER TABLE sell_sheet_items_new RENAME TO sell_sheet_items;
