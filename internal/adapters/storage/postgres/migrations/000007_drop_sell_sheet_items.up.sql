-- Drop sell_sheet_items: the redesigned sell sheet operates on
-- inventory state (received + unsold) and persists nothing per user.

DROP INDEX IF EXISTS public.idx_sell_sheet_items_added_at;
DROP TABLE IF EXISTS public.sell_sheet_items CASCADE;
