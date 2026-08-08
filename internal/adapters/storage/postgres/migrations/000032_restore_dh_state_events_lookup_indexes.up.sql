-- Restore the per-purchase and per-cert lookup indexes on dh_state_events.
--
-- 000001 created both; 000003 dropped them (lines 270-271) because Supabase
-- reported zero scans. That was correct at the time: the store had exactly two
-- methods, Record and CountByTypeSince, and nothing in the codebase ever
-- filtered on purchase_id or cert_number. The indexes were dead weight.
--
-- SLA-58 adds the read path they were built for — DHEventStore.ByPurchase and
-- ByCert, surfaced at GET /api/dh/events — so the scans they serve now exist.
-- DDL is identical to 000003's own down migration (lines 55-56).
CREATE INDEX IF NOT EXISTS idx_dh_state_events_purchase ON dh_state_events(purchase_id, event_at);
CREATE INDEX IF NOT EXISTS idx_dh_state_events_cert ON dh_state_events(cert_number, event_at);
