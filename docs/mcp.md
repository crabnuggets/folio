# Folio MCP

Folio exposes four tools over the official Go MCP SDK's stateless Streamable HTTP
transport at `https://folio-j4md.onrender.com/mcp`:

- `read_monthly_sheet` reads a month's items (with IDs, amounts and explanations),
  section IDs, current version, and totals.
- `list_pending_monthly_drafts` reads pending proposals for one month.
- `create_monthly_item_draft` persists one pending proposal for a blank or new
  item, optionally with a breakdown (explanations) for a money-out item.
- `propose_monthly_item_amendment` persists one pending proposal to change an item
  that already has an amount: a corrected amount, or explanations that add detail.

The intended client is Claude (claude.ai, desktop or mobile) with Folio added as a
custom connector. You upload a statement, screenshot, or describe amounts in chat;
Claude reads the file and calls the tools. Folio receives only the structured
proposal, never the uploaded file.

Every MCP request, including initialization and tool discovery, requires an owner
access token. MCP cannot accept, reject, edit, or post amounts. The signed-in Folio
UI shows Pending proposals with review, edit, accept, reject, and refresh actions.
Accepting a proposal fills a matching blank item or adds a monthly item, and copies
its breakdown to the item's explanations. It refuses to overwrite an entered amount;
only an amendment, accepted explicitly, changes one. Recurrence preferences stay intact. Pending and rejected proposals never contribute
to totals.

An amendment records the item's entered amount and explanations when it is
proposed. Accepting it replaces both and keeps the item's name, section, note and
recurrence. Acceptance is refused with `ITEM_CHANGED` if either has changed since.

Source namespace plus external identity deduplicates proposals across reconnects,
retries, edits, and review decisions. The match compares what a proposal would
change (month, item, amount and explanations), not the assistant's note or evidence
wording, so reading the same document again finds the earlier proposal. The tools
then return `outcome: already_proposed` with that proposal and its state (pending,
accepted or rejected). The same identity with a different item, amount or
explanations fails, and the error names the existing proposal so the assistant
can report it. Proposals staged before this matching existed conflict on any
re-proposal under their identity, with the same informative error.
Creation checks the sheet version; editing and rejection check the proposal
version. Acceptance checks both versions inside the owner transaction. State,
audit, and HTTP retry responses commit together. Replaying an acceptance cannot
add it twice, including after Undo. Undo reverses the monthly change, breakdown
included, while leaving the source marked as reviewed.

## Hosted authorization setup

Claude authorizes against Folio through Supabase's OAuth 2.1 server, using your
existing Folio sign-in.

These are external account changes. They have not been applied by this
implementation. The project's OAuth discovery returned 404 during investigation
(the OAuth server is not enabled yet); a successful round trip remains to be
verified after setup.

1. **Enable the OAuth server.** In Supabase project `jnmomouljwexebylcxei`, open
   Authentication > OAuth Server and enable it. Set the Authorization Path to
   `/oauth/consent`. Leave dynamic client registration disabled. In
   Authentication > URL Configuration set Site URL to
   `https://folio-j4md.onrender.com` and keep the existing callback
   `https://folio-j4md.onrender.com/auth/callback` allowlisted.
2. **Check discovery.** Open
   `https://jnmomouljwexebylcxei.supabase.co/.well-known/oauth-authorization-server/auth/v1`.
   It must return JSON advertising the authorization-code and refresh-token grants,
   PKCE `S256`, and issuer `https://jnmomouljwexebylcxei.supabase.co/auth/v1`. The
   project must sign tokens with asymmetric ES256 or RS256 keys, which Folio's JWT
   verification requires.
3. **Register Claude as a client.** Under Authentication > OAuth Apps, create a
   confidential client named `Claude` with these redirect URIs:

   ```text
   https://claude.ai/api/mcp/auth_callback
   https://claude.com/api/mcp/auth_callback
   ```

   Anthropic documents the first and says it may move to the second, so register
   both. Set the token endpoint authentication method to `client_secret_post`,
   because Claude sends the secret in the request body; Supabase rejects it under
   the default `client_secret_basic`. Keep the client ID and secret for steps 4, 5
   and 7. Also set Authentication > URL Configuration > Site URL to exactly
   `https://folio-j4md.onrender.com`. The consent page fails with "unauthorized
   request origin" if the scheme or host differs.
4. **Install the audience hook.** Supabase's default access-token audience
   (`authenticated`) is shared with the web app. Folio requires MCP tokens to carry
   the dedicated audience `https://folio-j4md.onrender.com/mcp` and refuses them on
   every `/api/v1/` route. Run [the token hook](mcp-token-hook.sql) in the SQL
   editor after replacing `REPLACE_WITH_MCP_CLIENT_ID` with the client ID from
   step 3. Then enable it under Authentication > Hooks > Custom Access Token. If
   the project already has a hook, merge this branch into it. The hook changes
   only this client's audience, on initial issue and on refresh.
