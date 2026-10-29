# A simpler monthly Folio

Status: UX proposal for review, 2026-10-29. The user supports the simpler direction and requested history-based name suggestions and thoughtfully placed monthly recurrence. This proposes a replacement for the current manual experience. It does not change the running app or migrate existing data.

## The job

Open a month, enter the high-level amounts coming in and going out, and see the monthly remainder. The monthly sheet is both the place to enter amounts and the place to review them.

The attached workbook's `2026 Balance Sheet` is the reference. It contains named monthly amounts, recurring components, individual credit cards, reimbursements, returned personal-spending money, occasional outgoings, and short remarks. Its layout already supports the user's main task. The app should remove the spreadsheet's manual copying and formula maintenance without adding accounting administration.

The current app exposes seven destinations. An ordinary entry asks for an entry kind, dates, reporting month, accounts, and optional matching fields. A card adds statement dates, spending evidence, components, settlement, and obligation workflows. Those workflows make simple monthly totals expensive to enter.

## Proposed language

These terms are proposed for the new experience. They are not yet replacements for the existing glossary's accounting definitions.

| Term | Meaning | Examples |
| --- | --- | --- |
| Monthly sheet | The named amounts assigned to one calendar month. | October 2026 |
| Section | A heading organizing related items. Its subtotal is calculated. | Recurring expenses, Credit cards |
| Item | Something the user tracks by name across months. | Salary, House savings, Citi Cash Back+ Card |
| Monthly amount | The amount recorded against an item for the selected month. | A card's single monthly figure |
| Money in | Amounts assigned to the incoming side of the sheet. | Salary, received reimbursements, returned spending money |
| Money out | Amounts assigned to the outgoing side of the sheet. | Card amounts, allowance, savings and investment contributions |
| Monthly remainder | Entered money in minus entered money out for that month. | A positive or negative monthly result |

Use the section's actual name in the interface. The user clicks `Add item` under `Credit cards`, not `Create group` followed by `Create record`. Avoid `transaction` for an item that summarizes an entire month.

Keep **account** for a real bank, savings, investment, cash, or card account. A card item may refer to an account, but an allowance or reimbursement is not an account. Creating a monthly item must not require creating or configuring a financial account.

## The main screen

```text
Folio                         October 2026  [month selector]

Monthly remainder                         [calculated amount]
Money in [total]                         Money out [total]
Based on entered amounts. [N items still blank, when applicable]

Money in
  Income
    Salary                                      [amount]
  Other money in
    Reimbursement                               [amount]
    Personal spending account excess            [amount]
    Add item

Money out
  Recurring expenses                            subtotal
    Personal spending account top-up            [amount]
    House savings                               [amount]
    Mummy's allowance                           [amount]
    Investment                                  [amount]
    Use usual amounts
    Add item
  Credit cards                                  subtotal
    Citi Cash Back+ Card                        [amount]
    Maybank XL Cashback Card                    [amount]
    Add item
  Other money out
    Car expenses                                [amount]
    Add item
```

Money in and money out can sit beside each other on a wide screen and stack on a phone. Keep labels, amounts, section subtotals, and the headline in one reading flow. Do not put a dashboard between the user and these inputs. Previous months use the same screen through the month selector; a separate history dashboard is unnecessary for the first version.

The prototype uses sample amounts, not copied personal figures. Open [the throwaway flow prototype](../../apps/web/prototypes/monthly-sheet.prototype.html) directly in a browser. It asks whether reusable monthly items can capture this workflow using names and amounts alone. Its guided scenarios exercise that model; it is not an alternative production client.

## First use

1. Open the current month. Show empty suggested sections immediately.
2. Add an item inside the relevant section. Only **Name** and **Amount** are required. The current month, direction, section, and configured currency are already known.
3. Save the item. Its amount contributes to the month immediately.
4. Continue adding items or stop. There is no prerequisite account wizard or month-closing ceremony.

Suggested section names are editable organization, not accounting types. An empty sheet is usable without accepting the suggestions. Currency is one preference with a clear default to confirm, not a field repeated on each item. Dollar formatting in the reference alone does not establish its currency.

## Suggestions while adding an item

The **Name** field suggests items from the user's saved history as they type. Show a short list of matching names below that field, with matches from the current section first, then other sections on the same incoming or outgoing side. Within those groups, prefer frequently and recently used items. Ignore capitalization and extra spaces when matching and deduplicating names. An empty history simply leaves the ordinary text field.

Selecting a suggestion reuses that item in the selected month and section. It does not create a second item identity or copy a previous month's amount. Keep the amount blank for the user to enter, and retain any amount they have already typed. A usual-amount suggestion follows the same rules as a recurring row; history alone never makes an amount current.

Include the section name when needed to distinguish similar names. Keep free typing available so adding something new takes no extra step. Suggestions must support keyboard selection, touch, and dismissal without forcing a match. This is ordinary history lookup and needs no AI integration.

