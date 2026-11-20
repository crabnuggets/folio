# ADR 0009: Let Claude propose monthly recurring items over MCP

Status: Accepted by the user, 2026-11-20. Extends ADR 0007 and ADR 0008.

## Context

Over MCP, Claude could fill amounts but could not see which items recur into a month nobody had opened, add a monthly recurring item, or change one. Recurrence changes do not move totals, so letting the assistant write them directly was a real option. Chat requests have no document to build a source identity from.

## Decision

- Adding or changing a monthly recurring item over MCP creates a pending proposal. Only the owner's acceptance in Folio changes the item, as with amounts. We chose one trust rule over the convenience of direct writes.
- `create_monthly_item_draft` can also make the new item recur, with an optional usual amount. With recurrence requested, its amount may be blank.
- A **recurring item change** is a separate proposal kind. It carries the item's complete recurrence settings (repeats, usual amount, section) from a named month onward, records the settings it was made against, and is refused with `ITEM_CHANGED` if they have changed since. The owner can accept or reject it, not edit it. An amount change and a recurrence change for the same item are two proposals.
- A recurring item change has no `source_id` or `external_id`. Folio allows one pending recurring item change per item: the same change returns `already_proposed`; a different one is refused and names the pending proposal. Source identities stay on amount proposals, where a document supplies them. Reusing them here would block a corrected request after a rejection.
- `read_monthly_sheet` includes items that recur into the requested month even when the month has not been opened, as blank items, without writing anything. Claude sees what the owner would see on opening the month, so it reuses names instead of inventing duplicates.

## Consequences

The MCP read can list items the stored month does not yet hold. Opening the month in Folio produces the same blank items. Duplicate rules differ by proposal kind. The review UI gains a before-and-after view for recurrence settings.
