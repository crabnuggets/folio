# Product and experience

## Goal

Make the monthly check-in a short review of income, card bills, major direct expenses, and savings/investment contributions. Automate collection where feasible and expose exceptions clearly. The user should understand whether the month is on track without classifying every purchase.

Initial regional target: Singapore; proposed base currency SGD and timezone Asia/Singapore. Gmail is the confirmed email provider. The inspected workbook names Citi Cash Back+ Card and Maybank XL Cashback Card; current active status and bank statement formats still need confirmation.

## Scope and priorities

| Priority | Capability | Completion condition |
| --- | --- | --- |
| Core | One private user, account/card setup, configurable currency and timezone | Only the owner can access financial records |
| Core | Manual salary, statement, major expense, allocation, payment, and snapshot entry | Correct monthly summaries with edit/reversal history |
| Core | Expected monthly plan and completeness indicators | Missing bills and unreceived income are visible |
| Core | Responsive browser dashboard and export | Usable on desktop and phone; data portable |
| Required later | Native Android client | Same authenticated data and totals as web |
| Automation | CSV/manual statement imports, then a supported email provider | Retry-safe drafts with evidence and review |
| Enhancement | Voice entry and insights | Optional, grounded, and fully usable without paid inference |

Non-goals: initiating bank payments, trading, storing bank login credentials, scraping banking apps, real-time open-banking coverage, tax filing, shared household logins, exact daily cash balances from partial data, and mandatory transaction-level budgets. Direct bank APIs are a future country-specific investigation, not an assumed free capability.

## Monthly dashboard

- Month selector and a visible status: incomplete, ready to review, or closed.
- Primary monthly funding remainder, funding inflows/outflows, and expected commitments; secondary recorded income, recognized spending, completed savings/investment allocations, and monthly surplus.
- Projected surplus with expected-versus-actual comparisons. Negative indicates a planned shortfall, not necessarily an overdraft.
- Card rows: new spending, statement amount due, due date, remaining obligation, payment status, and evidence/review status.
- Major direct expense and allocation rows. No required merchant feed.
- Observed spendable cash with account-specific snapshot times; never label monthly surplus as cash in the bank.
- Coverage checklist: expected salary received, all active cards accounted for, major expenses reviewed, allocations checked. Explicit “no statement this month” with reason is different from missing data.
- Insights with linked contributing figures and data gaps. Card-only totals support comparisons between cards, not claims about groceries or restaurants.

## Essential flows

**Setup:** choose currency and timezone; create account aliases and card cycle/due-date hints; enter optional opening obligations and snapshots; add expected salary, bills, and target allocations. Bank name and last four digits are optional identifiers, not credentials.

**Monthly review:** open month → review imported drafts → match possible duplicates → confirm or correct amounts and months → review missing items → reconcile payments → review surplus → close month. Closing is a user assertion that review is complete, not proof of exhaustive financial coverage.

**Quick entry:** choose income/expense/allocation/payment → amount and date → account/category → confirm. Defaults reduce typing but cannot silently choose between ambiguous accounts.

**Statement import:** upload or paste values → show extracted total due, new spending, due date, and source snippets → resolve ambiguity → confirm. Distinguish minimum payment from full statement amount.

**Correction:** show original record, reason, and revised impact. Closed months must be explicitly reopened before their financial data changes. Reopening increments the summary revision.

**Android:** dashboard, quick entry, review inbox, statement details, and settings. File sharing into the app can follow core parity. Handle loading, expired sessions, offline state, API cold starts, and retry without duplicate writes.

## Spreadsheet-informed additions

Follow [the source mapping](08-spreadsheet-mapping.md). Include reimbursement/claim tracking, effective-dated recurring allocations, optional subscription checklists, and a single-owner joint-period reconciliation view. Card totals may contain reimbursable spending: show gross charges and known personal cost separately rather than treating every high card bill as overspending. Preserve historical spreadsheet balances as comparison records; never label their cumulative sum as bank cash. The user confirmed that “Monthly expenses” funds a separate personal spending account and excess is unused money returned. A separate funding view is required; these movements are not salary or consumption. See [ADR 0003](../adr/0003-funding-boundary-and-reimbursements.md).

## UX quality

Use accessible labels, keyboard navigation, adequate contrast, large touch targets, localized currency/date formatting, and explicit minus signs. Charts supplement numeric tables. Never rely on color alone. Avoid technical model scores on the main dashboard; show “Needs review” and explain the missing field.

## Success measures

Measure against the user's first manual baseline rather than invented performance claims. Target a monthly review under ten minutes after supported imports are configured; zero duplicate postings from retries; every displayed total traceable to records; all seeded accounting scenarios pass on both clients. Measure import confirmation/edit/rejection rate and time saved; do not optimize automation rate at the expense of correct amounts.