If the selected item already appears in this month, focus its existing row instead of adding another one or replacing its amount. This also applies to a typed name that matches an existing item after ignoring capitalization and extra spaces. A historical suggestion keeps its saved recurrence preference; reusing a one-off item does not silently make it recurring. Changing a reused item's section applies to this month's placement without rearranging earlier months.

## Setting an item to repeat monthly

Place an optional **Repeat monthly** switch directly below Amount in the inline add form. Keep it off for a new item, including a new card, until the user chooses it. Name and Amount remain the only required fields. This makes recurrence available while the user is already naming the item, without a separate setup screen or a trip into settings.

When switched on, explain: `Appears each month. Enter the amount when you know it.` Reveal one optional checkbox, **Suggest this amount each month**, initially unchecked. Checking it saves the entered amount as the usual suggestion. This suits an allowance or house savings contribution; leaving it unchecked suits a changing credit card bill. A saved historical item shows its existing recurrence and usual-amount preferences when selected.

For an existing item, clicking its name opens the same compact details area used for notes. Put **Repeat monthly** there, followed by **Usual amount**, an optional editable amount shown only when recurrence is on. Keep the visible monthly amount cell dedicated to this month's figure. A quiet `Monthly` label beside the item name makes the setting discoverable without adding another button to every row. The `Recurring expenses` section is only organization; recurrence is an item preference and also works for salary or cards in other sections.

Enabling recurrence saves the current month's item and includes its name in each subsequent month until the user turns it off. New monthly amounts start blank; usual amounts are suggestions that contribute only after the user enters or applies them. Opening the same month again cannot generate duplicate rows. No billing day, payment date, frequency picker, end date, or recurrence management page is needed for this monthly workflow.

Changing this month's amount changes only that cell. Changing the usual amount changes future suggestions and never overwrites recorded amounts. Turning recurrence off stops future automatic rows, preserves earlier months and entered future amounts, and removes only untouched blank rows generated by that recurrence. Removing an item from one month skips that month without stopping recurrence. Use **Stop repeating** in the item's details to end recurrence; do not make a one-month deletion silently change the ongoing preference.

These refinements are documented requirements. The interactive prototype has deliberately not been updated to implement history suggestions or this revised recurrence placement.

## Returning each month

1. Select the month. Reusable item names are already there; new monthly amounts start blank.
2. Enter salary and the card amounts directly in their rows. Tab or Enter moves between amounts.
3. Review the usual recurring amounts shown as suggestions. `Use usual amounts` fills the blank recurring rows together. Editing an individual amount takes precedence and is never overwritten by this action.
4. Add received reimbursements, returned spending money, and one-off costs where needed. The total recalculates as amounts are entered.
5. Leave when finished. Production saves valid edits automatically and shows saving, saved, or failed beside the edited row. There is no separate submit or close-month task.

Carrying a row forward does not assert that money arrived or a payment happened. Copy names automatically, not amounts. A card reappears with a blank amount; a recurring outgoing can show its usual amount as a suggestion. One-off items do not reappear unless the user chooses to reuse them.

The add form and item details use the same **Repeat monthly** preference described above. Notes remain optional. Recurrence, suggestions, and month-specific edits need proper persistence behavior in implementation; the prototype keeps only in-memory examples.

## Adding a card

Under `Credit cards`, choose `Add item`, enter or select `Citi Cash Back+ Card`, enter its bill amount, optionally turn on **Repeat monthly**, and save. The Maybank card follows the same flow. Repeating retains the card name for subsequent months while leaving each bill amount blank. No card number, funding account, statement cycle, minimum payment, evidence, or plan match is requested.

The user confirmed on 2026-10-29 that this is the **bill amount for the month**. Label card inputs `Bill amount`. It is not a payment confirmation. Paying that bill later creates no additional outgoing in this sheet. Do not add a per-card accounting-mode dropdown or a separate payment screen.

For early repayments, the monthly sheet records the bill figure the user chooses to account for, once. Do not automatically add the source's separate early-repayment row to that figure; the source does not establish whether they overlap. Historical imports must resolve that ambiguity explicitly.

## Calculations and awkward cases

