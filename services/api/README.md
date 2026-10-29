# Go backend

The manual financial core serves the same REST/JSON API to React and Kotlin clients.
The [shared contract](../../api/README.md) and [OpenAPI](../../api/openapi.yaml)
cover implemented and explicitly deferred operations. [ADR 0005](../../docs/adr/0005-backend-owner-aggregate.md)
explains the PostgreSQL owner aggregate and its scaling tradeoff.

## Local setup

Requirements: Go **1.26.10**, Docker with Compose, Node **22.22.2** for the web
client, and Python **3.12+** for contract validation. PostgreSQL is pinned to
**17.6**. No hosted account or API key is needed.
From the repository root:

```sh
docker compose -f infra/compose.yaml up -d --wait
cd services/api
export APP_ENV=development AUTH_MODE=dev
export OWNER_ID=11111111-1111-4111-8111-111111111111
export DEV_AUTH_TOKEN="$(openssl rand -hex 32)"
export DATABASE_URL='postgres://folio:folio-local-only@localhost:5432/folio?sslmode=disable'
go run ./cmd/folio
```

Keep the dev token private and use that shell value from another terminal or your
API client. `.env.example` documents configuration; `.env` is not loaded
automatically. PostgreSQL is required; the running server never selects an
in-memory fallback. Migrations apply under a startup transaction lock. The local
Compose volume is durable; `docker compose down -v` deletes local data.

### Local web development

The React client supports the P2 manual monthly review. Keep the Go API running
in one terminal and run Vite from another terminal at the repository root:

```sh
cd apps/web
npm ci
npm run dev
```

Vite proxies `/api` requests to `http://localhost:8080`; override the target with
`FOLIO_API_PROXY_TARGET`. Use `npm.cmd` and `npx.cmd` in PowerShell. Sign in with
the configured owner bearer token; the client keeps it only in tab memory. Leave `WEB_DIST_DIR`
unset in this workflow: Go then serves only API and health routes, and Vite serves
the development client. The explicit `AUTH_MODE=dev` token from the API setup is
accepted for local API calls; production refuses this mode.

### Built frontend and static hosting

Set `WEB_DIST_DIR` to a built Vite `dist` directory to enable Go's optional SPA
hosting. The directory must contain `index.html`. `GET` and `HEAD` requests for
existing files are served from that directory; extensionless browser navigations
accepting `text/html` fall back to the app entry point. Missing assets, API paths,
and health paths return JSON errors or their normal JSON health responses. The
default when `WEB_DIST_DIR` is unset remains API-only.

The combined production image builds the frontend with Node **22.22.2** and the
Go API, then serves the bundle from `/app/web/dist` as the non-root Go process.
From the repository root:

```sh
docker build -f services/api/Dockerfile -t folio:local .
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

Replace the database password and project reference before starting the
production image. Production browser Supabase session/PKCE login and refresh are
P3; the API currently verifies JWT bearer tokens, while `AUTH_MODE=dev` remains
local-development-only.

```sh
curl -H "Authorization: Bearer $DEV_AUTH_TOKEN" http://localhost:8080/api/v1/settings
curl -X POST http://localhost:8080/api/v1/accounts \
  -H "Authorization: Bearer $DEV_AUTH_TOKEN" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"alias":"Main","kind":"bank","currency":"SGD","active":true,"include_in_liquid_cash":true,"funding_boundary":[{"effective_month":"2026-01","included":true}]}'
```

Use UUID retry keys once per user action; preserve the key and exact body on
network retries. Fetch the resource's ETag before versioned writes and send it in
`If-Match`. Entries carry explicit source/destination account references. Both
clients display Go summaries; neither recomputes financial totals.

## Verification

Create a dedicated test database once from the repository root:

```sh
docker compose -f infra/compose.yaml exec postgres createdb -U folio folio_test
```

Then, from `services/api`:

```sh
export TEST_DATABASE_URL='postgres://folio:folio-local-only@localhost:5432/folio_test?sslmode=disable'
go test -race -count=1 ./...
go vet ./...
go build -o /tmp/folio ./cmd/folio
```

Without `TEST_DATABASE_URL`, real database tests explicitly skip. CI supplies
PostgreSQL and runs them. Database tests commit synthetic owner UUIDs and retry
records without deleting them; always use a dedicated test database such as
`folio_test`, separate from the application's `folio` database. They test parallel
idempotency, rollback, durability, owner isolation and RLS. HTTP contract tests
validate real responses against OpenAPI and consume shared fixtures.

From the repository root, in a Python virtual environment:

```sh
python -m pip install -r api/requirements-contract.txt
python scripts/validate_contract.py
```

## Runtime boundaries

`APP_ENV=production` refuses development auth and requires PostgreSQL
`sslmode=verify-full`. Configure `AUTH_MODE=jwt`, `OWNER_ID`, `JWT_ISSUER` as the
Supabase Auth issuer, and `JWT_AUDIENCE=authenticated`. The server verifies
RS256/ES256 signatures with issuer-pinned HTTPS JWKS, audience, issuer, expiration,
and the configured owner. Supabase projects using legacy HS256 keys must move to
asymmetric signing keys before using this verifier. Disable public signup.

The startup DB role needs DDL privileges for migrations. The private `folio`
schema revokes PUBLIC and Supabase Data API role access and enables forced RLS;
the backend still checks owner identity itself. A privileged database role is not
made safe by RLS alone. The container runs as a non-root user. `/healthz` checks
process health and `/readyz` checks DB connectivity. SIGTERM drains requests.

This slice implements bearer authentication, manual structured draft review,
rule-based insights and JSON export. The React client covers the P2 manual monthly
review and displays server-derived financial summaries. Production browser
session/PKCE UX and token refresh remain P3. Android, restore/CSV, bank parsers,
Gmail, and natural-language capture remain subsequent planned work. Their
integration routes return a structured 501 until implemented. No hosting or paid
provider has been provisioned.
