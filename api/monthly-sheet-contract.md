# Monthly sheet API

Implementation contract, agreed backend/frontend boundary. All paths below have
the `/api/v1` prefix. Existing financial endpoints and export fields stay intact.

## Commands and concurrency

Every POST/PUT needs the existing UUID `Idempotency-Key` and JSON content type.
Except `open`, commands need `If-Match: "<version>"` from the latest sheet.
The version covers the owner's complete monthly-sheet model because recurrence,
sections and Undo can change more than one month. Clients serialize commands and
retain immutable request bodies and retry keys, with saving/errors shown per row.
Settings keep their existing independent version. Stale commands return 412.
Frontend agreed to queue writes globally, construct immutable commands at the
queue head with the newest version, and keep row-local failure/retry status.
All successful commands return status 200 and `MonthlySheet`, with its ETag.

| Method | Path | Request schema |
| --- | --- | --- |
| GET | `/monthly-sheets/{month}` | none, persisted rows only |
| POST | `/monthly-sheets/{month}/open` | `EmptyInput`, `{}` |
| POST | `/monthly-sheets/{month}/items` | `MonthlyItemInput` |
| PUT | `/monthly-sheets/{month}/items/{id}` | `MonthlyItemUpdate` |
| POST | `/monthly-sheets/{month}/items/{id}/remove` | `EmptyInput` |
| POST | `/monthly-sheets/{month}/items/{id}/restore` | `EmptyInput` |
| POST | `/monthly-sheets/{month}/usual-amounts` | `MonthlyUsualInput` |
| POST | `/monthly-sheets/{month}/undo` | `MonthlyUndoInput` |
| POST | `/monthly-sheets/{month}/sections` | `MonthlySectionInput` |
| PUT | `/monthly-sheets/{month}/sections/{id}` | `MonthlySectionRename` |
| GET | `/monthly-item-suggestions?month=YYYY-MM&section_id=UUID&q=text` | none |

`open` persists missing recurring rows for exactly the requested month and seeds
empty suggested sections. Reopening cannot duplicate rows or change amounts.
GET does not materialize rows. New clients open each selected month before editing.

## Exact JSON shapes

Money uses the existing canonical signed-int64 decimal minor-unit string on the
wire. Entered monthly and usual amounts must be nonnegative. `null` means blank;
`"0"` is an entered zero. Settings supply currency and exponent.

`MonthlyItemInput` requires `section_id`, `name`, `amount_minor`. Optional fields
are `item_id` default null, `note` default empty, `repeat_monthly` default false,
and `usual_amount_minor` default null. `item_id` reuses a history identity. When
reusing an item, recurrence and usual preferences are preserved when omitted.
To clear an existing usual suggestion on add, supply repeat_monthly explicitly
alongside usual_amount_minor null. A matching normalized typed name also reuses its
identity within the same direction. Existing active current-month rows are returned
unchanged, including their amount and section. Focus the returned matching row.
Reusing a removed monthly row preserves its item identity and applies the newly
submitted amount, note, section and preferences. Its previous saved fields remain
in history and Undo. The explicit restore endpoint instead recovers saved fields.

```json
{"section_id":"UUID","name":"Card name","amount_minor":"12345","item_id":null,"note":"","repeat_monthly":false,"usual_amount_minor":null}
```

`MonthlyItemUpdate` requires the full editable state below. `{id}` is the stable
item ID, also the row ID. Direction cannot change. Rename changes the item's
display name across months and retains old names for history lookup. Amount,
note and section apply only to the selected month. Repeat/usual changes take
effect from the selected month. A regular amount edit resubmits existing prefs.

```json
{"section_id":"UUID","name":"Card name","amount_minor":null,"note":"optional note","repeat_monthly":true,"usual_amount_minor":null}
```

`MonthlyUsualInput` is `{"section_id":null}` for all sections or a section UUID.
It fills only blank active recurring rows with their usual suggestion.
`MonthlyUndoInput` is `{"undo_token":"UUID"}`. Undo restores the last command's
monthly-sheet state only if no later monthly command changed that state; otherwise
412. Its response advances version and has null undo_token. Audit history remains.
Clear is an ordinary update with amount_minor null. Remove skips this item in
this month without disabling recurrence; restore retains its last entered fields.
Remove/restore and clear can be undone with the returned token.

