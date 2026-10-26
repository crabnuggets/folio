# Accounting and data contract

This document is authoritative for financial semantics. Amounts in examples are fictional currency units. The application uses one configured currency initially; do not silently convert or add other currencies.

## Dates and amounts

Store monetary amounts as integer minor units in PostgreSQL `bigint`. Send them as decimal integer strings in JSON to avoid JavaScript precision loss. Validate range and currency exponent; do not assume all currencies have two decimal places. No floating-point arithmetic for money. Percentages use decimal/rational arithmetic and specified rounding only at display boundaries.

Store event dates and due dates as ISO local dates, reporting month as the first date of a month, timestamps as UTC instants, and the user's timezone as an IANA name. Convert spoken relative dates using the capture timestamp and user timezone. Default reporting month follows receipt/payment date except statements, which use due month. Preserve overrides and reasons.

## Financial effects

| Event | Recognized spending | Income | Allocation | Cash / obligation effect |
| --- | --- | --- | --- | --- |
| Salary received | 0 | Positive | 0 | Cash inflow observation |
| Direct expense | Positive | 0 | 0 | Cash outflow observation |
| Card new cycle spending | Positive, net of cycle refunds | 0 | 0 | Statement separately declares obligation |
| Card payment | 0 | 0 | 0 | Cash outflow; linked amount reduces obligation |
| Transfer to investments/savings | 0 | 0 | Positive | One transfer between owned accounts |
| Ordinary internal transfer | 0 | 0 | 0 | No consolidated wealth change |
| Investment withdrawal to spendable cash | 0 | 0 | Negative | Reverses available allocation; not salary |
| Market gain/loss snapshot | 0 | 0 | 0 | Valuation only; not spendable income |
| Direct-expense refund | Negative, linked when possible | 0 | 0 | Cash inflow observation |
| Opening card debt | 0 | 0 | 0 | Opening obligation only |
| Envelope funding / return | 0 | 0 | 0 | Transfer crossing funding boundary; not consumption |
| Matched reimbursement receipt | Negative personal-cost recovery once | 0 | 0 | Confirmed receipt; pending claim has no cash effect |

A movement has one purpose: ordinary transfer, allocation, allocation withdrawal, or card settlement. It cannot create both a standalone expense and a statement payment. Withdrawal and refund caps require review when the original records are outside tracked history; do not invent missing historical events.

## Statement semantics

Keep `new_charges`, `fees`, `interest`, and `refunds` separate where available. Recognized statement spending = new charges + fees + interest − refunds. A bank-provided, clearly labeled net new cycle spending figure can substitute with evidence. Refunds already netted into new charges must not be deducted again; each bank parser documents its field convention.

Keep statement amount due, minimum due, previous balance, and cycle payments separately. A statement balance is an observation as of cycle end, not a new debt to add on top of the previous statement balance. Carry links identify obligations rolled into a later statement; retire their payable representation when superseded so future commitments do not double-count old debt. Historical statements and payment links remain intact.

Remaining payable = max(0, active statement amount due − settlement amounts applied after its statement cutoff − explicit subsequent obligation credits). Do not subtract payments already included in the statement's printed amount due. Excess payments create a separately displayed credit, not negative spending. A refund reduces spending once and may reduce the obligation once; those are different effects of the same adjustment.

If only total due is available and it is unclear whether prior debt is included, the payable amount is known but spending is unknown. Show an incomplete summary and ask for net new spending or explicit confirmation of a zero prior balance and no unmodeled adjustments. Never infer “paid” from the arrival of a statement or from expected autopay.

Partial payments, carried balances, credits, and revised statements must be modeled before automatic posting is considered. A revised statement supersedes the old version and recomputes derived values; it does not add another expense.

## Summary formulas

For month M, using confirmed, active records:

- `I` = received income assigned to M.
- `E` = direct expenses net of direct refunds + net new statement spending assigned to M − matched expense recoveries assigned to M.
- `A` = completed allocations minus allocation withdrawals assigned to M.
- `monthly_surplus = I - E - A`.
- `consumption_savings = I - E` (before allocations); an investment transfer is not consumption.
- Savings rate is `(I - E) / I` only when I > 0; otherwise show unavailable. Explain that this is based on tracked coverage.

