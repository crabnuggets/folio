# Open questions and evidence

## Confirmed from the user

- Personal, single-user monthly finance tracking focused on the bigger picture.
- Browser and Android clients, React preference, Go backend preference, and conventional Android development.
- Third-party services must offer free tiers; development AI should not incur API charges.
- Income, card bills, investments, useful monthly surplus/overspending views, and minimal manual work.
- Optional email/voice capture and grounded allocation/spending suggestions.
- Country: Singapore. Email provider: Gmail.
- First hosted version: Render + Supabase, selected by the user. Finalize unattended processing during Gmail integration; see [ADR 0004](../adr/0004-first-hosted-version.md).
- “Monthly expenses” funds a separate personal spending account; “Personal spending account excess” is unused money returned from it (explicit follow-up confirmation).

SGD and Asia/Singapore are proposed defaults consistent with Singapore; the user did not separately specify currency or cross-border account needs. Take-home cash salary is the initial income basis. Singapore residency alone does not imply CPF coverage, tax status, bank choice, or investment suitability.

## Missing inputs and bounded assumptions

| Input | Why it matters | Current baseline / blocking scope |
| --- | --- | --- |
| Referenced spreadsheet | Existing categories, formulas, month convention, history | Inspected through Drive; see [mapping](08-spreadsheet-mapping.md). Migration parity remains unverified |
| Bank/card statement examples | Format-specific extraction, password handling | Sheet identifies Citi Cash Back+ and Maybank XL Cashback; parser implementation still needs redacted statements |
| Bill month preference | Due month versus statement month or spending month | Proposed due-month convention with explicit override; compare against spreadsheet before migration |
| Existing card debt/credits | Prevent repeated expense recognition | Explicit opening obligations and cycle components |
| Cross-currency accounts | FX policy and currency-specific reporting | SGD-only postings initially; reject/hold foreign currency |
| Desired autonomous posting level | Review effort versus error tolerance | Confirm all imported/spoken drafts initially |
| Gmail app eligibility and consent setup | Token longevity, verification, restricted scopes | On-demand/manual fallback; no assumption of perpetual testing tokens |
| Hardware for local speech/model experiments | Whether local inference is practical | Rules/manual remain fully functional |
| Emergency target, debt rates, investment horizon | Useful personal suggestions | User-configured targets; no invented suitability assumptions |
| “Decisions API” / “Jev” exact references | Product identity, licensing, pricing, capability | Unverified; no implementation dependency |
| Unattended processing | Scheduler/worker choice and idle-period reliability | Deferred to P6 Gmail integration; no mechanism selected |
| Availability expectations | Selected free hosting may sleep/pause | Implement Render cold-start and Supabase pause recovery; no always-on promise |

## Spreadsheet mapping procedure

Initial structure and formulas are recorded in [the source mapping](08-spreadsheet-mapping.md). For implementation, reread current workbook/sheet names, relevant headers, formulas versus entered values, currencies, month attribution, opening balances, carryovers, and category meaning. Produce a table: source sheet/column → Folio entity/field → transform → unresolved ambiguity. Preserve a source row ID in migration data. Preview changes and duplicate candidates, compare all four required months—April (joint adjustment), May (refund exclusion), September (claim exclusion), and October (incomplete month)—then export a reconciliation report. Never infer a historical transfer is an expense merely because it is negative in the workbook.

## Reference register

The links below are official documentation starting points for implementing agents. Subsequent hosting research succeeded: see [the dated comparison](09-hosting-recommendation.md) for sources now verified; the rest of this register retains its original unverified status. Live search returned tool errors during this session, so these URLs and current provider terms were not independently fetched/verified. Do not quote specific quotas or future eligibility from this plan. Check current documentation and record retrieval date, applicable region/plan, and any relevant limits before deployment.

| Area | Official starting point | Verify |
| --- | --- | --- |
| Android | https://developer.android.com/compose | Current Compose and native architecture guidance |
| React | https://react.dev/ | Current setup and supported tooling |
| Go | https://go.dev/doc/ | Supported releases and HTTP/toolchain guidance |
| Render | https://render.com/docs/free | Free web-service limitations, sleep, bandwidth, ephemeral storage, database trial lifetime |
| Supabase | https://supabase.com/pricing | Free quotas, pausing, region, backups and billing behavior |
| Supabase Auth | https://supabase.com/docs/guides/auth | JWT verification, Google OAuth, redirects, PKCE/session behavior |
| Gmail scopes | https://developers.google.com/workspace/gmail/api/auth/scopes | Read-only scope classification and restricted-scope obligations |
| Google OAuth | https://developers.google.com/identity/protocols/oauth2 | Testing token expiration, refresh, consent and verification |
| Gmail sync | https://developers.google.com/workspace/gmail/api/guides/sync | History expiry, sync recovery and pagination |
| Gmail push | https://developers.google.com/workspace/gmail/api/guides/push | Watch renewal and infrastructure requirements |
| OpenAI | https://platform.openai.com/docs/api-reference/responses | Exact supported API if an optional adapter is later selected |
| OpenAI pricing | https://openai.com/api/pricing/ | Paid usage; no assumed permanent free allowance |
| Speech on Android | https://developer.android.com/reference/android/speech/SpeechRecognizer | On-device availability and recognition service behavior |

## Decision log maintenance

Update ADRs when changing financial month attribution, authentication, platform selection, or hosted deployment. Mark assumptions resolved only with user input or evidence. Keep costs and feature status explicit: proposed, verified, implemented, or deferred. Never equate a documented plan with a working integration.
