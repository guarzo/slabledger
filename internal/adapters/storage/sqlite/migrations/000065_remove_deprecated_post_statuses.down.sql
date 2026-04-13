-- No-op: we cannot safely restore original 'approved'/'rejected' statuses
-- because the distinction between draft/failed and approved/rejected is ambiguous.
-- This migration is intentionally irreversible.
SELECT 1;
