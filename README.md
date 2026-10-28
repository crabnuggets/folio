# Folio

A personal finance tracker for monthly income, credit card bills, major expenses, and savings or investment allocations. Designed for the browser and Android, with a Go backend.

The Go backend is implemented in [`services/api`](services/api/README.md), with shared [API contracts](api/README.md) and synthetic [fixtures](fixtures). It covers the manual financial core, PostgreSQL persistence, owner authentication and review drafts. React and native Android clients are next; no hosting, bank access, email integration, or paid services have been provisioned.

The first hosted version will use **Render Free** for the Go API and React frontend, and **Supabase Free** for PostgreSQL and authentication. Unattended processing will be finalized during Gmail integration. See the [accepted hosting decision](docs/adr/0004-first-hosted-version.md).

Start with the [implementation handoff](docs/planning/README.md). It links the product requirements, accounting rules, architecture, automation design, deployment options, and phased work plan.

Shared terminology lives in [GLOSSARY.md](GLOSSARY.md); architectural decisions live in [docs/adr](docs/adr). Repository agent instructions remain in [AGENTS.md](AGENTS.md).
