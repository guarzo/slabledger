-- dh_state_events: append-only audit log for DH pipeline state transitions.
-- Used to debug drift between local state and what DH reports, and to drive
-- the orders-ingest health fields on /api/dh/status.
CREATE TABLE dh_state_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    purchase_id TEXT,                          -- nullable: orphan events have no local purchase
    cert_number TEXT,                          -- always populated when known, even for orphans
    event_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_type TEXT NOT NULL,                  -- enrolled | pushed | listed | channel_synced | sold | orphan_sale | already_sold | held | dismissed | unmatched | card_id_resolved
    prev_push_status TEXT,
    new_push_status TEXT,
    prev_dh_status TEXT,
    new_dh_status TEXT,
    dh_inventory_id INTEGER,
    dh_card_id INTEGER,
    dh_order_id TEXT,                          -- TEXT to match campaign_sales.order_id type
    sale_price_cents INTEGER,
    source TEXT NOT NULL,                      -- dh_orders_poll | dh_inventory_poll | cert_intake | cl_import | psa_import | manual_ui | cl_refresh | dh_listing
    notes TEXT
);

CREATE INDEX idx_dh_state_events_purchase ON dh_state_events(purchase_id, event_at);
CREATE INDEX idx_dh_state_events_cert ON dh_state_events(cert_number, event_at);
CREATE INDEX idx_dh_state_events_type_time ON dh_state_events(event_type, event_at DESC);
