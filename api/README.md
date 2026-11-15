# Folio API

The [monthly sheet contract](monthly-sheet-contract.md) and generated OpenAPI
are shared by React and the Go API. The API owns money arithmetic, recurrence,
duplicate protection, owner authorization, version checking, and Undo.

Money is a canonical signed 64-bit minor-unit integer string. Monthly entered
amounts and proposal amounts are nonnegative. Months use YYYY-MM. API responses
are private and uncached. Bearer tokens identify the configured owner; identities
are never accepted from request bodies or arbitrary owner headers.

Versioned REST writes require `If-Match: "N"` and a UUID `Idempotency-Key`.
Repeating an exact committed request replays its original status, body, and ETag.
A changed request with the same key returns 409. Failed writes do not consume
keys. All financial state, audit, and retry responses commit atomically.
List endpoints support an exclusive UUID cursor and limits from 1 to 100.

Monthly proposals have their own persisted lifecycle under `/monthly-drafts`.
GET lists pending proposals, optionally by month. PUT `/{id}` edits a proposal;
POST `/{id}/reject` rejects it; POST `/{id}/accept` requires its version and a
`sheet_version` body field. Only acceptance changes monthly amounts. The public
MCP transport can read monthly context and stage proposals, never accept them.
See [MCP setup](../docs/mcp.md) for tools, OAuth, and hosted configuration.

Export schema 2 contains monthly sheets, proposal history, settings, and audit.
Removed accounting endpoints and their contracts no longer exist. Stored retired
fields remain opaque in owner documents and are not converted or deleted.

A proposal with a non-null `amendment` changes an item that already has an
amount. It records the entered amount and explanations it was made against;
acceptance replaces them, keeps the item's name, section, note and recurrence, and
returns 409 `ITEM_CHANGED` if they no longer match. The in-app chat capture
and ChatGPT connection routes have been removed.

Run `scripts/generate_openapi.py` to regenerate `openapi.yaml`, then
`npm run generate:api` in `apps/web`. `scripts/validate_contract.py` checks
OpenAPI validity and generator drift; `npm run check:api` checks generated types.
