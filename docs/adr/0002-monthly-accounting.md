# ADR 0002: Summary spending with separate cash movements

Status: Proposed recognized-spending baseline, supplemented by ADR 0003 after spreadsheet inspection. Date: 2026-10-26.

## Decision

Track new credit card cycle spending as one summarized expense, normally assigned to the statement's due month. Track settlement separately as cash movement. Keep total amount due and remaining obligation separate from recognized spending. Income uses receipt month; direct expenses use payment month; allocations use transfer month. Explicit reporting-month overrides are allowed and audited.

This is personal monthly planning, not statutory accounting or a complete double-entry ledger. Partial tracking cannot reconstruct accurate bank balances or net worth. Use dated balance snapshots for those views.

The source workbook and user confirmation establish a separate funding-boundary view as the primary monthly check-in. [ADR 0003](0003-funding-boundary-and-reimbursements.md) adds this view without reclassifying internal transfers as consumption. Historical source months are preserved; due-month attribution remains a proposal for new statement spending, not an observed workbook convention.

## Why

A statement, bank debit, and payment notification can describe the same underlying spending. Counting each as an expense overstates spending. A total amount due can also include old debt; recognizing it every month would repeatedly count the same expense.

## Consequences

The UI must distinguish monthly surplus, projected surplus, and observed cash. Missing statement components yield unknown spending and a review task. They do not silently turn the full outstanding balance into new spending. Legacy opening card debt becomes an opening obligation without a current-month expense. Monthly budgeting and payment timing may differ; both are displayed.

A future transaction-level import must reconcile into existing statement totals, not add a second expense stream. Changing the reporting convention later requires an explicit migration preview and revalidation of closed months.
