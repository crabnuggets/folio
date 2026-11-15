# ADR 0007: Remove legacy accounting and add assistant proposals over MCP

Status: Accepted by the user, 2026-11-15. Supersedes ADR 0006's decision to retain legacy endpoints and the lossless combined export. Its retention of the in-app ChatGPT composer is superseded by ADR 0008.

## Context

ADR 0006 kept the original accounts, entries, statements, claims, plans, recurrences, reconciliation periods, and drafts behind legacy endpoints. The user no longer needs them and asked for their removal. Separately, the user wants to fill monthly sheets from statements, screenshots, and plain language using an AI assistant. ChatGPT's hosted plan-usage sign-in needs OpenAI partner approval, and ChatGPT Plus cannot call write tools on custom MCP servers. Claude Pro can.

## Decision

- Remove the legacy accounting routes, domain types, UI, contracts, and fixtures. Export schema 2 contains monthly sheets, monthly proposals, settings, and audit.
- Leave stored legacy fields untouched. Owner documents still load; unknown fields round-trip as opaque JSON. No conversion to monthly items is inferred.
- Add monthly proposals (drafts) to the owner aggregate. An assistant creates them through an authenticated MCP endpoint. Only the owner can accept, edit, or reject them in Folio, and only acceptance changes totals.
- Authenticate MCP with Supabase's OAuth 2.1 server and a dedicated access-token audience, so MCP tokens cannot call the REST API.
- Target Claude custom connectors first. The endpoint is client-neutral.

## Consequences

Legacy data can no longer be viewed or exported through the app. It stays in the database until someone deletes it deliberately. Uploaded files stay with the assistant; Folio keeps only the proposal text, a source reference, and an optional breakdown. Setup requires enabling the Supabase OAuth server, registering a client, and installing a token hook ([MCP setup](../mcp.md)).
