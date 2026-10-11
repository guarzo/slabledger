-- Keep legacy packed/acknowledged history unchanged. Removal invalidates older intents.
ALTER TABLE showprep_lists ADD COLUMN withdrawal_generation BIGINT NOT NULL
 DEFAULT 0 CHECK (withdrawal_generation BETWEEN 0 AND 9007199254740991);
CREATE TABLE showprep_command_receipts (
 command_id UUID PRIMARY KEY,
 list_id UUID NOT NULL REFERENCES showprep_lists(id) ON DELETE CASCADE,
 kind TEXT NOT NULL CHECK (kind IN ('scan','confirm_existing')),
 payload_hash TEXT NOT NULL,
 result JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Retired UUIDs never expire and have no FK to live shows or financial records.
CREATE TABLE showprep_retired_lists (
 list_id UUID PRIMARY KEY,
 delete_command_id UUID NOT NULL,
 payload_hash TEXT NOT NULL,
 retired_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
DECLARE t TEXT;
BEGIN
 FOREACH t IN ARRAY ARRAY['showprep_command_receipts','showprep_retired_lists'] LOOP
  EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY',t);
  EXECUTE format('REVOKE ALL ON TABLE public.%I FROM PUBLIC',t);
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='service_role') THEN
   EXECUTE format('CREATE POLICY "service role bypass" ON public.%I TO service_role USING (true) WITH CHECK (true)',t);
   EXECUTE format('GRANT ALL ON TABLE public.%I TO service_role',t);
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='anon') THEN
   EXECUTE format('REVOKE ALL ON TABLE public.%I FROM anon',t);
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='authenticated') THEN
   EXECUTE format('REVOKE ALL ON TABLE public.%I FROM authenticated',t);
  END IF;
 END LOOP;
END $$;