For projections, use monthly plan lines with stable identities. Each line has an expected total and zero or more matched actual records. If marked complete, forecast equals its actual total. If incomplete, forecast equals actual total plus explicitly estimated remaining amount. When an expected full total is supplied, default remaining to `max(expected - actual, 0)` for normal nonnegative lines; signed corrections/withdrawals require explicit remaining estimates. Unplanned actuals are added once. This prevents counting salary or a statement both as expected and actual. Projected surplus is forecast income minus forecast spending minus forecast allocations.

Surplus excludes opening cash and is not a spendable bank balance. Optional cash planning starts from a coherent set of recent snapshots and subtracts only known unpaid commitments after each snapshot cutoff; expected future receipts are separately labeled. If chronology or coverage cannot be established, omit a combined available-cash figure. Never subtract all monthly spending from a current snapshot that already reflects payments.

## Worked fixture

| Item assigned to October | Amount | Effect |
| --- | --- | --- |
| Received salary | 5,000 | I = 5,000 |
| Card A new spending; due October | 1,200 | E +1,200 |
| Card B new spending; due October | 300 | E +300 |
| Direct rent | 1,000 | E +1,000 |
| Completed investment transfer | 800 | A +800 |
| Card A settlement | 1,200 | No additional E |

E = 2,500; A = 800; monthly surplus = 1,700. If Card B remains unpaid, its 300 obligation still appears. Paying it later changes cash/obligation status, not October spending. A 100 refund assigned to October reduces E to 2,400 and raises surplus to 1,800; it is not salary. A November statement with 300 carried balance and 200 new charges contributes 200 of spending, with an active payable of 500 before subsequent payments; the old 300 is not another active payable.

If planned income is 5,000 and received income is 5,000, forecast income stays 5,000. If an 800 allocation is planned and 300 completed with 500 still expected, forecast allocation is 800, not 1,100. Unknown Card B spending makes E a known subtotal and surplus provisional, never a complete result.

## Persistence model

Every owner-controlled table has `owner_id`, UUID identity, created/updated timestamps, and a version for concurrency. Foreign keys must preserve ownership, using composite owner/id constraints where appropriate.

| Entity | Essential fields / constraints |
| --- | --- |
| user_settings | owner, currency, timezone, onboarding state; only one owner allowed in deployed personal instance |
| accounts | alias, kind, currency, include_in_liquid_cash, active; archive instead of deleting referenced accounts |
| reporting_months | month, open/closed state, closed_at, revision, checklist assertions |
| plan_lines | month, kind, expected amount, estimated remainder, complete flag, optional recurrence identity |
| entries | kind, amount, currency, effective date, reporting month, account references, category, plan line, source, reversal link |
| statements | card, cycle start/end, issue/due dates, amount due, minimum due, spending components, spending-known flag, reporting month, active revision, carry/supersession links |
| settlements | movement entry, statement, applied amount, effective date; aggregate applied amount cannot exceed movement amount |
| obligation_adjustments | statement, amount, date, reason, linked refund where relevant; no duplicate effect |
| balance_snapshots | account, amount, observed_at, source; historical observations retained |
| recurrences | kind, amount estimate, schedule, active interval; creates unique month/recurrence plan lines |
| import_sources | provider, external ID, content hash, parser version, evidence metadata; unique owner/provider/external ID |
| drafts | source, candidate fields, validation state, field evidence, duplicate links, accepted record ID |
| jobs | kind, unique dedupe key, status, attempts, retry_at, lease expiry, redacted failure |
| audit_events | owner, action, entity, timestamp, correlation ID, before/after reference and reason |
| idempotency_records | owner, key, request hash, operation, response; transactionally coupled to mutation |

Statement spending is derived from the active confirmed statement and its adjustments; do not also persist an independent expense entry for it. Direct expenses use entries. Enforce allowed signs by kind: positive income, expense, allocation, payment; dedicated refund/withdrawal kinds supply negative summary effects. Statement net spending may be negative. Export preserves source IDs and revisions.

## Mutation rules

All writes validate owner, currency, month state, and expected version. Accepting a draft atomically writes the financial record, evidence links, audit event, and accepted status. Unique constraints and transactional locks prevent two concurrent accepts. Corrections append revision/reversal information rather than erasing history. Statement revisions keep settlement associations valid or explicitly flag them for rematching. Soft deletion/voiding excludes an item from summaries but preserves evidence. Closed-month imports remain drafts until the user reopens the month.

