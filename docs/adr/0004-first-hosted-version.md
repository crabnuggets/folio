# ADR 0004: Render and Supabase for the first hosted version

Status: Accepted by the user. Date: 2026-10-26.

## Context

Folio needs browser and Android access, a Go backend, and services with free tiers. The user selected Render + Supabase after reviewing their free-tier constraints, and explicitly deferred the unattended-processing choice until Gmail integration.

## Decision

- Run the Go API and compiled React frontend together on one Render Free web service, targeting Singapore.
- Use one Supabase Free project, targeting Singapore, for PostgreSQL and Supabase Auth. Both clients use the same authenticated Go API; Go owns financial calculations and writes.
- Use a provider subdomain with HTTPS initially. Local development remains independent of hosted accounts.
- Keep document storage optional; use private Supabase Storage only if retained documents are needed. Persistent records must not live on Render's ephemeral filesystem.
- Finalize unattended processing during P6 (Gmail integration). No scheduler, background-worker host, or push infrastructure is selected by this decision. Start Gmail integration with on-demand sync.
- Retain Cloud Run/Cloud Scheduler, Supabase Cron, and Neon as evaluated alternatives, not first-version dependencies. A future change to selected hosting must be recorded in a new or superseding decision.

## Consequences and implementation gates

Render cold starts and Supabase inactivity pausing must be visible and recoverable. Do not promise continuous availability or guaranteed zero invoices beyond free-plan limits. Verify current account-specific quotas, region availability, and billing configuration before provisioning. Maintain encrypted off-site exports and prove restoration before storing real financial records.

P3 implements the selected stack and validates auth, TLS/database connectivity, cold-start handling, pause/recovery procedures, and backup restoration. P6 evaluates the unattended-processing mechanism against cost, idle/restart behavior, retries, OAuth longevity, and deduplication. Preserve manual/on-demand fallback if an acceptable mechanism is not yet available.

This accepts the hosting choice only; it does not mark deployment or integrations as implemented. No infrastructure was provisioned as part of the documentation update.

## References

- [Hosting decision and comparison](../planning/09-hosting-recommendation.md): dated provider evidence and alternatives.
- [Operations and cost](../planning/05-operations-and-cost.md): implementation controls and recovery.
- [Platform ADR](0001-platform-and-delivery.md): supplemented by this accepted hosting decision.
