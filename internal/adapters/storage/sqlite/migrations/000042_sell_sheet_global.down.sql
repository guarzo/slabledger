CREATE TABLE sell_sheet_items_old (
    user_id     INTEGER NOT NULL,
    purchase_id TEXT    NOT NULL,
    added_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, purchase_id)
);

INSERT OR IGNORE INTO sell_sheet_items_old (user_id, purchase_id, added_at)
    SELECT 0, purchase_id, added_at FROM sell_sheet_items;

DROP TABLE sell_sheet_items;
ALTER TABLE sell_sheet_items_old RENAME TO sell_sheet_items;
CREATE INDEX idx_sell_sheet_items_user ON sell_sheet_items(user_id);
