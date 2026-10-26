# Operations, security, and cost

## Cost policy

Required: every selected third-party service offers a free tier, and development works with no paid API usage. “Free tier” does not guarantee unlimited use, continuous availability, or permanent pricing. Render Free and Supabase Free are selected for the first hosted version in [ADR 0004](../adr/0004-first-hosted-version.md). The [hosting decision and comparison](09-hosting-recommendation.md) records verified provider documentation and material limits as of 2026-10-26. Unattended processing is deferred until Gmail integration. Record verified limits and date in the deployment decision before provisioning.

| Component | Selected starting point / deferred capability | Constraints to verify / fallback |
| --- | --- | --- |
| Local development | Go + React + local PostgreSQL | No hosted account needed; normal hardware/electricity cost |
| Go and web hosting | Render Free web service in Singapore, serving Go API and React bundle | Confirm free Go/container support, sleep/cold start, monthly compute and outbound quotas, region, and no automatic paid upgrades |
| Database | Supabase Free PostgreSQL in Singapore | Confirm database/storage/egress quotas, inactivity pausing, connection limits, region, restore/backup features; use pooled TLS connections as required |
| Authentication | Supabase Auth on the Free plan | Confirm Google login, redirects, token/session support, quotas; single invited owner and closed registration |
| File storage | No durable raw-file storage initially | Parse transiently, retain structured evidence; optional private object storage only after retention need established |
| Gmail integration | Gmail API under applicable quotas | OAuth and verification requirements may dominate feasibility; manual upload remains available |
| Scheduled imports | On-demand first; unattended mechanism unselected until P6 Gmail integration | Any external scheduler must itself have a suitable free tier and authenticated bounded calls; no exact SLA assumed |
| AI/transcription | Rules/templates, optional local inference | No hosted paid calls; browser/OS recognition availability and privacy vary |
| Android distribution | Locally built, signed APK for personal sideloading | Preserve signing key; verify current device/install/developer requirements; Play publication is not assumed free |
| Domain | Provider subdomain | Custom domain purchase excluded from zero-cash baseline |
| CI | Local checks, then repository's available GitHub Actions allowance | Verify private-repository minute/storage quotas; Android jobs may be expensive |

Do not use a short-lived trial database as durable storage. Do not store financial data on an ephemeral web-service disk. Do not keep a free host awake using artificial requests to evade provider limits. If free managed services cannot support a required feature, keep it manual or evaluate self-hosting on existing hardware; do not silently enable billing. Self-hosting adds uptime, patching, backups, and remote-access work and is not automatically free in practice.

## Deployment sequence

1. Verify current provider eligibility, quotas, allowed use, billing behavior, region, and recovery options. Prefer Singapore/nearby region where available; document any cross-region traffic and data residency tradeoffs.
2. Create local production-like configuration, run migrations and end-to-end synthetic checks. Bind production traffic only over HTTPS.
3. Configure single-owner auth, server secrets, TLS database connections, minimal database privileges, and fixed callback origins. Disable dev auth and public signup.
4. Deploy API/web with health/readiness endpoints that expose no finance data. Set small database pool, bounded requests, and graceful shutdown.
5. Prove browser and Android connectivity, cold-start recovery, and session renewal. Display connection errors without replacing totals with zeros.
6. Complete backup/restore drill before entering real records. Only then configure optional Gmail access.
7. Add scheduled imports only after a trigger/worker survives restarts and missed runs without duplicates. Show last successful sync and reconnect actions.

## Durable jobs

Use PostgreSQL job claims with leases and bounded retries, exponential backoff/jitter, dead-letter state, and unique source keys. Commit progress at message/document boundaries. Expired leases can be reclaimed safely after crashes. A sleeping host cannot promise scheduling; on-demand requests resume work or a separately verified trigger wakes a bounded batch. Do not keep a client request open for long parsing tasks. UI polls job status with backoff.

## Security controls

- Protect both web and Android with owner authentication. Possession of an unguessable URL is not authentication.
- Encrypt refresh tokens and any retained sensitive evidence at application level using a key held separately from the database. Use TLS in transit and provider encryption at rest. Document key rotation and recovery; never hardcode keys in clients.
- Store account aliases and optional last four digits, never full card numbers, CVV, banking passwords, or NRIC-based statement passwords.
- Exclude amounts, account numbers, email content, audio, OAuth codes, and tokens from routine logs. Log IDs, status codes, parser versions, and durations. Redact provider errors.
- Limit upload size, archive expansion, page count, parsing CPU/time, and MIME types. Use isolated parsers and keep dependencies patched.
- Use least-privilege Gmail scopes and no send/delete permission. Disconnect revokes or deletes stored credentials and stops future jobs.
- Protect mutations with CSRF for cookie auth, rate limits, idempotency, and version preconditions. Restrict CORS when origins are split.
- CSV exports escape formula-like text cells (`=`, `+`, `-`, `@` prefixes) without corrupting numeric amounts. Authenticated downloads use no-store headers and short-lived access if staged.
- Clear local session/cached financial data on logout. Android signing keys and encrypted backup keys require secure user-controlled backups.

## Retention and recovery

Default: delete temporary PDFs/audio immediately after processing or failure cleanup; add a cleanup sweep for crashed jobs. Retain minimal evidence fields needed to explain postings, source identifiers for deduplication, and the financial audit history. Persist raw document copies only with explicit retention settings and private storage. Provide “delete import evidence” separately from “void financial record.”

Export schema-versioned JSON for full fidelity and CSV for spreadsheet use. Include accounts, entries, statements/revisions, payment links, snapshots, plans, coverage assertions, reporting settings, import sources, and audit events; exclude active secrets. Build a dry-run import/restore path that preserves source identities and audit history and validates identities, references, currencies, and totals. Explain that mailbox authorization must be reconnected after a restore without secrets.

MVP backup: manual encrypted export after each monthly close to user-controlled storage. Hosted operation should add an automated encrypted PostgreSQL backup before claiming daily recovery coverage. Do not assume free-tier managed backups exist. Keep recovery keys outside the backup; schedule actual restore drills. Initial recovery objective is the last successful export, with manual recovery within one day; tighter objectives require implemented automation and a tested restore process.

Monitor last successful sync/backup, job failures, storage/egress use, and provider pausing. Alert on actionable failures, not every unchanged scan. Approaching quota disables optional ingestion/AI before core manual records; never auto-upgrade a plan. Keep migration history and reversible deployment procedures; destructive schema changes require a backup and tested migration path.