5. **Configure Render.** Add these environment variables and deploy this
   release. No database migration is needed:

   ```text
   MCP_PUBLIC_ORIGIN=https://folio-j4md.onrender.com
   MCP_CLIENT_ID=<client ID from step 3>
   ```

   Keep `AUTH_MODE=jwt`, `JWT_AUDIENCE=authenticated`,
   `JWT_ISSUER=https://jnmomouljwexebylcxei.supabase.co/auth/v1`,
   `SUPABASE_PUBLISHABLE_KEY`, `OWNER_ID`, and the database settings as they are.
   The client secret does not go in Render. With both MCP variables absent the
   app runs with MCP disabled; partial or invalid values stop startup.
6. **Check the endpoint.** Open
   `https://folio-j4md.onrender.com/.well-known/oauth-protected-resource/mcp`. It
   should name Supabase's issuer as the authorization server. A request to `/mcp`
   without a token must return 401 with a `WWW-Authenticate` header pointing to
   that metadata.
7. **Add the connector in Claude.** In Claude, open Settings > Connectors > Add
   custom connector. Name it `Folio`, enter `https://folio-j4md.onrender.com/mcp`,
   open Advanced settings, and paste the client ID and secret from step 3. Select
   Connect, sign in to Folio, then choose "Allow monthly sheets and proposals" on
   the consent page. If Claude reports that authorization failed, check the
   Supabase auth logs for `/oauth/token`: an "invalid authentication method" error
   means the client is not `client_secret_post`, and `invalid_client` usually means
   stray spaces in the pasted values.
8. **Create a Claude Project** named Folio with Folio enabled and instructions
   such as:

   ```text
   Folio tracks one amount per item per month, not individual transactions.
   Before proposing anything from a document, read the month in Folio and list its
   pending proposals. If an entered item or a pending proposal already covers the
   document, tell me what is there and ask before proposing again.
   Reuse existing item names and sections.
   For a credit card statement, propose one draft for the statement total in that
   card's bill item and list the charges as explanations. Explanations must not
   add up to more than the total.
   If the item already has an amount, propose an amendment instead: a corrected
   amount, or the charges as explanations.
   Build source_id from the issuer and document type, and external_id from details
   printed in the document, such as the card's last 4 digits and the statement
   date. Never use the file name.
   If Folio says a proposal already exists, tell me instead of creating another.
   Only create drafts. Never say anything has been saved or paid.
   ```

9. **Test.** Upload a statement or describe one amount, let Claude propose it, and
   confirm the Folio totals don't change until you accept it under Pending
   proposals. Use Undo to remove a test item. To revoke access, disconnect the
   connector in Claude and revoke the grant in Supabase.

The API checks signed JWTs locally. Revoking the grant in Supabase stops refresh,
but an access token already issued stays valid until it expires, so keep
Supabase's access-token lifetime short. Render's free instance sleeps when idle;
the first request after that can take close to a minute and may time out. Retry
once the app has woken.

ChatGPT can use the same endpoint by registering its own client and callback, but
write tools in custom connectors are limited to Business, Enterprise and Edu plans,
so it cannot create drafts on Plus or Pro.

## In-app composer removal

The in-app ChatGPT composer, its connection and capture routes, and the
`CHATGPT_*` settings are removed; see [ADR 0008](adr/0008-claude-only-assistant-and-amendments.md).
Remove `CHATGPT_REDIRECT_URI` and `CHATGPT_ENCRYPTION_KEY` from any environment
that sets them. A saved ChatGPT connection stays in the owner document as an
opaque retired field and is never read.

## Legacy removal and storage compatibility

The owner's removal request supersedes ADR 0006's retained legacy endpoints and
combined export; see [ADR 0007](adr/0007-remove-legacy-accounting.md). Legacy
accounting routes, domain types, UI, contracts, and fixtures are removed. Export
schema 2 contains monthly sheets, monthly proposals, settings, and audit. It omits
private provider state and retired aggregate fields.

Existing PostgreSQL owner documents still decode with monthly sheets and
recurrence intact. Unknown aggregate fields remain opaque JSON and round-trip
through later writes. The implementation neither converts legacy postings nor
deletes stored legacy fields. Historical ADRs are kept as history.

## Primary references

- [Claude: custom connectors using remote MCP](https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp)
- [Claude: building custom connectors](https://support.anthropic.com/en/articles/11503834-building-custom-connectors-via-remote-mcp-servers)
- [Supabase OAuth setup and consent](https://supabase.com/docs/guides/auth/oauth-server/getting-started)
- [Supabase MCP authentication](https://supabase.com/docs/guides/auth/oauth-server/mcp-authentication)
- [Supabase token audience hooks](https://supabase.com/docs/guides/auth/oauth-server/token-security)
- [Official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)
