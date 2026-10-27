# Folio API v1

This contract is shared by React and native Android. The Go module lives in
`services/api`; clients belong in `apps/web` and `apps/android`. OpenAPI is the
machine-readable contract; `internal/domain/model.go` defines the implementation
DTOs. Financial arithmetic lives exclusively in Go.

## Wire conventions

Base `/api/v1`. Bearer access tokens authenticate the configured single owner.
The initial browser client must send bearer tokens; backend cookie sessions/PKCE
are a later authentication milestone, not an implicit cookie fallback. Local auth
requires explicit development mode and an explicit dev bearer token. No identity
is taken from request bodies or arbitrary owner headers.

JSON fields are snake_case. Money is a signed 64-bit canonical decimal integer
**string**, with a configured currency exponent. Dates are ISO local dates,
months `YYYY-MM`, timestamps RFC3339. Unknown spending remains null. UUIDs identify
resources. Every response has `X-Request-ID` and `Cache-Control: no-store`.

All mutations require a UUID `Idempotency-Key`. Keys are owner-scoped and retained
indefinitely initially (at least 30 days). Repeating the same method, path, body,
and precondition replays the original status/body/ETag. Different content yields
409 `IDEMPOTENCY_CONFLICT`. Failed commands do not consume a key. Atomic state,
audit, and retry-response commits prevent duplicate writes.

Versioned mutations require `If-Match: "N"`; missing is 428, stale is 412.
GET resource responses expose ETag. A plan initially has version 1; an untouched
month has version 1. List endpoints accept `limit` (1–100, default 50) and `cursor`
(last UUID); return `{items: [], next_cursor: null}`. Entries and statements also
accept `month`. Use no API response caching.

Errors are `{error:{code,message,fields:{},request_id}}`. Statuses: 400 malformed,
401 unauthenticated, 403 non-owner, 404 inaccessible/missing, 409 domain conflict,
412 stale version, 422 invalid financial data, 428 missing precondition,
501 explicitly deferred capability, 503 dependency unavailable. Health endpoints
`/healthz` and `/readyz` are public and expose no configuration.

## Manual financial core (implemented in this slice)

| Method and path | Request | Result |
| --- | --- | --- |
| GET/PATCH `/settings` | SettingsInput for PATCH, versioned | Settings |
| GET/POST `/accounts` | AccountInput | Page / Account (201) |
| GET/PATCH `/accounts/{id}` | Full AccountInput for PATCH, versioned | Account; referenced accounts archived, never deleted |
| GET/POST `/entries` | EntryInput | Page / Entry (201) |
| GET `/entries/{id}` | — | Entry |
| POST `/entries/{id}/corrections` | EntryCorrection, versioned | Replacement Entry or voided original; original retained |
| GET/POST `/statements` | StatementInput | Page / Statement (201) |
| GET `/statements/{id}` | — | Statement |
| POST `/statements/{id}/revisions` | StatementRevision, versioned | New Statement (201), settlements migrate, history retained |
| POST `/statements/{id}/settlements` | SettlementInput, versioned statement | Settlement (201); references existing cash entry |
| POST `/statements/{id}/obligation-adjustments` | ObligationAdjustmentInput, versioned | Adjustment (201); affects payable only |
| GET/POST `/balance-snapshots` | SnapshotInput | Page / Snapshot (201) |
| GET/POST `/claims` | ClaimInput | Page / Claim (201) |
| GET/PATCH `/claims/{id}` | Full ClaimInput for PATCH, versioned | Claim |
| POST `/claims/{id}/receipt-matches` | ReceiptMatchInput, versioned claim | ReceiptMatch (201); capped receipt, claim, cost recovery |
| GET/PUT `/months/{month}/plan` | PlanInput for PUT, versioned | Plan; stable line IDs match actuals |
| GET `/months/{month}` | — | Month state/checklist/revision |
| GET `/months/{month}/summary` | — | Summary with funding, spending, payable, forecasts, coverage |
| POST `/months/{month}/close` | MonthTransition, versioned | Month; all checklist assertions required |
| POST `/months/{month}/reopen` | MonthTransition with reason, versioned | Month; revision increments |
| GET/POST `/recurrences` | RecurrenceInput | Page / Recurrence (201) |
| POST `/recurrences/{id}/revisions` | RecurrenceInput, versioned | Effective-dated successor (201); no automatic postings |
| GET/POST `/reconciliation-periods` | PeriodInput | Page / Period (201); observations only |
| GET/POST `/drafts` | DraftInput | Page / Draft (201); manual structured staging |
| GET `/drafts/{id}` | — | Draft |
| POST `/drafts/{id}/accept` | `{}`, versioned | Draft with accepted_record_id; posting committed atomically |
| POST `/drafts/{id}/reject` | `{reason}`, versioned | Draft; terminal transition |
| GET `/months/{month}/insights` | — | Rule-based observations with contributing values |
| GET `/legacy-month-summaries` | — | Page of read-only historical comparisons |
| GET `/audit-events` | — | Page of mutation history |
| GET `/exports` | — | Versioned lossless JSON financial state; never CSV by default |