- **Subtotals:** sum leaf items once. `Recurring expenses` and `Credit cards` cannot also hold editable amounts when they contain item amounts. For summary-only tracking, use a single item instead of a subtotal plus its components.
- **Blank versus zero:** blank means no amount entered, is visibly blank, and does not count as confirmed zero. An explicit zero is valid. Show incomplete coverage beside the remainder, without blocking editing or requiring a review workflow.
- **Monthly remainder:** sum incoming monthly amounts minus outgoing monthly amounts. Do not call it account balance, net worth, or spending surplus. Do not include an opening bank balance. A negative result is a shortfall in this sheet, not automatically overspending.
- **Savings and investments:** include contributions in money out. Do not label the total as consumption or expenses.
- **Personal spending top-up:** count funding on the outgoing side and returned excess on the incoming side. Do not also require purchases inside that spending account. Do not infer consumption from top-ups minus returns.
- **Reimbursements:** enter received money under money in. Outstanding claims are outside the first version. A claim label in the old workbook does not prove receipt.
- **Refunds:** cash returned can be an incoming item. A credit already netted into a card amount must not also be entered as money in. Notes can explain it without creating another accounting flow.
- **Compound amounts:** the user can enter a single total and optional note. No formula editor or mandatory breakdown is needed.
- **Corrections:** edit the monthly cell directly. Deleting or clearing an amount updates the total and offers Undo. Preserve history internally when implemented; do not ask for a correction reason in routine use.
- **Custom organization:** allow adding or renaming a section through a quiet sheet menu. Items inherit its incoming/outgoing side. No nesting beyond section and item, rules engine, or account mapping.

The reference is evidence for workflow, not a perfect set of import totals. For example, the May inflow subtotal `E35` sums `E25:E27` although a later inflow exists at `E34`. Its recurring totals use child amounts in column L, while the monthly outflow totals use column K. A migration must inspect formulas and leaf amounts instead of blindly copying displayed totals or summing every visible number.

## What leaves the first version

Remove these from the product's normal navigation and required data entry:

- Separate Overview and Records screens.
- Statement cycles, spending components, obligations, settlements, and evidence forms.
- Funding-boundary setup, source/destination account selection, and observed account balances.
- Separate plans, projections, coverage checklists, and month close/reopen workflows.
- Claim lifecycle and receipt matching.
- Draft staging, JSON entry, imports, Gmail, and AI review.
- Recognized-spending analysis and multiple competing surplus figures.

The first version keeps month selection, sections, named monthly amounts, history-based name suggestions, automatic totals, optional notes, monthly recurrence, usual-amount suggestions, and straightforward correction. Settings can retain currency, export, and sign-out. AI can later suggest values in these same rows; it should not require a second user workflow.

## Relationship to existing decisions

[ADR 0002](../adr/0002-monthly-accounting.md) proposes separate recognized spending, obligations, and settlement. This UX removes those from the initial product scope because the user wants a high-level monthly sheet.

[ADR 0003](../adr/0003-funding-boundary-and-reimbursements.md) requires confirmed cash movements across selected funding accounts as the primary view. A sheet of assigned monthly amounts without payment dates or account setup cannot honestly claim to implement that measure. This proposal therefore names its result **Monthly remainder** and defines it only from entered monthly amounts. Card bills contribute in their assigned month regardless of payment timing.

The financial distinctions behind those ADRs still explain double-counting risks. The redesign should retain correct arithmetic while reconsidering which financial questions the product promises to answer. The confirmed card-bill interpretation makes this a monthly accounting summary rather than a confirmed-cash ledger. After the overall flow is accepted, record the superseding product decision and update the product requirements and accounting contract together. Do not silently translate old postings into new monthly cells or delete old financial data.

The existing [visual guide](10-visual-design.md) supplies the light, restrained appearance. Its multiple review metrics and review-task hierarchy need revising for this single-sheet scope.

## Acceptance criteria for the eventual implementation

- Starting from a sheet, adding a card needs one contextual add action, two required fields, and Save.
- Typing an item name offers matching saved history while allowing a new name. Selecting an existing item retains typed amounts and avoids duplicates in the current month.
- Monthly recurrence can be enabled in the add form or changed in item details, without opening settings or creating a schedule.
- Recurrence works independently of section names. Fixed amounts can have a usual suggestion; changing bills can repeat with no suggested amount.
- Skipping one month and stopping recurrence are distinct actions. Both preserve historical amounts and never erase entered amounts in another month.
- Updating an existing monthly amount needs no modal, navigation, account selection, or explicit save button.
- A new month retains reusable names, distinguishes blank from zero, and never silently treats copied suggestions as received or paid money.
- Usual recurring amounts can be entered together without overwriting an edited value.
- Savings, investments, and returned spending money produce the expected monthly remainder without being presented as consumption or salary.
- Section totals and the headline agree with the entered leaf amounts; overlapping card/refund amounts are not counted twice.
- No real account, statement, claim, plan, draft, or close-month configuration is necessary to complete the monthly check-in.
- On a phone, amounts remain editable without sideways scrolling. Save errors preserve the edit, identify the affected row, and offer a safe retry.

## Next implementation boundary

First settle the overall flow. Card amounts already have a confirmed meaning: the bill amount for the month. Then build a clean monthly-sheet path with a minimal data contract. Existing endpoints require dates, financial classifications, and account relationships that this flow deliberately does not request; hiding those fields and inventing values would preserve the wrong model.

Keep existing financial history intact while preparing an explicit mapping for any migration. No backend removal, data deletion, or replacement of the running app is part of this design proposal.
