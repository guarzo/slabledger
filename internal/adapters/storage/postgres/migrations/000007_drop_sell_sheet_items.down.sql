-- Restore sell_sheet_items table to the state left by migration 000003.

CREATE TABLE IF NOT EXISTS public.sell_sheet_items (
    purchase_id TEXT NOT NULL PRIMARY KEY,
    added_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE public.sell_sheet_items ENABLE ROW LEVEL SECURITY;
CREATE POLICY "service role bypass" ON public.sell_sheet_items
    USING (true) WITH CHECK (true);

CREATE INDEX IF NOT EXISTS idx_sell_sheet_items_added_at
    ON public.sell_sheet_items USING btree (added_at);
