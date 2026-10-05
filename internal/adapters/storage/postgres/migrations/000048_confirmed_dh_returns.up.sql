-- Durable, retained target bookkeeping: no expiry, queue or historical backfill.
CREATE TABLE confirmed_dh_returns (
 id TEXT PRIMARY KEY,
 idempotency_key TEXT NOT NULL UNIQUE CHECK(length(idempotency_key) BETWEEN 1 AND 255),
 purchase_id TEXT REFERENCES campaign_purchases(id) ON DELETE SET NULL,
 captured_purchase_id TEXT NOT NULL,
 dh_inventory_id BIGINT NOT NULL CHECK(dh_inventory_id>0),
 cert_number TEXT NOT NULL CHECK(cert_number<>''),
 grader TEXT NOT NULL CHECK(grader<>''),
 captured_sale_id TEXT,
 captured_order_id TEXT NOT NULL DEFAULT '',
 returned_order_id TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','conflicted','completed')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 completed_at TIMESTAMPTZ,
 listing_authorized_at TIMESTAMPTZ,
 error_code TEXT NOT NULL DEFAULT '',
 error_message TEXT NOT NULL DEFAULT '',
 error_phase TEXT NOT NULL DEFAULT '',
 observed_receipt JSONB,
 CHECK((state='completed')=(completed_at IS NOT NULL)),
 CHECK(listing_authorized_at IS NULL OR state='completed')
);
CREATE UNIQUE INDEX confirmed_dh_returns_unresolved_live ON confirmed_dh_returns(purchase_id) WHERE state<>'completed';
CREATE UNIQUE INDEX confirmed_dh_returns_unresolved_target ON confirmed_dh_returns(grader,cert_number) WHERE state<>'completed';
CREATE INDEX confirmed_dh_returns_target ON confirmed_dh_returns(grader,cert_number,created_at DESC);
CREATE INDEX confirmed_dh_returns_inventory ON confirmed_dh_returns(dh_inventory_id);
CREATE INDEX confirmed_dh_returns_order ON confirmed_dh_returns(grader,cert_number,returned_order_id) WHERE returned_order_id<>'';

CREATE TABLE dh_mutation_attempts (
 id TEXT PRIMARY KEY,
 purchase_id TEXT REFERENCES campaign_purchases(id) ON DELETE SET NULL,
 captured_purchase_id TEXT NOT NULL,
 dh_inventory_id BIGINT NOT NULL CHECK(dh_inventory_id>=0),
 cert_number TEXT NOT NULL,
 grader TEXT NOT NULL,
 kind TEXT NOT NULL,
 phase TEXT NOT NULL,
 idempotency_key TEXT NOT NULL DEFAULT '',
 payload_identity TEXT NOT NULL CHECK(payload_identity<>''),
 return_operation_id TEXT REFERENCES confirmed_dh_returns(id),
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 settled_at TIMESTAMPTZ,
 outcome TEXT NOT NULL DEFAULT 'open' CHECK(outcome IN ('open','succeeded','rejected')),
 receipt TEXT NOT NULL DEFAULT '',
 CHECK((outcome='open')=(settled_at IS NULL))
);
CREATE UNIQUE INDEX dh_mutation_attempts_open_target ON dh_mutation_attempts(grader,cert_number) WHERE outcome='open' AND cert_number<>'';
CREATE UNIQUE INDEX dh_mutation_attempts_open_inventory ON dh_mutation_attempts(dh_inventory_id) WHERE outcome='open' AND dh_inventory_id>0;
CREATE UNIQUE INDEX dh_mutation_attempts_open_purchase ON dh_mutation_attempts(captured_purchase_id) WHERE outcome='open';
CREATE TABLE dh_target_watermarks (
 grader TEXT NOT NULL,
 cert_number TEXT NOT NULL,
 dh_inventory_id BIGINT NOT NULL DEFAULT 0,
 mutation_started_at TIMESTAMPTZ,
 mutation_settled_at TIMESTAMPTZ,
 PRIMARY KEY(grader,cert_number)
);

DO $$
DECLARE t TEXT;
BEGIN
 FOREACH t IN ARRAY ARRAY['confirmed_dh_returns','dh_mutation_attempts','dh_target_watermarks'] LOOP
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
