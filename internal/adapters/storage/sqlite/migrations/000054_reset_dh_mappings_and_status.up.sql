-- Clear all DH card ID mappings so the bulk match re-resolves every card
-- from scratch. Previously, cards with a mapping but dh_push_status='unmatched'
-- were permanently skipped by the bulk match process.
DELETE FROM card_id_mappings WHERE provider = 'doubleholo';

-- Reset all DH push statuses so every unsold purchase goes through fresh
-- resolution and push. Resetting 'matched' is intentional for a clean slate.
-- Duplicate pushes are prevented by the DHInventoryID != 0 guard in both
-- pushMatchedToDH (dh_match_handler.go) and processPurchase (dh_push.go),
-- which skip or fix status for cards that already have an inventory ID.
UPDATE campaign_purchases SET dh_push_status = ''
WHERE dh_push_status IN ('matched', 'unmatched');
