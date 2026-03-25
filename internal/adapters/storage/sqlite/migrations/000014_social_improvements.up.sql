-- Add error_message column for failed publishes
ALTER TABLE social_posts ADD COLUMN error_message TEXT NOT NULL DEFAULT '';

-- Migrate approved → draft (approve step is being removed)
UPDATE social_posts SET status = 'draft' WHERE status = 'approved';

-- Delete posts where ALL associated cards have no images (created before image filter)
DELETE FROM social_posts WHERE id IN (
    SELECT sp.id FROM social_posts sp
    WHERE sp.status NOT IN ('published', 'publishing')
    AND NOT EXISTS (
        SELECT 1 FROM social_post_cards spc
        JOIN campaign_purchases p ON p.id = spc.purchase_id
        WHERE spc.post_id = sp.id AND p.front_image_url <> ''
    )
);
