# Automation and AI

## Design principle

Rules collect and validate facts; models optionally interpret ambiguous input and explain already-computed figures. Go owns arithmetic and posting invariants. A model never initiates transfers, trades, payments, deletes email, or changes confirmed records directly.

The user mentioned “OpenAI Decisions API” and “Jev.” These names do not identify verified dependencies in this session. Do not invent SDKs or assume they are free. Ask for exact product links before evaluating them. OpenAI's Responses API is a possible future paid adapter, not an assumed correction of the user's intended product and not part of the zero-cost baseline.

## Gmail ingestion

Gmail is confirmed as the initial email provider. The application must have its own Google Cloud OAuth configuration; the tools connected to this chat do not grant the future application mailbox access.

1. User explicitly connects Gmail using OAuth, granting the narrowest scope that permits reading statement bodies/attachments. `gmail.readonly` is the likely scope; verify current classifications and requirements. A label/query filter limits application behavior but does not narrow the OAuth token's technical mailbox access.
2. Configure supported sender patterns and an optional user-created “Folio Statements” label. Fetch a bounded window, initially 90 days, with preview before importing older history. No mailbox-wide LLM sweep.
3. Persist message/attachment IDs and content hashes. Use history cursors where supported, plus bounded overlap scans; handle expired cursors with a deduplicated rescan. Paginate and checkpoint so restarts do not lose messages.
4. Check source type, size, sender authentication metadata where available, and bank template version. Sender display names are not proof of authenticity.
5. Extract statement facts with deterministic templates first. A notification saying “your statement is ready” without amounts does not establish the bill amount. Do not open bank login links automatically.
6. Create drafts with field-level evidence. Match to card aliases, cycle dates, existing statements, and related payment observations. Ambiguous matches require review.
7. Confirming a draft creates one posting atomically. Retrying a sync or importing the same PDF manually cannot add the same statement twice.

Render + Supabase is the accepted first-version hosting stack ([ADR 0004](../adr/0004-first-hosted-version.md)); the unattended-processing mechanism will be finalized during Gmail integration (P6), not during initial hosting setup. No scheduler has been selected. Use on-demand sync in the first Gmail release, with a visible last-success timestamp and actionable connection failures. Later scheduled scans use an external authenticated trigger or durable worker; a timer inside a sleeping web process is not reliable. Gmail push requires additional infrastructure, watch renewal, and recovery scans; defer until polling is insufficient. No need for near-real-time financial updates in this product.

Google OAuth testing-mode refresh tokens can expire quickly (commonly seven days for relevant scopes); verify current rules for the actual app configuration. Sensitive/restricted Gmail scopes may require consent-screen verification and, depending on deployment/data handling, a security assessment. Personal-use exceptions must be verified, not assumed. If compliant unattended hosting cannot stay within the cost constraint, keep on-demand/manual file imports and document reconnect requirements rather than promising hands-free operation.

## Statement parsing

Start with manual summary entry and CSV, then one explicitly identified bank/template with redacted samples. The workbook identifies Citi Cash Back+ and Maybank XL Cashback cards. Prioritize one of these after obtaining a redacted statement; names alone do not establish parser layouts.

- Validate MIME type and content, set configurable size/page/time limits, isolate PDF/OCR processing, and disable document scripts/external resource fetches.
- Distinguish statement total, minimum payment, installment balance, carried debt, new spending, and due date. Preserve unknowns.
- Password-protected statements require user-assisted unlocking initially. Do not guess password schemes, store national identity information, or retain passwords. Clear temporary plaintext files after processing.
- Unsupported layout, scanned image, multiple currencies, missing pages, or inconsistent arithmetic → review/manual entry. OCR is a later local option with accuracy tests, not a guaranteed free hosted service.
- Optional category breakdowns must sum to known statement spending. Retain an “unclassified” remainder. Breakdown rows annotate the statement; they are not additional expenses.
- For manual PDFs, use `provider=manual_pdf` and the SHA-256 hash of the original uploaded bytes as both the stable `external_id` and content hash. Source identity is `(owner_id, provider, external_id)`, independent of filename, upload time, or parser version; enforce it with a database uniqueness constraint before acceptance. An unchanged re-upload reuses the source and pending draft, or links to its existing statement after acceptance, without another posting.
- Duplicates across email, PDF, and CSV are matched by source and domain identity. Same card/cycle with different totals is a revision conflict, never a silent replacement.

## Draft contract and lifecycle

