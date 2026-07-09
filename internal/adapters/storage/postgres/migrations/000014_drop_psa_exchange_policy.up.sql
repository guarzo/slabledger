-- 000014_drop_psa_exchange_policy: drop the persisted PSA-Exchange policy row.
-- The HTTP write surface (PUT /api/psa-exchange/policy) and PolicyDrawer UI
-- were removed in PR #8 (audit §1.9). EffectivePolicy() now returns the
-- env-seeded DefaultPolicy directly; no DB lookup. Operators still tune via
-- PSA_EXCHANGE_* env vars at process start.

DROP TABLE IF EXISTS public.psa_exchange_policy;
