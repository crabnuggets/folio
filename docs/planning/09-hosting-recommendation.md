# First hosted version: Render + Supabase

Researched 2026-10-26 against official provider documentation. Status: selected by the user on 2026-10-26; see [accepted ADR 0004](../adr/0004-first-hosted-version.md). Unattended processing will be finalized during Gmail integration. No deployment has been provisioned.

## Selected first hosted version

Use one Render Free web service in Singapore for the Go API and compiled React assets; one Supabase Free project in Singapore for PostgreSQL and authentication; and a Google Cloud project for the application's Gmail OAuth configuration. Android uses the same API and needs no separate application server. Use a provider subdomain and HTTPS initially.

This minimizes services and preserves a conventional Go/PostgreSQL architecture. Supabase supplies identity and storage infrastructure; Go still owns all financial logic. Use Google login with an owner allowlist and separate consent for Gmail reading. Do not rely on application login alone to grant mailbox access.

## Services and responsibilities

| Need | Choice | Timing |
| --- | --- | --- |
| Web and API compute | Render Free, Go serving React build | First hosted version |
| Persistent relational data | Supabase Free PostgreSQL | First hosted version |
| Owner authentication | Supabase Auth | First hosted version |
| Gmail access | Google OAuth client and Gmail API | Email ingestion phase |
| Private document storage | Supabase Storage, only if retention is needed | Optional; transient parsing remains default |
| Scheduled processing | Unselected; start with on-demand sync | Finalize during P6 Gmail integration after evaluating cost and failure behavior |
| Backups | Encrypted exports and database dumps to user-controlled off-site storage | Before real data; automate subsequently |
| AI inference | Rules and optional local development model | No hosted inference service required |
| Android distribution | Signed personal APK | Android phase; no separate backend |

## Verified limits and implications

- [Render Free](https://render.com/docs/free): service sleeps after 15 idle minutes; waking takes about one minute. Local filesystem is ephemeral. Free instance hours total 750 per workspace/month. Bandwidth/build overages can incur charges with a payment method; without one, relevant services/builds are suspended instead. Render describes free instances as hobby/testing infrastructure, not production infrastructure. Do not use free Render PostgreSQL for durable finance records: it expires after 30 days.
- [Supabase pricing](https://supabase.com/pricing): Free includes 500 MB database, 50,000 monthly active users, 1 GB file storage and 5 GB egress. Projects can pause after one week of inactivity. These capacities appear ample for one user's summary records, but are not measured app usage estimates. Inactivity pausing is a real drawback for monthly-only use; plan an explicit dashboard restore procedure and test it. Do not assume it behaves like automatic request-triggered cold start.
- Both [Render](https://render.com/docs/regions) and [Supabase](https://supabase.com/docs/guides/platform/regions) list Singapore. Check free-instance capacity when creating the project. Co-location across vendors does not imply private networking or eliminate egress quotas.
- [Supabase backups](https://supabase.com/docs/guides/platform/backups) recommends regular exports and off-site backups for Free projects. Do not assume paid-plan daily backup access. Retained object-storage files need their own backup in addition to database data.
- [Supabase Cron](https://supabase.com/docs/guides/cron) supports scheduled SQL and HTTP calls. Treat this as an automation candidate, not proven end-to-end unattended delivery. Verify extension availability in the selected project, authenticated HTTP invocation, cold-start timeout/retry, paused-project behavior, and durable processing before selecting it. A scheduler inside a paused database cannot provide independent recovery. Genuine imports are useful work; artificial keepalive traffic is not the design.

## Alternatives retained for future evaluation

These options are not dependencies of the selected first hosted version. Supabase Cron is also unselected; its mention above records a capability to evaluate during Gmail integration.

**Google Cloud Run + Cloud Scheduler:** strongest alternative for learning managed container deployment and running scheduled bounded Go requests. Cloud Run can host the same Go/React container; PostgreSQL/auth can remain external. Use request-based billing and minimum instances zero, small connection pools, bounded batch size and retries. Never rely on a goroutine continuing after an HTTP response when CPU allocation is request-scoped.

[Cloud Run pricing](https://cloud.google.com/run/pricing) lists request-based monthly free allowances of 180,000 vCPU-seconds, 360,000 GiB-seconds, and two million requests, applied as a discount based on us-central1 pricing. Singapore is available, but region and networking charges matter. [Cloud Scheduler](https://cloud.google.com/scheduler/pricing) includes three jobs per billing account. A daily scan is one job, not a new job per execution.

This is free-allowance, usage-billed hosting, not a guaranteed zero invoice. [Google's free program](https://docs.cloud.google.com/free/docs/free-cloud-features) distinguishes trial credit from ongoing free usage. Include registry storage, builds, logs, secrets, and outbound traffic in a deployment estimate. Ordinary alerts-only [budgets](https://docs.cloud.google.com/billing/docs/how-to/budgets) do not cap spend; check eligibility and coverage separately if using newer spend-cap budgets. Replacing Render does not fix Supabase's inactivity pause or Gmail OAuth restrictions.

**Neon PostgreSQL:** useful database alternative if Supabase pausing proves disruptive. [Current pricing](https://neon.com/pricing) lists a no-credit-card Free plan with 1 GB/project, 100 CU-hours/project/month, and scale-to-zero after five inactive minutes. The page also lists managed authentication. Do not adopt the full stack solely from pricing: validate wake/reconnect behavior, region, Go connection pooling, owner authorization, and native Kotlin authentication before changing the existing Auth design. Independent backups remain required.

## Accepted decision and implementation checks

The user selected Render + Supabase for the first hosted version, with unattended processing to be finalized during Gmail integration. Accept recoverable cold starts and possible inactivity restoration explicitly; this is not an always-available promise. Keep deployment containerized and SQL portable so app compute or database can change later.

Before unattended Gmail becomes a requirement, prove the scheduler/database/worker combination can survive idle periods, token expiry, failures, and duplicate delivery. If this cannot be achieved on the selected free plans, compare Cloud Run/Scheduler and a different database rather than declaring automation complete.

No infrastructure was created. Before deployment: verify account-specific eligibility and quotas, avoid automatic paid upgrades, exercise restore and authenticated client access, confirm IPv4/IPv6 connection mode, and perform an encrypted backup restore drill. A $0 development target is feasible within limits; continuous availability at $0 is not established by this decision.
