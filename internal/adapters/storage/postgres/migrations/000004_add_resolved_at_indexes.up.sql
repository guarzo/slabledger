-- Indexes suggested by Supabase index advisor (round 2).
-- Covers two queries filtering on resolved_at IS NULL.

-- price_flags: WHERE resolved_at IS NULL GROUP BY purchase_id
CREATE INDEX IF NOT EXISTS idx_price_flags_resolved_at
    ON public.price_flags USING btree (resolved_at);

-- psa_pending_items: WHERE resolved_at IS NULL ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_psa_pending_items_resolved_at
    ON public.psa_pending_items USING btree (resolved_at);
