# Folio web

One monthly sheet for incoming and outgoing amounts. React, TypeScript and Vite
render the sheet; Go computes totals, recurrence, history suggestions and Undo.
The API contract is [monthly-sheet-contract.md](../../api/monthly-sheet-contract.md).

## Local development

Use a Node version supported by package.json. From apps/web on PowerShell:

```powershell
npm.cmd ci
npm.cmd run dev
```

Start the Go API using [its instructions](../../services/api/README.md). Vite
proxies /api/v1 to http://localhost:8080. FOLIO_API_PROXY_TARGET overrides that
server-side development target. The production build is served by Go.

Sign in with the configured owner bearer token. Tokens stay in tab memory and
are sent only to the same-origin API, with cookies omitted and redirects rejected.
No credential belongs in a VITE variable, URL, source file or browser storage.
Hosted sign-in and token refresh remain separate work.

## Monthly entry

Choose a month, then add an item inside its section with Name and Amount.
Credit cards use Bill amount. Saved history suggests names without copying prior
amounts. Repeat monthly is optional and off for new names. Fixed recurring amounts
can have a usual suggestion; applying suggestions fills only blank rows.

Edit a row's amount directly. Blur or Enter saves; Tab and Enter move between
amounts. Blank differs from an explicit zero. Item names open notes, rename,
recurrence and usual-amount details. Skip this month retains history and recurrence;
Stop repeating ends future automatic rows. Clear and skip offer Undo.

Sheet options add or rename sections. Settings contains currency, export and
sign-out. Old financial data stays separate and remains in the lossless export.
The monthly remainder is entered money in minus entered money out, not a bank
balance or a measure of consumption.

## Saving and recovery

The client serializes monthly writes because one version covers all monthly
sheets. Each submitted write retains an immutable body, retry key and precondition.
Saving, saved and failed states appear beside the affected row or form. An
uncertain write blocks subsequent writes until Retry same action recovers it.
A stale precondition offers Compare latest, retaining the typed edit for an
explicit fresh save. The app never silently replaces a stale command's version.

Edits and retry identities survive session interruption in this tab. Sign in with
the same owner token to continue. Month changes and sign-out wait for unsaved
edits to be saved or discarded. Closing or reloading the tab loses in-memory edits
and retry identities; the browser warns while work is unsaved. This is not a
durable offline write queue. Reconcile saved rows before resubmitting after reload.

## Validation

```powershell
npm.cmd run check:api
npm.cmd run typecheck
npm.cmd test
npm.cmd run build
npm.cmd exec tsc -- --project e2e/tsconfig.json
npm.cmd exec playwright -- install chromium
npm.cmd run test:e2e
```

The backend owns generated src/lib/schema.d.ts. Unit tests cover exact money,
API command identity, authentication races and retained form state. Browser tests
run the built app against the real Go test server, including lost committed
responses, stale versions, history, recurrence, export, keyboard focus and 320px
reflow. See [e2e/README.md](e2e/README.md). Do not run concurrent browser suites
against its one resettable store.

The first version uses a light theme, locally bundled Inter, solid financial
surfaces, visible focus and 44px controls. Hosted auth, import/migration, restore,
Android changes and durable offline writes are outside this frontend change.
