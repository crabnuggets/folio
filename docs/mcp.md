# Folio MCP

Folio exposes three tools over the official Go MCP SDK's stateless Streamable HTTP
transport at `https://folio-j4md.onrender.com/mcp`:

- `read_monthly_sheet` reads a month, section IDs, current version, and totals.
- `create_monthly_item_draft` persists one pending proposal, optionally with a
  breakdown (explanations) for a money-out item.
- `list_pending_monthly_drafts` reads pending proposals for one month.

The intended client is Claude (claude.ai, desktop or mobile) with Folio added as a
custom connector. You upload a statement, screenshot, or describe amounts in chat;
Claude reads the file and calls the tools. Folio receives only the structured
proposal, never the uploaded file.

Every MCP request, including initialization and tool discovery, requires an owner
access token. MCP cannot accept, reject, edit, or post amounts. The signed-in Folio
UI shows Pending proposals with review, edit, accept, reject, and refresh actions.
Accepting a proposal fills a matching blank item or adds a monthly item, and copies
its breakdown to the item's explanations. It refuses to overwrite an entered amount.
Recurrence preferences stay intact. Pending and rejected proposals never contribute
to totals.

Source namespace plus external identity deduplicates proposals across reconnects,
retries, edits, and review decisions. Changed source content returns a conflict.
Creation checks the sheet version; editing and rejection check the proposal
version. Acceptance checks both versions inside the owner transaction. State,
audit, and HTTP retry responses commit together. Replaying an acceptance cannot
add it twice, including after Undo. Undo reverses the monthly change, breakdown
included, while leaving the source marked as reviewed.

## Hosted authorization setup

Claude authorizes against Folio through Supabase's OAuth 2.1 server, using your
existing Folio sign-in. It is separate from the in-app "Continue with ChatGPT"
connection, which is unchanged.

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
   both. Keep the client ID and secret for steps 4, 5 and 7.
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
   the consent page. If the token exchange fails with `invalid_client`, check for
   stray spaces in the pasted values, then confirm the client's token endpoint
   authentication method in Supabase accepts a client secret.
8. **Create a Claude Project** named Folio with Folio enabled and instructions
   such as:

   ```text
   Folio tracks one amount per item per month, not individual transactions.
   Always read the month in Folio first and reuse existing item names and sections.
   For a credit card statement, propose one draft for the statement total in that
   card's bill item and list the charges as explanations. Explanations must not
   add up to more than the total.
   Use the statement's file name and date as source_id and external_id.
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
