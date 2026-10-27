# ADR 0001: Go API, React web, native Kotlin Android

Status: Proposed baseline for implementation. Date: 2026-10-26.

## Context

One person needs browser and Android access, wants Go learning opportunities, prefers conventional frontend technology, and wants third-party services with free tiers. Operational simplicity matters more than scale.

## Decision

Use a modular Go monolith, React with TypeScript and Vite, PostgreSQL, and native Android with Kotlin and Jetpack Compose. Use REST/JSON with an OpenAPI contract and generated client types. Start online-first; offline financial writes are deferred. Use Supabase Auth as the selected managed identity provider for the first hosted version and ordinary PostgreSQL access from Go. Local development must work without a cloud account.

Prefer standard `net/http`, `pgx`, `sqlc`, versioned SQL migrations, TanStack Query, React Hook Form, and a small accessible component set. Android uses ViewModel, coroutines/Flow, and a conventional HTTP client. Pin supported versions during setup rather than guessing future version numbers here.

Serve the built React bundle from Go initially to share an origin and reduce deployment complexity. Keep modules for accounts, monthly planning, statements, imports, and insights inside one service. A database-backed job queue can be added when imports require it; no Redis or message broker initially.

Hosting is accepted separately in [ADR 0004](0004-first-hosted-version.md): Render Free for Go/React and Supabase Free for PostgreSQL/Auth. Unattended processing is deferred until Gmail integration. Other platform details in this ADR retain their proposed status.

## Alternatives

- React Native shares language and some patterns with web, but native Kotlin/Compose fits the request to learn mainstream native Android. The cost is maintaining two UI implementations.
- A PWA alone is cheaper to maintain but does not fully meet the requested native Android target.
- Next.js is reasonable for server-rendered websites; this private dashboard has little SEO need and already has a Go backend.
- Firebase is viable, but PostgreSQL better supports reconciliation constraints and relational reporting while giving Go meaningful responsibility.
- SQLite is excellent locally, but a persistent PostgreSQL database avoids relying on ephemeral free-host disks and keeps development close to hosted operation.

## Consequences

Share contracts and test vectors, not UI components, between web and Android. Avoid microservices and full offline synchronization until demonstrated needs justify them. Free hosting may sleep; reliable unattended ingestion is conditional on a viable scheduler and availability. Revisit the ADR if native UI maintenance becomes the dominant burden.
