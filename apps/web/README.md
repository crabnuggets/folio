# Folio web

The P2 manual monthly financial review client. React + TypeScript + Vite, TanStack
Query for server state, and React Hook Form for accessible labelled forms. The Go
API owns every financial calculation. Runtime data always comes from the API;
synthetic records exist only in tests.

## Local development

Use Node **22.13.0** (or a supported newer version satisfying `package.json`) and
npm. Packages are pinned and `package-lock.json` is checked in. Commands below run
from `apps/web`. On Windows PowerShell use **`npm.cmd` / `npx.cmd`** to avoid blocked
PowerShell script shims. On Unix, use `npm` / `npx`.

```powershell
npm.cmd ci
npm.cmd run dev
```

Start the Go API separately using [its local instructions](../../services/api/README.md).
Vite proxies `/api/v1` to **http://localhost:8080**. Override the server-only proxy
target when necessary:

```powershell
$env:FOLIO_API_PROXY_TARGET = 'http://127.0.0.1:18080'
npm.cmd run dev
```

Open the Vite URL and sign in with the access token configured for that API's
owner. The manual bearer sign-in supports the explicit local development token or
a valid owner JWT. Do not put a bearer token in a `VITE_*` variable, source file,
URL, localStorage, sessionStorage, or a committed environment file. The application
stores it only in tab memory, sends it only to same-origin `/api/v1`, omits cookies,
and rejects redirects. It clears private query data on sign-out/401; JWT expiration
also clears the session using the untrusted `exp` only as a UI timeout hint. Go
remains responsible for identity and token verification. Hosted PKCE/refresh-token
sign-in is P3 work.

The selected review month remains explicit and is retained during the active tab
session. Navigation uses URL fragments, so browser Back/Forward works between
destinations without placing financial values or credentials in the URL.

## Review workflow

1. In **Accounts & settings**, choose currency/timezone before financial data,
   create account aliases and designate the main funding boundary with an effective
   month. A separate personal spending envelope remains outside that boundary.
2. In **Plan & review**, add recognized and funding expectations independently.
   Match actual entries/statements to stable plan lines; expectations never post.
3. **Add entry** records confirmed income, expenses, allocations, transfers,
   envelope funding/returns, refunds, reimbursement receipts or settlement cash.
   Decimal input accepts a decimal point without thousands separators; all
   conversion uses exact integer strings/BigInt, with signed-64-bit bounds.
4. **Statements** keeps new spending, amount due, remaining payable, excess credit,
   evidence and revisions distinct. Choose unknown spending, evidenced net
   spending, or all four components. No value is inferred from the printed due.
   **Record payment** records and links confirmed cash, or links an existing payment.
   This records payments already made; Folio does not initiate bank payments.
5. Review funding remainder first, then recognized spending/surplus, forecasts,
   coverage gaps and observed dated balances. Unknown remains unknown, including
   statement spending, surplus and unavailable forecasts. Snapshot rows do not
   imply a complete cash balance or reconstruct cash from partial entries.
6. Complete the checklist and any explicit no-statement reasons to **Close month**.
   A reason is required to reopen. Closing is a review assertion; unknown costs
   remain unknown. Entry corrections/voids and statement revisions retain history.
7. Track **Claims** separately from cash. Record a reimbursement receipt and match
   it to a claim, optionally classifying an open recovery month against a linked
   expense or statement. Pending claims are not salary. **Drafts** supports manual
   structured JSON candidates and explicit evidence/accept/reject review.
8. **Export JSON** downloads the versioned lossless financial export, including
   histories. It is sensitive; store it securely. Restore is a later milestone.

## Writes, retries and concurrency

Every action captures one immutable body, UUID idempotency key and (when required)
`If-Match` version. Repeated submits are single-flight. Neither TanStack Query nor
the API client automatically retries financial mutations. After a timeout,
disconnect, invalid success body or server failure, use **Retry same action**.
The exact original key/body/precondition is retained; the form is locked until the
outcome is recovered. An unfinished step's uncertain outcome stays uncertain across later rejected
retries and session expiry; sign in again and recover the original action. Recovery controls are inside the active native dialog, so
they remain keyboard-accessible while the rest of the page is inert.

Settlement retains each successful payment ID before linking. A failed or stale
link never starts another payment on retry. Once the payment result is recovered,
a definitive link rejection permits returning to review and linking that existing
payment against the current statement. A lost link response still requires recovery. If a confirmed conflict requires
returning to review, the payment remains in Records; reopen **Record payment** and
choose **Link an existing recorded payment**. Do not enter another payment.

`412` asks the user to refresh and compare the latest record before a revised
submission; the client never silently replaces a stale precondition. Validation
failures offer **Edit form**, retaining values. Submitted values remain inspectable
in the recovery panel. A closed affected month must be explicitly reopened.

Pending actions are retained **in memory only**, including across session expiry
and a renewed same-owner sign-in in this tab. Keep the page open during recovery.
Reloading/closing loses the key; the browser warns while an action is pending.
After a reload, inspect existing records/export and reconcile before submitting
again. This is online-first, not a durable offline financial-write queue.

## Build and hosting boundary

```powershell
npm.cmd run generate:api   # regenerate src/lib/schema.d.ts from api/openapi.yaml
npm.cmd run check:api      # detect contract/type drift
npm.cmd run typecheck
npm.cmd test
npm.cmd run build
```

`npm.cmd run build` emits **`apps/web/dist`**. The Go static-hosting integration
serves this directory at the same origin as `/api/v1`; see the API README for its
distribution-directory configuration. The bundle has no environment-provided
credential or external font request. Fonts are locally bundled Inter (OFL licence
shipped with `@fontsource/inter`). Vite's proxy is for development only. Static
hosting must preserve API/health routes, asset MIME types, SPA fallback and a CSP
compatible with same-origin script/style/font assets.

`npm.cmd run preview` previews compiled assets only; it is not the API server.
Do not use it as a production deployment.

## Checks and known boundaries

Focused tests in `tests/` cover exact money (including values beyond Number's safe
integer range), pagination, auth/cache/redirect isolation, logout races, version
conflicts, single-flight writes, lost responses/timeouts, immutable retry identity,
two-step settlement recovery and modal recovery ancestry. They use synthetic
fixtures and fake transports, not a fake production dataset. jsdom 26.1.0 is pinned
to support the repository's Node 22.13.0 runtime.

The separately maintained `e2e/`, `playwright.config.ts`, Go browser test server and
web workflow exercise the compiled application against real Go calculations:

```powershell
npx.cmd playwright install chromium
npm.cmd run test:e2e
```

Do not run concurrent browser suites against the shared resettable test store.
See E2E configuration for its loopback ports and synthetic auth. Docker is not
required for the test-only memory-store harness. Production still uses PostgreSQL.

The UI includes keyboard focus styles, native modal focus containment/restoration,
persistent labels, numeric table headers, 44px targets, mobile navigation/reflow,
reduced-motion and opaque glass fallbacks. Browser integration, screen-reader and
visual acceptance require their own checks; unit tests do not establish them.
The initial version is light only. Recurrence template editing, reconciliation
period authoring, automatic parsing/imports, Gmail, text/voice capture, hosted
session refresh and restore remain outside this client slice. Structured draft
creation is an advanced JSON form, not a document importer or inference feature.