`MonthlySectionInput` is `{"name":"Custom section","direction":"out"}`.
Direction is `in` or `out`. `MonthlySectionRename` is `{"name":"New heading"}`.
Custom sections have amount_label `Amount`. Renaming retains direction and label.

`MonthlySheet` has exactly these fields, all required:

```json
{
  "month":"2026-10","version":1,"currency":"SGD","currency_exponent":2,
  "has_legacy_data":false,"undo_token":null,
  "money_in_minor":"0","money_out_minor":"0","remainder_minor":"0","missing_count":0,
  "sections":[{
    "id":"UUID","name":"Credit cards","direction":"out","amount_label":"Bill amount",
    "total_minor":"0","missing_count":1,
    "items":[{"id":"UUID","name":"Card name","section_id":"UUID","amount_minor":null,
      "note":"","repeat_monthly":true,"usual_amount_minor":null}]
  }]
}
```

Schemas are `MonthlySheet`, `MonthlySection`, `MonthlyItem`,
`MonthlyItemSuggestion`, `MonthlyItemSuggestions` plus the inputs named above.
Suggestions return `{"items":[...]}`. Each suggestion has exactly `id`, `name`,
`section_id`, `section_name`, `direction`, `repeat_monthly`, `usual_amount_minor`,
`current_month_item_id` which is the ID or null. No amount is copied from history.
Matches ignore case and repeated whitespace, include prior names, deduplicate by
normalized current name and direction, rank current section before same direction,
then frequency and recency, and return at most 10. Opposite directions never match.

## Recurrence and defaults

New items never repeat unless opted in, independent of section. Effective-dated
preferences prevent rows before the start month. Opening later months creates
blank rows in the effective preferred section. Editing a month's section updates
that month's placement and recurrence's future placement without moving older rows.
Usual amounts are suggestions only; changing them never overwrites entered cells.
Stopping removes only future untouched blank generated rows. User-entered future
amounts and notes survive. Removing one month creates a persistent skip. Resuming
starts a new effective period without backfilling stopped months.

Suggested section UUIDs and order are stable:

| ID | Name | Direction | Amount label |
| --- | --- | --- | --- |
| `00000000-0000-4000-8000-000000000001` | Income | in | Amount |
| `00000000-0000-4000-8000-000000000002` | Other money in | in | Amount |
| `00000000-0000-4000-8000-000000000003` | Recurring expenses | out | Amount |
| `00000000-0000-4000-8000-000000000004` | Credit cards | out | Bill amount |
| `00000000-0000-4000-8000-000000000005` | Other money out | out | Amount |

All totals sum active leaf amounts once in Go. Blank rows add to missing_count;
zero does not. A card amount is its bill amount for this month. No payment posting
or old summary contributes. `has_legacy_data` indicates retained old financial data.
`GET /exports` retains schema_version 1 and adds `state.monthly_sheet` containing
sections, item identities and aliases, effective preferences, all monthly rows,
skips, version and latest Undo state. Currency locks once any monthly or usual
amount exists, including zero or a removed amount. No migration is inferred.

## Validation evidence

The backend adds domain cases for totals, blank/zero, direction isolation,
normalized identity reuse, historical aliases, suggestion ranking, rename
collisions, effective recurrence start/stop/resume, skips, future note/amount
preservation, usual changes, overflow and Undo after JSON serialization.

Real httptest requests validate successful sheet/suggestion/export payloads
against generated OpenAPI and exercise retry replay, stale/missing preconditions,
malformed inputs, overflow rollback, sections, removal/restore and currency lock.
Memory store tests exercise detached JSON state, Undo persistence, failed-command
rollback and loading legacy aggregates without a monthly_sheet field.

`go test ./...`, `go vet ./...`, the OpenAPI generator check and
`scripts/validate_contract.py` pass. Generated TypeScript schemas are current.
`TestPostgresMonthlySheetRestart` runs the same monthly store checks and verifies
retry bytes and Undo through a new connection pool when TEST_DATABASE_URL is set.
It is explicitly skipped in this environment because no PostgreSQL URL is set;
the orchestrator's local Docker attempt hung. No restart claim rests on Memory.
