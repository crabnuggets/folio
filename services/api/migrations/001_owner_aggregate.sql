-- One transactional aggregate per owner is deliberately used for the initial
-- single-owner service. Financial references and caps are validated in Go under
-- an owner transaction lock. This keeps retry, audit and posting commits atomic.
CREATE SCHEMA IF NOT EXISTS folio;
REVOKE ALL ON SCHEMA folio FROM PUBLIC;
CREATE TABLE IF NOT EXISTS folio.schema_migrations (
 version bigint PRIMARY KEY,
 applied_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS folio.owner_state (
 owner_id uuid PRIMARY KEY,
 document jsonb NOT NULL CHECK (jsonb_typeof(document) = 'object'),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS folio.idempotency_records (
 owner_id uuid NOT NULL REFERENCES folio.owner_state(owner_id),
 key uuid NOT NULL,
 request_hash text NOT NULL,
 response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner_id,key)
);
ALTER TABLE folio.owner_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE folio.owner_state FORCE ROW LEVEL SECURITY;
ALTER TABLE folio.idempotency_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE folio.idempotency_records FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS owner_isolation ON folio.owner_state;
CREATE POLICY owner_isolation ON folio.owner_state
 USING (owner_id::text = current_setting('folio.owner_id',true))
 WITH CHECK (owner_id::text = current_setting('folio.owner_id',true));
DROP POLICY IF EXISTS owner_isolation ON folio.idempotency_records;
CREATE POLICY owner_isolation ON folio.idempotency_records
 USING (owner_id::text = current_setting('folio.owner_id',true))
 WITH CHECK (owner_id::text = current_setting('folio.owner_id',true));
REVOKE ALL ON ALL TABLES IN SCHEMA folio FROM PUBLIC;
-- Supabase Data API roles must never access backend-owned financial tables.
DO $$
DECLARE role_name text;
BEGIN
 FOREACH role_name IN ARRAY ARRAY['anon','authenticated'] LOOP
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
   EXECUTE format('REVOKE ALL ON SCHEMA folio FROM %I',role_name);
   EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA folio FROM %I',role_name);
  END IF;
 END LOOP;
END $$;
INSERT INTO folio.schema_migrations(version) VALUES (1) ON CONFLICT DO NOTHING;