Account kinds: bank, cash, credit_card, investment, savings. Explicit `active`
is required. Effective-dated funding boundary membership cannot silently restate
existing financial history. Settings currency is immutable after financial data.
Entry kinds: income, direct_expense, refund, reimbursement_receipt, transfer,
envelope_funding, envelope_return, allocation, allocation_withdrawal,
card_settlement. Amounts are positive; kind controls sign. External inflows have
only destination, outflows only source, owned transfers both. Reporting month
defaults to effective month; override requires reason. Refund/withdrawal reference
an original tracked entry and cannot exceed it. Corrections append replacement
history and preserve linked financial invariants; linked entries may require
explicit reconciliation rather than silent rewriting.

Statements default to due month. Either all four spending components are supplied,
or an evidenced net amount, or spending is unknown. Never infer spending from due.
Carry retires the earlier payable only; its historical spending stays. Settlement
must be after cutoff, to the same card, within the cash entry amount. Revisions
cannot move settled or recovered cost across history silently. Adjustments after
cutoff reduce payable once. Excess paid is shown separately as credit.

Claims never create cash. Receipt matching may reduce cost only when a recognized
expense or statement is linked, within unrecovered cost. Receipt month and recovery
month are separate; closed recovery months reject posting. Unmatched receipts
make personal-cost coverage incomplete. Closed months allow draft staging but
reject direct posting and acceptance. Recurrences never imply actual payment.

## Contracted later capabilities

The following operations have complete request/response schemas and explicit
`x-implementation: deferred`. They return 501 `FEATURE_NOT_AVAILABLE` until their
planned integration milestone; clients must not treat them as working integrations.
No scheduler, provider credentials, automatic parsing, paid model, or infrastructure
is provisioned by this slice.

| Method and path | Request / response and transition |
| --- | --- |
| POST `/imports` | ImportRequest (source kind, external ID, content); 202 ImportJob, queued → running → needs_review/failed; no direct posting |
| GET `/imports/{id}` | ImportJob with draft IDs, retry_at, redacted failure |
| POST `/gmail/connect` | GmailConnectRequest (allowed redirect URI); GmailConnectResponse (authorization URL, expires_at); state/PKCE verified on callback |
| GET `/gmail/callback` | code/state query; GmailConnection; disconnected → connected only after verified callback |
| POST `/gmail/sync` | GmailSyncRequest (bounded date interval); 202 ImportJob; no concurrent duplicate source posting |
| DELETE `/gmail/connection` | Empty body; GmailConnection disconnected; preserves financial records |
| POST `/capture/text` | CaptureRequest (text, captured_at); CaptureResponse with review draft IDs; never posts |

OAuth connection auth is separate from app login. Attachments and original bank
parser conventions require samples and provider work in P5/P6. JSON export is
implemented; CSV and restore are later portability work. Shared fixtures are
synthetic; no spreadsheet parity or bank balance is claimed.

A claim with only unclassified receipt matches may later acquire a recognized
cost link through its versioned PATCH. To classify an existing receipt allocation,
POST the same receipt/claim/amount to `receipt-matches` with an explicit
`recovery_month`. This promotes that match once, preserving its ID and incrementing
its version (201); it does not create cash or allocate the receipt again. Cost caps
and open receipt/recovery months are required. Already recovered matches conflict.
Claims in monthly summaries are anchored to expected receipt month, otherwise
linked cost month; undated/unlinked claims remain in the claims backlog. A receipt
in a later month does not restate an earlier month’s outstanding claim amount.

## Monthly forecast and claim timing

Recognized and funding plan matches are independent (`plan_line_id` and
`funding_plan_line_id`). Numerical forecasts may be visible before checklist review;
`is_provisional` remains true. Unknown or missing statement spending keeps projected
surplus null. An unpaid statement without an explicit card-specific
`funding_outflow` plan has no known funding source, so projected funding remainder
is null with an `obligation_funding_plan_missing` item; actual funding coverage
remains separate. Statement issuance alone never assumes a boundary payment.

Monthly unmatched claim totals use the expected receipt month, falling back to the
linked cost month. Undated, unlinked claims remain in the claims list. Later-month
receipts do not rewrite earlier expected-month outstanding amounts. Receipt cost
classification can be resolved after initial matching: when all matches are still
unrecovered, update the claim cost link, then match the same receipt/claim/amount
with an explicit recovery month. This promotes the existing match once, preserving
its ID and incrementing its version, without creating another receipt allocation.

An incomplete plan line with remaining expected items emits `planned_item_pending`;
received zero differs from an explicitly completed zero-income line. Actual
coverage remains provisional until reviewed/complete, while a numerically known
forecast can still be shown. `savings_rate` is a decimal ratio string rounded to
six decimal places, unavailable for nonpositive income or incomplete spending.
