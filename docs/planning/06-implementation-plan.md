# Implementation plan and acceptance gates

Work packages are ordered for subsequent agents. Do not estimate delivery dates until the first slice establishes learning pace. Each handoff should list changed files, commands/checks run, evidence, open questions, and next unblocked package. No application code is delivered by this planning task.

## P0 — Confirm examples and bootstrap contracts

Dependencies: none. Output: synthetic fixtures, OpenAPI skeleton, local setup, decision updates.

- Adopt Singapore, SGD, and Asia/Singapore defaults; Gmail is the confirmed mail provider. The workbook names Citi Cash Back+ and Maybank XL Cashback; active-card status and statement templates remain to be confirmed.
- Use the inspected [spreadsheet mapping](08-spreadsheet-mapping.md). Resolve flagged formula exclusions and ambiguous funding/claim semantics; prepare a private import preview and synthetic regression fixtures. Do not block the synthetic core slice on unresolved historical rows.
- Pin supported Go, Node, PostgreSQL, Kotlin/Gradle, and Android SDK versions; document reproducible setup. Create local DB and explicit dev-only identity.
- Turn the financial examples into shared test fixtures and finalize endpoint schemas. Create future GitHub implementation tickets according to repository instructions; copy acceptance criteria and dependencies from this plan.

Gate: a new agent can start local dependencies, validate the API contract, and explain the baseline financial fixture. Initial spreadsheet mapping is available; full parity remains blocked on semantic decisions and private reconciliation. Bank-parser work remains blocked on statement samples.

## P1 — Financial core in Go

Dependencies: P0 contract/fixtures; spreadsheet not required for initial implementation.

Implement migrations, ownership constraints, account/settings CRUD, claims/receipts and reimbursement links, entries, statement revisions, settlements, plan lines, snapshots, month summaries, corrections, and audit history. Use transactions, source uniqueness, idempotency records, and optimistic concurrency. Keep database and HTTP concerns outside pure financial calculation functions.

Gate: table-driven Go tests plus PostgreSQL integration tests prove all accounting scenarios below; malformed currency and cross-owner references are rejected; closed-month writes fail; duplicate concurrent accepts cannot create two postings.

## P2 — Browser monthly review

Dependencies: P1. Build React dashboard, quick entry, card detail, expected-versus-actual view, coverage checklist, correction flow, and month close/reopen. Add accessible responsive forms and clear loading/error/empty states. Display actual and projected figures separately.

Gate: Playwright flow records the worked month and gets surplus 1,700, settles a card without changing spending, corrects a statement, and sees revised totals. Works at desktop and phone widths. Missing statement makes summary provisional. Repeated submit and timeout retry do not duplicate records.

## P3 — Safe personal use and portability

Dependencies: P2. Integrate production owner auth, export/full restore, session expiry handling, redacted logs, backup procedure, and local production configuration. Implement the accepted Render Free + Supabase Free deployment configuration from [ADR 0004](../adr/0004-first-hosted-version.md), targeting Singapore. Recheck account-specific quotas and record deployment settings; hosting selection is already resolved. Do not select unattended-processing infrastructure in P3. No paid service is required to complete local gates.

Gate: unauthenticated and different-owner API requests cannot read/write/download data; development auth cannot start in production; export/restore reproduces totals and source links; encrypted backup restore succeeds. If deploying, verify quotas and test Render cold-start/reconnect behavior, Supabase pause/recovery procedures, and pooled TLS database connectivity. This is the first usable manual browser release.

## P4 — Native Android

Dependencies: P3 and stable API contract. Implement Kotlin/Compose dashboard, auth via system browser/PKCE, manual entries, statements/payment views, review inbox shell, correction flow, and secure logout. Use generated/shared contract fixtures and reuse the backend financial logic.

Gate: same fixture has identical summary on both clients; edit on web appears after Android refresh; timeout retry is idempotent; expired sessions and disconnected network have recoverable states. Signed APK installs and upgrades without data loss. Native Android is required for the overall requested outcome, even if email automation ships independently afterward.

## P5 — Imports and reconciliation

Dependencies: P3; P4 can consume completed endpoints afterward. Add the source workbook block/subtotal mapping, private legacy-total comparison, joint-period reconciliation, CSV mapping/preview, manual statement upload, durable jobs, review drafts, evidence, and duplicate/revision matching. Add one bank parser only after redacted samples and currency/format conventions are known.

Gate: reimported CSV/PDF/email-derived fixture produces one statement; changed statement amount prompts revision review; corrupt/encrypted/unsupported documents do not corrupt records; unknown new spending cannot become a false complete summary. Confirmed imports update both clients.

## P6 — Gmail collection

