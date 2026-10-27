# ADR 0005: Transactional owner aggregate for the initial financial core

Status: Implementation decision for the initial single-owner backend. Date: 2026-10-27.

## Decision

Use the planned monorepo layout: Go in `services/api`, OpenAPI in `api`, shared
synthetic vectors in `fixtures`, future React and Android clients in `apps`.

For this first manual financial slice, persist the owner's typed financial state
as one PostgreSQL JSONB aggregate in a private `folio` schema. Serialize commands
with an owner-scoped transaction advisory lock, and commit the updated aggregate
and idempotent HTTP result in the same transaction. The memory adapter exists for
tests; the server requires PostgreSQL. Domain services validate references within
that owner aggregate; a client cannot supply an owner ID.

This refines the proposed normalized-table persistence outline in the planning
handoff. A single aggregate makes cross-record accounting invariants and rollback
explicit while the manual model is being established. It deliberately trades
row-level query efficiency for simpler transactional correctness at personal-use
volume. Financial state and audit history survive Render process restarts; they
never rely on its ephemeral filesystem.

## Consequences

All reads load the owner's state and all writes serialize for that owner. State,
audit, and indefinitely retained retry responses grow over time. This is suitable
for the initial single-owner manual tracker; it is not the final ingestion or
large transaction-feed storage design. Before provider ingestion materially grows
data volume, measure aggregate size and migrate the same domain services to
normalized owner-scoped tables if needed. Preserve export schema compatibility,
source identities, correction history and retry guarantees through that migration.

PostgreSQL constraints protect owner identity and idempotency uniqueness; the Go
aggregate enforces intra-owner financial references. RLS and revoked Data API role
privileges provide defense in depth. This does not claim that a privileged DB
connection is restricted by RLS. Future direct clients must continue using Go.

Hosted application session/PKCE UX and restore remain P3 work. This slice verifies
short-lived Supabase-compatible JWT bearer access and offers an explicit local
development token mode that refuses production startup. Gmail, parsing, and text
capture have contracted deferred endpoints and cannot post records.
