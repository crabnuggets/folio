# Browser acceptance

From `apps/web`, use Node 22 and Go 1.26.10:

```sh
npm ci
npx playwright install chromium
npm run test:e2e
```

In PowerShell, use `npm.cmd` and `npx.cmd`. CI installs Chromium's Linux system
dependencies with `npx playwright install --with-deps chromium`.

Playwright builds the React app once, then starts a test-only Go server on
`127.0.0.1:18080`. It serves the compiled app using `NewWithWebDistDir`, including
the production CSP, assets, SPA fallback and same-origin API. No Vite server,
Docker, PostgreSQL, hosted account or real credential is required for this suite.
The existing backend workflow remains responsible for PostgreSQL integration.

`start-server.mjs` explicitly sets `FOLIO_BROWSER_E2E=1` and the absolute
`FOLIO_BROWSER_WEB_DIST_DIR`. The server is implemented solely in a Go `_test.go`
file, uses real `auth.New` middleware and `httpapi.New`/`NewWithWebDistDir`, and
creates the memory adapter only inside the test binary. Ordinary `go test ./...`
skips the long-running server. Invalid configured build directories fail startup.

Each browser test resets state using an authenticated, test-only
`POST /__e2e/reset`, outside the production API. One worker prevents test data
from crossing resets. Every alias, financial amount and bearer is synthetic.

Coverage is split across complementary flows:

- `worked-month.spec.ts`: browser account setup, income 5000, expense 1000,
  allocation 800, statements 1200+300 and checklist close; verifies Go spending
  2500/surplus 1700. The browser reopens, records and links payment 1200 without
  changing spending, revises statement spending to 1100, and closes with spending
  2400/surplus 1800. Saves synthetic desktop/phone overview screenshots.
- `financial.spec.ts`: targeted display assertions with API-created state for
  settlements, idempotent replay, statement revision, closed-month rejection,
  missing statements and unknown spending.
- `review-actions.spec.ts`: checklist close with no-statement reasons, audited
  reopen, real stale-version conflict and recovery, same-month funding boundary
  edits, claims and receipt matching, draft staging while closed and acceptance
  after reopen, downloaded export contents, plan matching, entry correction/void,
  and focus return after successful dialog submission.
- `retry.spec.ts`: repeated submit, deliberately lost committed response, identical
  UUID/body/precondition and one posting; retained inline draft form locking;
  authentication expiry and sticky uncertainty across a rejected retry/sign-in.
- `session.spec.ts`: production CSP boot, SPA/API/health routing, bearer rejection,
  logout, absent token persistence, offline/read-error recovery, keyboard modal
  navigation/focus return, and 320px reflow.

The standard projects exercise desktop 1440px and phone 390px. The extra 320px
test runs once in the phone project. Routing interception is confined to deliberate
network faults, expired authorization, and forwarding real committed responses;
financial success responses come from Go.

Type-check the acceptance code separately (the app tsconfig excludes `e2e`):

```sh
npx tsc --noEmit -p e2e/tsconfig.json
npx playwright test --list
```

Failure traces/screenshots are local ignored artifacts and CI uploads them for
seven days. Run only against synthetic data. Passing browser tests establish the
listed flows, not every possible account or financial combination. Go contract,
fixture and PostgreSQL tests cover the underlying invariants separately. Hosted
PKCE/refresh, restore, recurrence editing and reconciliation-period authoring
remain outside this P2 browser slice.
