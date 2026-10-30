# Folio

A personal finance tracker for monthly income, credit card bills, major expenses, and savings or investment allocations. Designed for the browser and Android, with a Go backend.

The Go backend in [`services/api`](services/api/README.md) persists an owner's monthly sheets in PostgreSQL and computes their totals. The React client in [`apps/web`](apps/web) opens directly into one editable month: money in, money out, named items, section subtotals, and monthly remainder. It supports history suggestions, optional monthly recurrence and usual amounts, notes, correction with Undo, and JSON export. Go serves the compiled Vite bundle in the combined image. Native Android follows later; no hosting, bank access, email integration, or paid services have been provisioned.

The [monthly-sheet contract](api/monthly-sheet-contract.md) and [UX requirements](docs/planning/11-simple-monthly-ux.md) define the current workflow. Older financial endpoints and data remain intact for compatibility and export. They are not automatically converted or included in monthly-sheet totals. [ADR 0006](docs/adr/0006-monthly-sheet.md) records why the new model replaces the earlier account, statement, and payment workflows.

The first hosted version will use **Render Free** for the Go API and React frontend, and **Supabase Free** for PostgreSQL and authentication. Unattended processing will be finalized during Gmail integration. See the [accepted hosting decision](docs/adr/0004-first-hosted-version.md).

## Web development and combined build

For local development, run the Go API in explicit development-token mode and start Vite in a second terminal. The [API README](services/api/README.md) has the PostgreSQL setup and token commands. Then:

```sh
cd apps/web
npm ci
npm run dev
```

Use Node **22.22.2** for the pinned production build (the package also supports local Node 22.13+). In PowerShell, use `npm.cmd` and `npx.cmd`. Vite proxies API requests to `http://localhost:8080`; set `FOLIO_API_PROXY_TARGET` to override that target. Leave `WEB_DIST_DIR` unset so Go stays API-only during Vite development. Sign in with the configured owner bearer token; it stays in tab memory and is cleared on sign-out or expiry.

The sheet leads with monthly remainder, calculated from entered incoming and outgoing amounts. Card figures mean monthly bill amounts; paying a bill does not add another outgoing. Blank amounts remain distinct from zero, and recurring amounts are suggestions until applied. The remainder is not a bank balance. Writes retain their retry identity after uncertain outcomes.

From `apps/web`, run `npm run check:api`, `npm run typecheck`, `npm test`, and `npm run build`. Install Chromium with `npx playwright install chromium`, then run `npm run test:e2e`. The [browser suite](apps/web/e2e/README.md) serves the compiled app through an isolated test-only Go server with synthetic data. It requires no hosted account or production database.

The production Docker image builds the React bundle with Node **22.22.2**, builds the Go API, and serves both from port 8080. From the repository root, build it with:

```sh
docker build -f services/api/Dockerfile -t folio:local .
```

Start it with a production Supabase PostgreSQL URL and JWT settings (replace the placeholders with the values for the selected Supabase project):

```sh
docker run --rm --name folio -p 8080:8080 \
  -e APP_ENV=production \
  -e AUTH_MODE=jwt \
  -e OWNER_ID=11111111-1111-4111-8111-111111111111 \
  -e DATABASE_URL='postgres://folio:<database-password>@db.<project-ref>.supabase.co:5432/postgres?sslmode=verify-full' \
  -e JWT_ISSUER='https://<project-ref>.supabase.co/auth/v1' \
  -e JWT_AUDIENCE=authenticated \
  -e WEB_DIST_DIR=/app/web/dist \
  folio:local
```

This serves the built client and authenticated API from one origin. The browser's production Supabase session/PKCE login and token refresh remain P3 work; production API requests require a valid Supabase JWT bearer token. Local `AUTH_MODE=dev` token mode is for development only and is refused in production.

React's P2 implementation does not mean the hosted application is deployed. No hosting resources or paid services have been provisioned.

Start with the [implementation handoff](docs/planning/README.md). It links the product requirements, accounting rules, architecture, automation design, deployment options, and phased work plan.

The [visual design guide](docs/planning/10-visual-design.md) defines the shared web and Android appearance: a bright monthly overview, ocean-teal accents, selective glass, and restrained motion.

Shared terminology lives in [GLOSSARY.md](GLOSSARY.md); architectural decisions live in [docs/adr](docs/adr). Repository agent instructions remain in [AGENTS.md](AGENTS.md).
