-- Show planning is independent of financial/source-record lifetimes.
CREATE TABLE showprep_lists (
 id UUID PRIMARY KEY,
 name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE showprep_items (
 id UUID PRIMARY KEY,
 list_id UUID NOT NULL REFERENCES showprep_lists(id) ON DELETE CASCADE,
 purchase_id TEXT NOT NULL,
 card_name TEXT NOT NULL,
 cert_number TEXT NOT NULL,
 grader TEXT NOT NULL,
 grade DOUBLE PRECISION NOT NULL,
 added_at TIMESTAMPTZ NOT NULL,
 packed_at TIMESTAMPTZ,
 version BIGINT NOT NULL CHECK (version>0),
 acknowledged_price_cents BIGINT NOT NULL,
 acknowledged_status TEXT NOT NULL,
 last_command TEXT NOT NULL DEFAULT '',
 UNIQUE(list_id,purchase_id)
);
CREATE TABLE showprep_evidence (
 identity_key TEXT PRIMARY KEY,
 profile_id TEXT NOT NULL,
 grader TEXT NOT NULL,
 grade DOUBLE PRECISION NOT NULL,
 payload JSONB,
 attempt BIGINT NOT NULL CHECK (attempt>0),
 attempt_state TEXT NOT NULL CHECK (attempt_state IN ('running','complete','partial','failed')),
 attempt_error TEXT NOT NULL DEFAULT '',
 attempt_started_at TIMESTAMPTZ NOT NULL,
 UNIQUE(profile_id,grader,grade)
);
-- No FK, expiry, or public clearing operation: deleting a competitor is not
-- identity-safe verification of the surviving DH price association.
CREATE TABLE showprep_price_holds (
 purchase_id TEXT PRIMARY KEY,
 cert_number TEXT NOT NULL,
 grader TEXT NOT NULL,
 first_detected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE showprep_lists ENABLE ROW LEVEL SECURITY;
ALTER TABLE showprep_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE showprep_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE showprep_price_holds ENABLE ROW LEVEL SECURITY;
DO $$
DECLARE tab TEXT;
BEGIN
 FOREACH tab IN ARRAY ARRAY['showprep_lists','showprep_items','showprep_evidence','showprep_price_holds'] LOOP
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='service_role') THEN
   EXECUTE format('CREATE POLICY "service role bypass" ON public.%I TO service_role USING (true) WITH CHECK (true)',tab);
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN
   EXECUTE format('REVOKE ALL ON TABLE public.%I FROM anon',tab);
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN
   EXECUTE format('REVOKE ALL ON TABLE public.%I FROM authenticated',tab);
  END IF;
 END LOOP;
END $$;
