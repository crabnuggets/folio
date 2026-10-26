# Accounting Days: inspected structure and migration plan

Inspected read-only on 2026-10-26 through the Google Drive connector. The exact-title workbook (with a trailing space in its title) was selected; a similarly named “Master Expenses & Balance Sheet” was not used. No source cells were changed. Private amounts, personal remarks, and account credentials are intentionally absent from repository documentation. Source identity and full-fidelity values belong in a private migration manifest, not committed fixtures.

## Inspection coverage

Metadata confirms timezone Asia/Singapore and locale en_US. Dollar formatting alone does not prove currency; SGD remains the proposed configured currency.

| Tab | sheetId | Read ranges | Observed structure |
| --- | --- | --- | --- |
| 2026 Balance Sheet | 1608450678 | A1:AF18, A19:N115 | March–October blocks, recurring components, inflows, card totals, monthly and cumulative balances |
| Joint Spendings v1 | 506484232 | A1:AB12, A13:I65 | Period-based contributions, company fuel claims, fuel/carpark deductions, period remainder |
| Monthly Subscriptions | 1533387758 | A1:H16 | Four service names; no amounts or billing dates in inspected cells |
| Income Balance Sheet | 1083329131 | A1:Q14, A15:F45 | December 2025 salary/allocation layout and a residual formula |

These are bounded reads, not proof that every other cell is empty. Formulas and displayed values were inspected. No bank PDFs or account transactions were available; the spreadsheet cannot establish payment dates or full statement component semantics.

## Source-to-domain mapping

| Source | Observed meaning | Proposed mapping and caveat |
| --- | --- | --- |
| 2026 A4, A15, A25, A36, A46, A56, A66, A76 | Month labels initiating variable-height blocks | Explicit reporting month in year 2026; preserve source month, do not infer due date |
| C/E/G in each block | Inflow label, amount, remarks | Classify salary, reimbursement, refund, or internal return separately; “Debit” does not imply accounting debit semantics |
| I/K/M | Outflow label, summarized amount, remarks | Direct expense, allocation, statement/payment observation, or transfer pending classification |
| J/L within recurring groups | Detail label and amount beneath K subtotal | Normalize leaf components once; subtotal is a check, not a second posting |
| House savings; Investment | Recurring funding allocations | Savings/investment targets and actual transfers only when completion is established |
| Monthly expenses | Repeated fixed amount | User-confirmed transfer to a separate personal spending account; classify as envelope funding, not consumption |
| Personal spending account excess | Inflow on several months | User-confirmed return of unused envelope funds; classify as envelope return, never salary |
| Family allowance | Recurring outgoing support | Distinct expense category, with planned/paid state confirmed |
| Card rows, including J33:K34 and later equivalents | Named card amounts | Candidate cards: Citi Cash Back+ Card and Maybank XL Cashback Card; identify whether figures are statement due, net spending, or actual payments |
| Early repayment row | Additional credit card payment label | Separate payment observation; reconcile against statement totals to avoid counting spending twice |
| Refund and claim inflows | Refunds/reimbursements, including compound formulas | Distinguish received cash from pending claims and link expense/card only where evidence supports it |
| E/K total rows | Formula totals for each block | Preserve original result as legacy comparison data; do not import as events |
| N monthly result cells | Difference between block inflow and outflow totals | Legacy monthly review balance, not necessarily consumption surplus |
| E1 = SUM(N:N) | Sum of monthly results | Cumulative tracked remainder; not an observed bank balance |
| Joint A:C, D:E, G:I | Period dates, additions, deductions, notes | Period-based reconciliation worksheet; own date range distinct from calendar month |
| Joint D total cells | Additions minus deductions | Derived period remainder; can inform proposed top-up, not an automatic transfer |
| Monthly Subscriptions A1:A4 | Zwift, Strava, Google, Simba | Optional subscription checklist seeds; amount/date/payment-account unknown |
| Income Balance Sheet A/B | Earlier salary, savings, ETFs, housing, joint spending, allowance, personal-expense layout | Historical template/reference; do not assume a second set of actual transactions |

