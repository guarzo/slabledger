-- 000074_dh_unlisted_detected_at.up.sql
-- Timestamp set by the DH reconciler when a purchase's dh_inventory_id is
-- missing from the authoritative DH inventory snapshot (i.e. someone deleted
-- the listing on the DH side). Cleared when the item is successfully re-listed.
-- NULL otherwise. Drives the "Re-list (removed from DH)" row badge.
ALTER TABLE campaign_purchases ADD COLUMN dh_unlisted_detected_at TIMESTAMP NULL;
