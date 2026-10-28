-- Retry responses are wire artifacts: preserve their exact JSON body bytes.
-- JSONB canonicalizes nested JSON and cannot provide byte-identical replay.
ALTER TABLE folio.idempotency_records ALTER COLUMN response TYPE text USING response::text;
INSERT INTO folio.schema_migrations(version) VALUES (2) ON CONFLICT DO NOTHING;
