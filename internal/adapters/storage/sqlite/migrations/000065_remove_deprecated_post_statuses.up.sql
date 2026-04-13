-- Migrate deprecated 'approved' and 'rejected' social post statuses.
-- 'approved' → 'draft'   (it was a pre-publish approval step; draft is the correct equivalent)
-- 'rejected' → 'failed'  (it was a rejection of a draft; failed is the correct equivalent)
UPDATE social_posts SET status = 'draft'  WHERE status = 'approved';
UPDATE social_posts SET status = 'failed' WHERE status = 'rejected';