Dependencies: P5 and verified Google OAuth feasibility. Implement explicit connection/disconnection, bounded queries, read-only access, checkpoints, on-demand sync, token refresh, and reviewable drafts. Finalize the deferred unattended-processing choice in this phase and record its decision, cost/eligibility checks, and failure behavior. Evaluate scheduler, worker execution, and database availability together; no scheduler is currently selected. Add scheduling only after the chosen mechanism passes these checks.

Gate: duplicate delivery, pagination, expired history cursor, revoked token, quota throttling, process restart, and changed template are covered. Statement-ready emails without amounts remain incomplete. No send/delete scope is requested. If OAuth or hosting constraints prevent unattended operation, document the blocker and ship on-demand/manual fallback without claiming automation is complete.

## P7 — Insights and optional voice/models

Dependencies: P2 for deterministic insights, P5 for extraction integration, P4 for Android voice.

Start with rule-based insights and typed natural-language drafts. Add on-device speech where supported, optional local model adapters, and strict schema/evidence checks. Evaluate before selecting a model. Paid hosted adapters are deferred and disabled.

Gate: zero-API-key development/CI works; no insight invents merchant categories; no suggested allocation exceeds configured affordable surplus; unknown amounts request correction; “will receive” does not post received salary. Audio and malicious prompt fixtures cannot trigger financial writes without the draft acceptance boundary.

## P8 — Monthly workflow validation

Dependencies: P4, P5; P6/P7 capabilities included only if their gates pass.

Run a complete synthetic month, then user-led shadow tracking against the supplied spreadsheet for a real cycle. Record discrepancies and causes; resolve every financial mismatch before treating the app as the preferred record. Measure review time and number of corrections. Complete restore and cold-start checks and document remaining manual steps.

Gate: web and Android agree, tracked categories match the mapped spreadsheet convention, retries remain harmless, incomplete data is visible, and the user can explain each total. Do not declare spreadsheet equivalence without the file and a completed comparison.

## Required financial scenarios

| Scenario | Expected invariant |
| --- | --- |
| Salary 5,000; statements 1,200 + 300; rent 1,000; allocation 800 | Spending 2,500; surplus 1,700 |
| Pay statement 1,200; retry same request | One payment; surplus unchanged |
| Pay 400 of 1,200 after cutoff | Remaining payable 800; spending unchanged |
| Payment already included before statement cutoff | Not deducted from printed amount due again |
| Next cycle carries 300 and adds 200 | New spending 200; one active payable 500 |
| Refund 100 | Spending reduced once; cash/obligation effect separately linked |
| Revised statement 1,200 → 1,100 | One active spending amount; retain revision history |
| Duplicate email/PDF and simultaneous confirmation | One posting, deterministic acceptance response |
| Two legitimate equal payments | Review candidate, not automatically dropped |
| Planned salary 5,000 matched to actual 5,000 | Forecast income remains 5,000 |
| Planned allocation 800, completed 300, remaining 500 | Forecast allocation 800 |
| Internal transfer or market gain | No new income or consumption |
| Salary absent or income zero | Missing/zero distinguished; savings rate unavailable for zero income |
| Missing card spending but known bill due | Provisional summary; payable still visible |
| End of month near UTC boundary | Reporting month follows Asia/Singapore local event date |
| Closed month receives an import | Draft only until reopen |
| Web and Android edit same revision | Stale update rejected; no lost update |
| Unknown or foreign currency | Review/reject, no silent SGD summation |

## Test strategy

Use pure Go table tests for arithmetic and property tests for retry/transfer invariants; real PostgreSQL integration tests for transactional acceptance, constraints, and concurrent edits; contract tests for generated clients; focused Playwright and Compose flows for financial correctness and key failure paths. Parser tests use sanitized golden fixtures with malformed and changed templates. Security checks cover auth, ownership, CSRF, file limits, and secret/log redaction. Run appropriate checks per package rather than repeatedly running every platform suite for documentation changes.

## Required additions after workbook inspection

- P0: adopt ADR 0003; personal spending funding and returned excess are user-confirmed transfers. Joint attribution, historic claim receipt status, and card figure semantics remain unresolved. Add synthetic fixtures without private values.
- P1: implement effective-dated funding boundaries, funding summaries/projections, claim/receipt/recovery matching, and recurrence versioning alongside the original financial core.
- P2/P4: make monthly funding remainder the primary check-in on both clients; show spending surplus and observed cash separately. Add pending/received claims and clear gross-versus-net spending presentation.
- P5: preserve legacy totals as private comparison data; do not import subtotal rows and components twice. Include single-owner joint-period reconciliation and subscription checklists without inventing payment data.
- Required tests: synthetic funding fixture returns 2,250; envelope return does not increase salary; pending claim does not increase actual cash; matched reimbursement reduces personal cost once; duplicate receipt matching fails atomically; internal funding-boundary transfer nets zero; boundary version changes do not silently restate history; incomplete October-style data stays provisional; out-of-range refund/claim formulas are flagged during migration.
