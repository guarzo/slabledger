CREATE TABLE sell_sheet_items (
    user_id     INTEGER NOT NULL,
    purchase_id TEXT    NOT NULL,
    added_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, purchase_id)
);
CREATE INDEX idx_sell_sheet_items_user ON sell_sheet_items(user_id);
