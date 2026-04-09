-- Clear all DH card ID mappings so the bulk match re-resolves every card
-- from scratch. Previously, cards with a mapping but dh_push_status='unmatched'
-- were permanently skipped by the bulk match process.
DELETE FROM card_id_mappings WHERE provider = 'doubleholo';

-- Reset all DH push statuses so every unsold purchase goes through fresh
-- resolution and push.
UPDATE campaign_purchases SET dh_push_status = ''
WHERE dh_push_status IN ('matched', 'unmatched');