Trim whitespace when matching aliases but retain original labels in private evidence. Blocks have varying lengths; parsing must locate month/total labels and formula dependencies rather than assume a fixed number of rows per month. Numeric Excel/Sheets dates and text such as an end date embedded after “to” both occur in the joint worksheet.

## Accounting implications

The workbook is a monthly funding-and-reconciliation worksheet. It combines savings, investment transfers, expense totals, claims, and possible transfers under inflow/outflow headings. Therefore its balance need not equal the proposed recognized-spending surplus.

Retain three distinct concepts:

1. **Legacy monthly review balance:** imported original formula result with source coordinates and completeness/reconciliation flags. Read-only comparison, excluded from live postings.
2. **Monthly surplus:** deterministic income minus net recognized spending minus completed savings/investment allocations, as defined in the accounting contract. Reimbursements reduce matched personal cost rather than inflate salary.
3. **Observed cash:** dated account snapshots. The sheet's cumulative result cannot seed a bank balance without independent confirmation.

The user confirmed both personal spending-envelope funding and return of unused funds. Implement an explicit funding view: transfers from the main funding account into the envelope reduce available funding at that boundary; returned unused funds increase it. Across owned accounts these remain transfers, not consumption. Label this view separately and derive it from actual movement records. Do not treat funding and the later purchases as two expenses. This funding view is a core requirement; individual historical payment/transfer dates and completion status still require mapping. See [ADR 0003](../adr/0003-funding-boundary-and-reimbursements.md).

Joint-expense tracking remains a single-owner feature; it does not require shared logins or a household product. The period worksheet can inform contribution decisions without generating both joint contributions and underlying fuel expenses as personal spending. Choose contribution-boundary or attributable-expense reporting explicitly before posting historical rows.

## Reconciliation findings requiring decisions

- May inflow total E35 uses `SUM(E25:E27)` and excludes the populated refund at E34. This may be deliberate (for example, a refund already reflected in the card figure), or an omission; evidence does not settle it.
- September inflow total E75 uses `SUM(E66:E67)` and excludes the populated claim at E68. Confirm whether it was received, deferred, already netted elsewhere, or inadvertently omitted.
- October contains blank car-expense and Maybank-card amounts while totals still evaluate. Display this month as provisional until explicitly marked zero/not applicable or completed; spreadsheet SUM treating blank as zero is not evidence of completeness.
- Joint D18 feeds the March contribution formula; D21 feeds April's additional-contribution formula through ABS. An absolute value of a future positive remainder would incorrectly create a top-up if copied blindly. A deficit-only proposal uses `max(0, -remainder)`, subject to the user's contribution policy.
- A joint period label and its supporting bill-date remark are not always aligned. Preserve both and request reporting attribution instead of silently shifting dates.
- Card due dates, payment status, prior balances, new-spending components, and whether reimbursements are already netted are not established by these cells.

Do not “fix” the workbook or silently reproduce questionable formulas as authoritative app rules. These findings are migration review flags, not a completed financial audit.

## Migration and validation sequence

1. Reread the relevant cells at migration time and capture spreadsheet revision/read timestamp in a private manifest. Store source workbook ID, sheetId, range, original formula/value, and stable import identity privately.
2. Stage normalized leaf rows and original formula totals. Do not evaluate arbitrary imported formulas in application code; use read values and a constrained dependency mapping.
3. Resolve pending-versus-received claims, planned-versus-completed allocations, historical envelope transfer completion/dates, joint boundary, and card spending-versus-payment semantics.
4. Generate two comparisons per month: original workbook result versus a faithful legacy replay, then legacy result versus normalized app summary with each semantic difference explained.
5. Use at least April (joint adjustment), May (refund exclusion), and September (claim exclusion), plus October (incomplete month). Run reconciliation privately; only anonymized synthetic analogues belong in tests.
6. Accept historical postings only after mapping review. Keep unresolved items and original summaries in staging; source row identity prevents duplicate imports.

Synthetic regression cases: a refund outside a SUM range is flagged rather than auto-added; a pending claim creates no cash receipt; a recurring subtotal plus its children is counted once; a positive joint remainder creates no deficit top-up; blank card amount means unknown; envelope return is not salary; cumulative remainders never become a bank snapshot. Full migration and financial parity have not yet been performed.