States: received → parsing → needs_review → accepted/rejected; parsing failures follow the durable job's bounded retry policy. When retries are exhausted and the parsing job enters dead-letter state, move its draft to needs_review with a redacted failure and actions to retry parsing or complete fields manually. An explicit retry returns the draft to parsing with a new bounded retry budget. A duplicate can link to an existing record and finish without posting. Only validated drafts enter acceptance.

A candidate contains event kind, amount/currency, local date, reporting month, account/card reference, statement components where applicable, per-field evidence locations, parser/model version, validation errors, and duplicate candidates. Treat model confidence as advisory; it is not calibrated evidence of financial correctness.

Initial policy: every imported or spoken financial record requires one confirmation. Later, opt-in automatic acceptance may be limited to a deterministic template with proven fixtures, exact required fields, no duplicates, open month, explicit amount bounds, and an audit trail with reversal. New templates, changed bank formats, unknown spending, and all ambiguous speech always require review. This allows gradual reduction of work without making unverifiable automation the first release.

## Voice

Browser and Android offer a microphone action alongside typed capture. Prefer Android on-device speech recognition when supported, with runtime capability checks. Browser speech recognition varies by browser and may send audio to a vendor; do not promise universal offline support. Display the processing destination before enabling it. A local Whisper-compatible transcription service is an optional development experiment on the user's computer; CPU, storage, and availability costs remain even without API fees.

Example: “My salary of five thousand came in today.” Transcript → draft income of SGD 5,000 → account/date confirmation → posting. “Move eight hundred to investments” proposes an allocation record; it does not move money. “I will get paid tomorrow” creates an expected plan item, not received income. Missing currency can default to the configured base currency; uncertain amount, date, account, or intent must be highlighted. Do not interpret “fifteen” and “fifty” by guesswork. Clear audio after transcription by default; allow transcript edits before parsing.

## Model boundary and cost

Define interchangeable `ExtractDraft(text, schema)` and `ExplainInsights(facts)` interfaces, with timeout, cancellation, strict schema validation, input limits, model/version metadata, and redaction. Baseline implementations use rules and templates. Tests use deterministic fakes; no API credential required in CI.

Optional local inference can run through a local model runtime on the user's machine; evaluate model license, memory, latency, structured-output reliability, and extraction accuracy before selecting a model. Do not run a large model on a tiny free web instance or expose a local inference server publicly. A hosted Go API cannot reach localhost on the user's laptop without an explicit secure connectivity design; therefore local model experimentation initially runs with the local Go development stack.

Paid adapters remain disabled. Never fall back to a paid service automatically. If later enabled, use explicit provider selection, server-side keys, hard request/token budgets, timeouts, and a zero-budget kill switch. A ChatGPT subscription does not establish free API access. Free promotional credits are not an architecture dependency.

Treat all email, PDF, and transcript text as untrusted data. Model prompts cannot grant tools or override owner authorization. The extraction model receives no mailbox-send, shell, database-write, or bank capabilities. Validate all output against known accounts and currencies. Minimize input to required fields/snippets; do not send entire mailboxes or PDFs to hosted inference by default.

## Insights

Compute facts and recommendations deterministically first:

| Trigger | Grounded response |
| --- | --- |
| Projected surplus < 0 | Show shortfall and largest known spending/optional allocation lines |
| Statement total unusually above comparable prior months | Show magnitude and comparison period; require at least three comparable complete months |
| Positive projected surplus and emergency-fund target gap | Suggest a configurable amount capped by positive surplus and known target gap |
| Emergency-fund target met and remaining surplus | Suggest considering a user-defined investment allocation after near-term obligations |
| Missing statement or stale cash snapshots | Explain why recommendations are provisional or unavailable |

Separate salary from reimbursements and transfers; distinguish gross card charges from known personal cost before flagging overspending. Pending claims are not spendable cash, and reimbursement delays can create a cash timing gap even when net personal cost is low. Separate essential expenses from user-designated discretionary expenses. Without merchant/category detail, say “Card A is your largest tracked expense” rather than inventing dining or transport cuts. Allow optional broad category summaries if the user wants more actionable suggestions. Do not recommend specific securities or assume risk tolerance, investment horizon, CPF balances, or debt interest rates. CPF contributions are not spendable take-home income; cash salary should be the net received amount. CPF tracking itself is deferred unless requested.

Generated wording may only reference supplied fact IDs and amounts. Reject unsupported figures, fabricated categories, or suggestions above the computed affordable allocation. Show fact citations and coverage alongside the insight. Cache by month revision and model/prompt version; corrections invalidate stale insights. Rules-only output remains available when models fail.
