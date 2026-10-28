# P2 browser implementation verification

Verified locally on October 28, 2026, with Node 22.13.0, Go 1.26.10 and Chromium
through Playwright. Scope: GitHub issue #4 and the P2 manual monthly review slice.
The compiled React application is served by the real Go HTTP stack; the opt-in
browser harness uses the real development/test authentication middleware and an
isolated memory store. All browser financial records and bearer values are
synthetic.

## Acceptance evidence

| Requirement | Evidence |
| --- | --- |
| Worked month through the browser | Accounts, income 5000, statement spending 1200 + 300, rent 1000 and allocation 800 produce Go spending 2500 and surplus 1700. |
| Settlement and revision | UI payment of 1200 leaves spending at 2500; UI statement revision from 1200 to 1100 produces spending 2400 and surplus 1800. Payment and obligation linkage remain distinct. |
| Review lifecycle | Checklist and explicit missing-statement reason permit close; posting controls lock; reopening records a reason and allows changes. |
| Unknown data | Missing statements and unknown cycle spending retain provisional/unknown figures rather than substituting zero. |
| Corrections and planning | Plan-matched income changes from 1000 to 900 through a reasoned correction, then zero through void; only the active replacement contributes. History remains visible. |
| Claims and drafts | UI claim/receipt matching reduces personal cost once; staged drafts remain outside totals, acceptance respects closed months, and downloaded JSON preserves records and draft history. |
| Uncertain writes | Dropping a real committed response and retrying preserves the exact UUID key, body and If-Match and produces one posting. Retained inline forms lock; uncertainty survives a later 401 and renewed sign-in. |
| Stale writes | A real concurrent account update produces 412; recovery controls remain inside the active modal and allow returning to review. |
| Sessions and failures | Real auth rejects invalid/expired bearers; sign-out clears private UI, tokens are absent from persistent browser storage, and failed/offline reads recover. |
| Responsive and keyboard interaction | Desktop 1440px, phone 390px and 320px reflow; labelled forms, modal keyboard containment, Escape and successful-save focus return. Wide tables retain horizontal scrolling without widening the phone viewport. |
| Compiled serving | Go CSP permits the bundled client and fonts; API/health routing remains intact. Go static tests cover fallback, missing assets, cache headers and traversal. |

The phone dialog regression came from an absolutely positioned, visually hidden
label escaping the wide table's scrolling container. Positioning that container
keeps the label inside its scrollable area. The original Save entry click timed
out before the fix; the complete plan/correction/void browser flow passes after it,
including an explicit page-overflow assertion.

## Commands and results

Run web commands from `apps/web` (use `npm`/`npx` on Unix):

```powershell
npm.cmd run check:api
npm.cmd run typecheck
npx.cmd tsc --noEmit -p e2e/tsconfig.json
npm.cmd test
npm.cmd run test:e2e
```

- Generated API type drift and both TypeScript checks pass.
- Client tests: **44 passed** across four files.
- Browser suite: **35 passed, 1 intentionally skipped** across 36 configurations.
  The skipped desktop case duplicates the phone-only 320px test.
- Browser startup runs `npm run build` and serves its output from Go, rather than
  testing against Vite. The production build passes.
- `npm.cmd audit --omit=dev`: zero reported production vulnerabilities.

From `services/api`, `go test ./...`, `go vet ./...` and `go build ./...` pass.
The browser harness is opt-in; ordinary Go tests do not start a long-running
server. Its authenticated reset/isolation test also passes.

From the repository root, Python contract validation passes for **52 operations,
74 schemas and both shared financial fixtures**. `git diff --check` passes.

## Environment limits and later milestones

Docker, PostgreSQL and a C compiler are unavailable locally. The combined Docker
image, PostgreSQL integration and race checks require CI. Browser tests exercise
the HTTP/domain integration with memory persistence; they do not substitute for
the PostgreSQL suite. Chromium device emulation does not establish Safari or
physical-device compatibility. No hosting was provisioned.

The client implements explicit manual bearer sign-in for P2. Hosted Supabase
PKCE/refresh and restoration are P3 work. Pending retry identities and committed
settlement IDs survive failures in memory while the tab stays open; they are not
durable across reload. Reconcile records before recreating an interrupted action
after reload. Recurrence editing, reconciliation-period authoring, Android,
imports, Gmail and model capture remain later milestones. Structured draft
creation currently uses an advanced JSON form.