Keep source identities indefinitely while their financial records exist; short-lived HTTP retry keys alone are insufficient duplicate protection. Similar amount/date/card combinations produce review candidates, not unconditional deletion: two legitimate identical payments can occur.

## Source-informed extension: funding boundary, claims, and periods

This section supplements the original spending model following spreadsheet inspection and the user's confirmation. See [ADR 0003](../adr/0003-funding-boundary-and-reimbursements.md). The primary monthly review now includes a funding remainder; recognized-spending surplus remains a separate metric.

Add `is_funding_account` to accounts, distinct from `include_in_liquid_cash`: the personal spending account can contain liquid cash while being outside the selected funding boundary. Every cash-affecting entry has an owned source account, owned destination account, or external counterparty marker as applicable, with an explicit movement purpose. A salary/direct-expense entry itself is its cash movement; do not create an additional duplicate movement. Statements are non-cash records; linked payment entries represent their cash effects. Reject foreign-owned account links.

`funding_remainder(M) = sum(confirmed movements into the funding boundary in M) - sum(confirmed movements out of that boundary in M)`.

Transfers entirely inside or outside the boundary contribute zero. Envelope funding/return are ordinary owned transfers with dedicated purposes, excluded from I, E, and savings/investment A. Savings and investment transfers out of the boundary contribute to funding outflows and A in their respective views; these metrics are not added together. Cash receipts/settlements count only when confirmed, using their explicit reporting month. Include reporting-month overrides in the audit trail.

Synthetic fixture: salary 5,000; envelope funding 800; returned excess 150; card payment 1,000; house savings 600; investment transfer 400; family allowance 200; received reimbursement 100. Funding inflows = 5,250; outflows = 3,000; remainder = 2,250. A 1,000 statement linked to the payment contributes no additional funding outflow. A pending claim of 300 contributes no receipt. This fixture does not establish actual consumption in the personal spending account.

Projected funding remainder uses expected boundary cash movements: match actuals by stable plan-line identity and add only expected remaining cash amounts. Unpaid obligations contribute once if expected in the month. A statement's recognized spending line must not also become a second cash commitment alongside its payment plan. Pending reimbursements can appear as explicitly uncertain forecast receipts only with an expected date and user inclusion; exclude them from actuals and conservative available-cash calculations.

Add entities:

| Entity / extension | Required semantics |
| --- | --- |
| claims | Expected reimbursement amount, related expense/statement if known, expected receipt date, state, source evidence; outstanding amount derived from linked receipts and adjustments |
| reimbursement_receipts | Cash entry kind linked to one or more claims; each receipt allocation bounded by received amount; never income kind salary |
| expense_recoveries | Link confirmed reimbursement to personal-cost reduction exactly once; over-recovery requires review; unknown expense linkage remains unresolved |
| reconciliation_periods | Joint worksheet date range, explicit reporting month, additions/deductions, remainder, contribution proposal, and evidence; no automatic cash posting |
| legacy_month_summaries | Imported original formula result, source coordinates, completeness flags; read-only reconciliation data excluded from live sums |
| recurring template history | Effective start/end months and version; changed targets affect future plans, not closed historical months |

Track pending, partially received, received, disputed, and waived claim states. A confirmed recovery reduces E in its assigned recovery reporting month, separate from gross expense and from I; if the original cost was not recognized, do not fabricate negative spending—keep the recovery unmatched pending classification. A bank refund and a personal reimbursement are distinct sources but cannot both reduce the same cost twice accidentally. Flag possible overlaps for review. The receipt affects cash at receipt time regardless of whether cost matching is complete. Unknown matching makes net-personal-cost coverage incomplete, not necessarily the cash-funding calculation.

Period additions/deductions are reconciliation observations, not automatically owned-account movements. A proposed joint top-up is `max(0, -period_remainder)` unless the user chooses another explicit rule; actual transfer needs confirmation. Never count both the contribution boundary and underlying joint purchases as separate personal expenses. Subscription labels alone create optional checklist entries with null amount/date; they are not confirmed charges or extra expenses on top of a card statement.

Changing the funding-account boundary can change historical projections. Store its version/effective month and require an explicit comparison before restating past reports. Preserve the default historical view. Funding completeness and spending completeness are separate flags: a missing spending breakdown does not invalidate known cash payments, and a complete statement does not prove it was paid.
