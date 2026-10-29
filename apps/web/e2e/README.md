# Browser acceptance

From apps/web, install dependencies and Chromium, then run the suite:

```powershell
npm.cmd ci
npm.cmd exec playwright -- install chromium
npm.cmd run test:e2e
```

Playwright builds React and starts the existing Go test-only server on
127.0.0.1:18080 using the production static hosting and authentication middleware.
No separate Vite server, hosted account, database or real credential is needed.
The memory adapter is confined to the test binary. Production uses PostgreSQL.
Each test resets the isolated store through the authenticated /__e2e/reset route.
One worker keeps resets isolated. All names, amounts and tokens are synthetic.

Coverage runs on desktop and touch phone projects:

- worked-month.spec.ts enters a complete month and checks Go totals and export,
  recurring blank rows, usual amounts, explicit zero, skip, Undo and stop.
- financial.spec.ts reuses saved history with keyboard or touch, retains typed
  amounts and recurrence preferences, focuses existing rows without duplicates,
  and permits dismissed suggestions and free typing.
- retry.spec.ts loses a real committed response, rejects a retry through real
  auth middleware, renews the session and checks exact command replay. It also
  verifies stale comparison and serial versions for queued row edits.
- review-actions.spec.ts checks clear/Undo, skip/Undo, notes, rename and custom
  sections, including the stable Bill amount label after a section rename.
- session.spec.ts checks CSP, SPA/API/health routing, sign-in/sign-out, storage
  isolation, offline/read recovery, 320px reflow and keyboard entry/focus.

Financial success responses are never fabricated. Failure interception either
aborts transport or forwards to the actual Go server, including replay recovery.
The suite validates browser behavior; it does not replace PostgreSQL integration,
screen-reader testing or independent UX review.
