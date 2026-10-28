# Implementation handoff

Prepared 2026-10-26. These documents distinguish accepted decisions from proposed implementation details. Render + Supabase is accepted for the first hosted version; unattended processing remains deferred until Gmail integration.

## Read in this order

1. [Product and experience](01-product.md): scope, screens, assumptions, and success criteria.
2. [Accounting and data](02-accounting-and-data.md): authoritative arithmetic and invariants.
3. [Architecture and API](03-architecture-and-api.md): client boundaries, persistence, and contracts.
4. [Automation and AI](04-automation-and-ai.md): email, statements, voice, and grounded insights.
5. [Operations and cost](05-operations-and-cost.md): selected free-tier services, security, and recovery.
6. [Implementation plan](06-implementation-plan.md): ordered work packages and acceptance checks.
7. [Open questions and evidence](07-open-questions-and-evidence.md): unresolved decisions and research gates.
8. [Spreadsheet mapping](08-spreadsheet-mapping.md): inspected source structure, reconciliation findings, and migration gates.
9. [Hosting decision and comparison](09-hosting-recommendation.md): selected Render + Supabase services, verified limits, and deferred alternatives.
10. [Visual design and interaction](10-visual-design.md): confirmed appearance and monthly-review preferences, shared web/Android styling, and proposed implementation tokens.
11. [Platform ADR](../adr/0001-platform-and-delivery.md), [accounting ADR](../adr/0002-monthly-accounting.md), [funding-boundary ADR](../adr/0003-funding-boundary-and-reimbursements.md), and [accepted hosting ADR](../adr/0004-first-hosted-version.md).

## Rules for implementing agents

- Read the root [agent instructions](../../AGENTS.md) and [glossary](../../GLOSSARY.md).
- Implement one work package at a time; follow dependencies. Do not build AI or synchronization before financial invariants work.
- Preserve scope: monthly summaries, not mandatory merchant-by-merchant bookkeeping.
- Keep calculations in Go. Both clients display server summaries and use the same contract fixtures.
- Use synthetic data in code, tests, screenshots, and prompts. Never commit statements, tokens, or real account identifiers.
- Treat missing data as unknown, not zero. Do not market incomplete totals as an available bank balance.
- Default to local development and disabled paid providers. No paid fallback from a failed free/local service.
- Use the accepted Render + Supabase stack; verify provider terms before provisioning. Initial research was unavailable; the subsequent [hosting comparison](09-hosting-recommendation.md) verifies selected provider documentation as of 2026-10-26.
- The source spreadsheet has now been inspected; follow the [mapping](08-spreadsheet-mapping.md). Full parity still requires resolving ambiguous classifications and running a private migration comparison.
- The repository uses GitHub Issues for implementation specifications. These Markdown files are the requested planning handoff; when implementation starts, publish scoped work packages there following [tracker instructions](../agents/issue-tracker.md), linking back to these documents. This planning task did not create issues.

## Recommended first slice

Implement manual entry of one salary, personal spending funding and returned excess, two card statements and their payment states, a direct expense, and an investment allocation; display funding remainder and spending totals separately in a responsive React dashboard. Then add export and corrections. This proves the accounting before introducing document parsing or email permissions.

The final target includes a real Kotlin Android application. A responsive web app or installable PWA provides earlier mobile access but does not replace that deliverable.
