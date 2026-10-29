# Monthly-sheet implementation review

The accepted scope is the [simple monthly UX](../planning/11-simple-monthly-ux.md), [monthly-sheet contract](../../api/monthly-sheet-contract.md), and [ADR 0006](../adr/0006-monthly-sheet.md). Implementation snapshot `14c151e` is compared with base `54be303` on the PR branch.

## Initial validation

The implementation delegates reported passing full Go tests and vet, OpenAPI validation, generated TypeScript consistency, client typecheck, 45 client unit tests, production build, browser-test TypeScript, and 26 real-Go desktop/phone Playwright tests. The root inspected the desktop/phone screenshots, checked the clean snapshot, and verified that legacy endpoint contracts were unchanged apart from the additive export state.

Real PostgreSQL restart/retry/Undo coverage is present but skipped locally because `TEST_DATABASE_URL` is unset and Docker commands became unresponsive. No claim of live PostgreSQL validation is based on the memory adapter. Bearer authentication, in-memory unsaved edits/retry identities, and retained legacy data without automatic migration remain explicit scope limits.

## Standards review

A separate GPT-6.1-Sol delegate at high reasoning effort found no documented-standard breaches or actionable heuristic smells. The shared-version write queue was judged necessary, and no generic queue abstraction or legacy cleanup was requested.

## Specification review

An independent GPT-6.1-Sol delegate at high reasoning effort reported two P2 correctness findings. The root accepted both after inspecting the state and idempotency paths.

| Finding | Required correction |
| --- | --- |
| Comparing a concurrently removed or relocated row can unmount its draft and orphan the dirty state. | Retain the draft through comparison, allow explicit discard or reapply, and release the relevant dirty state safely. |
| An uncertain save followed by a definitive 412 remains permanently uncertain. | Resolve uncertainty on an authoritative stale response after idempotency lookup, retaining command identity until that response and preserving drafts. Authentication failures must not clear uncertainty. |

## UX review

A dedicated GPT-6.1-Sol delegate at high reasoning effort inspected the real UI, tests, and both screenshots. It reported three P2 behavioral findings, all accepted by the root.

| Finding | Required correction |
| --- | --- |
| Clicking Discard can trigger blur autosave before cancellation. | Pointer and keyboard discard must restore the saved value without a mutation; ordinary blur still saves. |
| Tab traverses intervening controls instead of the promised amount fields. | Support forward/backward amount traversal with normal boundary exits and keyboard access to item details. |
| First-use currency guidance is hidden before currency becomes locked. | Show a compact pre-lock currency/change affordance on the sheet, without a wizard or a false promise on a later empty month. |

The screenshot review otherwise found readable desktop/phone layouts, contextual card entry, and quiet settings. Account setup, payment screens, scheduling administration, and review ceremonies remain outside scope. The root found no need to reject a reported finding or add a broader feature.

## Fix-only review limit

After the focused fix pass, a fresh delegate will review only changes after `14c151e`, using these accepted findings as its brief. At most three fix-only review iterations are authorized. Findings must identify a regression, an incomplete accepted fix, or scope creep in that delta; they must not reopen the whole implementation or add unrequested features.

## Focused fix pass

The implementation delegate added regression cases that reproduced each accepted failure before applying fixes. The root inspected the resulting diff. Dirty rows remain mounted through comparison, with explicit local discard/reapply; reapply merges only edited fields so concurrent notes are preserved. An authoritative 412 resolves uncertainty, while authentication failures retain it. Discard suppresses its blur save, amount fields support Tab/Shift+Tab, and Escape provides a route to discard or item details. Currency guidance uses a new server-supplied `currency_locked` field backed by the existing Settings predicate, so an empty later month is not mistaken for first use.

Final fix-pass checks passed: full Go tests and vet, OpenAPI generation and validation, generated client types, client typecheck and 45 unit tests, production build, browser-test TypeScript, whitespace checks, and 54 real-Go Playwright tests across desktop and phone. The browser's `.last-run.json` also reports passed. Four PostgreSQL tests remain explicitly skipped locally.

## Fix-only iteration 1

The reviewer inspected `14c151e..172b095` and found one P2 incomplete keyboard fix: Shift+Tab from a dirty first amount focused its item-name button, then blur autosave disabled that button and dropped focus to the body. The earlier boundary test covered only a clean value. The root accepted this finding and reproduced it with a real-Go browser test that holds the save in flight.

The root removed the item-name button's disabled state. Amount and details editing remain locked by the existing save state. The new dirty-boundary regression and existing forward/backward/boundary/details traversal case pass on desktop and phone, four cases total. Browser-test TypeScript and the production build also pass. No broader keyboard controls or product features were added.

Fix-only iteration 2 reviews only that follow-up delta. One further iteration remains available if necessary.
