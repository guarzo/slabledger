-- A singleton lease/control record, not a selection-fed queue. Database time
-- owns lease expiry; UTC evidence windows use the worker's qualification clock.
CREATE TABLE showprep_worker (
 singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),
 owner TEXT NOT NULL DEFAULT '',
 epoch BIGINT NOT NULL DEFAULT 0 CHECK(epoch>=0),
 lease_until TIMESTAMPTZ,
 requested BOOLEAN NOT NULL DEFAULT false,
 retry_requested BOOLEAN NOT NULL DEFAULT false,
 active_retry BOOLEAN NOT NULL DEFAULT false,
 auth_hold BOOLEAN NOT NULL DEFAULT false,
 state TEXT NOT NULL DEFAULT 'idle' CHECK(state IN ('idle','running','failed','unconfigured','auth_hold')),
 error TEXT NOT NULL DEFAULT '',
 last_sweep_at TIMESTAMPTZ
);
INSERT INTO showprep_worker(singleton) VALUES(true);
-- Add scheduling metadata alongside the existing identity. Do not change the
-- retained payload, attempt generation or business/version projection.
ALTER TABLE showprep_evidence
 ADD COLUMN retry_window TEXT NOT NULL DEFAULT '',
 ADD COLUMN retry_attempts INTEGER NOT NULL DEFAULT 0 CHECK(retry_attempts BETWEEN 0 AND 3),
 ADD COLUMN retry_not_before TIMESTAMPTZ,
 ADD COLUMN retry_reset_epoch BIGINT NOT NULL DEFAULT 0;
ALTER TABLE showprep_worker ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE showprep_worker FROM PUBLIC;
DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='service_role') THEN
  CREATE POLICY "service role bypass" ON public.showprep_worker TO service_role USING (true) WITH CHECK (true);
  GRANT ALL ON TABLE public.showprep_worker TO service_role;
 END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN
  REVOKE ALL ON TABLE public.showprep_worker FROM anon;
 END IF;
 IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN
  REVOKE ALL ON TABLE public.showprep_worker FROM authenticated;
 END IF;
END $$;
