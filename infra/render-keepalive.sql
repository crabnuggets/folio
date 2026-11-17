-- Run once in the hosted Folio Supabase SQL Editor as postgres.
-- Re-running updates these named jobs. Supabase Cron uses GMT/UTC.
-- Singapore is UTC+8: prewarm at 09:55, then ping 10:00 through 00:55.
-- After the last ping, Render can sleep once its idle timeout expires.
-- This is best effort; it does not guarantee exemption from Supabase pausing.
BEGIN;

CREATE EXTENSION IF NOT EXISTS pg_cron WITH SCHEMA pg_catalog;
CREATE EXTENSION IF NOT EXISTS pg_net WITH SCHEMA extensions;
GRANT USAGE ON SCHEMA cron TO postgres;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA cron TO postgres;

SELECT cron.schedule(
  'folio-render-prewarm',
  '55 1 * * *',
  $$ SELECT net.http_get(
    url := 'https://folio-j4md.onrender.com/readyz',
    timeout_milliseconds := 120000
  ); $$
);

SELECT cron.schedule(
  'folio-render-keepalive',
  '*/5 2-16 * * *',
  $$ SELECT net.http_get(
    url := 'https://folio-j4md.onrender.com/readyz',
    timeout_milliseconds := 120000
  ); $$
);

-- pg_cron retains run history indefinitely. Keep seven days for these jobs only.
SELECT cron.schedule(
  'folio-keepalive-history-cleanup',
  '0 18 * * *',
  $$ DELETE FROM cron.job_run_details
     WHERE end_time < now() - interval '7 days'
       AND jobid IN (
         SELECT jobid FROM cron.job WHERE jobname IN (
           'folio-render-prewarm',
           'folio-render-keepalive',
           'folio-keepalive-history-cleanup'
         )
       ); $$
);

COMMIT;

-- Manage/disable these jobs in Integrations > Cron in the Supabase dashboard.
-- HTTP requests are asynchronous: a successful cron run only confirms enqueueing.
-- To inspect recent HTTP results, run:
-- SELECT id, status_code, timed_out, error_msg, created
-- FROM net._http_response ORDER BY created DESC LIMIT 10;
