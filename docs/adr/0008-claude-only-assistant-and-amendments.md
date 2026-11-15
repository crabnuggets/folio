# ADR 0008: Make Claude over MCP the only assistant path, and let proposals amend entered items

Status: Accepted by the user, 2026-11-15. Extends ADR 0007.

## Context

ADR 0007 added assistant proposals over MCP but kept the in-app ChatGPT composer. That composer depended on a plan-usage sign-in that needs OpenAI partner approval, and it added items directly after an in-app review instead of staging proposals. No public interface lets the app start a conversation in a Claude project, so the composer could not be pointed at Claude either.

Two gaps showed up in the MCP flow. Re-uploading a document produced a source conflict or a duplicate proposal, because matching compared the assistant's note and evidence wording and the suggested identity used the file name. An assistant could also see that a document added detail to an entered item, such as the charges behind a card bill, but could not propose that change, because acceptance refuses to overwrite an entered amount.

## Decision

- Remove the in-app composer, the ChatGPT connection and capture routes, their configuration, and their contracts. Stored ChatGPT credentials stay in owner documents as opaque retired fields, following ADR 0007.
- Match a source identity on what the proposal would change: month, item, amount and explanations. Report a match to the assistant as `already_proposed` with the proposal's state. A conflicting reuse names the existing proposal.
- Tell the assistant to read the month and its pending proposals before proposing, to ask the owner when a document already appears covered, and to build identities from details printed in the document.
- Add amendment proposals for items that already have an amount. An amendment records the entered amount and explanations it was made against. Accepting it replaces both and keeps the item's name, section, note and recurrence; it is refused if that amount or those explanations have changed since.

## Consequences

Claude, connected as a custom connector, is the only way to fill a sheet with an assistant. Uploads happen in Claude, not in Folio. Duplicate detection still depends on the assistant choosing a stable identity, and on it checking the sheet first; neither is enforced by the server. Proposals staged before the new matching conflict on any re-proposal under their identity.
