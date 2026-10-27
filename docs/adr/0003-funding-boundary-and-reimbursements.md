# ADR 0003: Separate funding remainder from recognized spending

Status: Required workflow grounded in source inspection and user confirmation; detailed implementation is proposed. Date: 2026-10-26.

## Evidence and change from ADR 0002

The inspected workbook includes salary, reimbursements, recurring funding allocations, card amounts, and returned personal-spending excess in a monthly inflow/outflow calculation. The user explicitly confirmed that “Monthly expenses” is a transfer to a separate personal spending account and the excess is unused funds returned from it.

ADR 0002 remains useful for recognized spending but is insufficient as the primary replacement for this workflow: an ordinary transfer does not affect consumption surplus even though it reduces funds available in the main funding account. Add a separate funding view rather than coercing transfers into expenses. See [source mapping](../planning/08-spreadsheet-mapping.md).

## Decision

Let the user designate a set of funding accounts, initially the main income/bill-paying account. Record actual cash movements with source/destination, purpose, amount, date, and reporting month. Monthly funding remainder is confirmed cash entering that boundary minus confirmed cash leaving it. Internal transfers between two selected funding accounts cancel; transfers to a personal spending account outside that boundary reduce the remainder, and returns increase it.

Salary, received reimbursements, envelope returns, direct expenses, card settlements, savings transfers, and investment transfers have distinct labels. Statement issuance is not a cash movement. A card payment affects funding once, but not recognized spending again. Historical card figures without a known meaning remain staged; do not automatically label them paid.

This remainder excludes opening balances and is not a current account balance. Keep observed balances separately. Preserve original workbook totals as legacy comparisons, not authoritative postings. Use actuals and expected remaining cash commitments for a separate projected funding remainder; never count a paid bill and its unmatched estimate twice.

Track claims and receipts independently. Pending claims are not received cash. A receipt linked to an expense reduces net personal cost once, without classifying it as salary. Receipt and expense may occur in different months; do not silently reopen a closed month. Clearly show timing differences between gross spending, recoveries, and cash receipts.

## Consequences

Both clients show the funding view first and recognized-spending analysis separately with coverage. Exact consumption is not inferred from envelope funding alone. Do not calculate personal spending as funding minus returned excess unless opening/closing envelope balances and all other flows are known. Maintain a single set of financial records with projections, not two editable ledgers.

The app remains single-user. Joint contribution attribution and historic claim/payment completion still require review; this decision does not grant shared account access or imply a specific ownership share.
