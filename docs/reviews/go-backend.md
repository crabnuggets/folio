# Go backend review record

The requested workflow is three independent GPT-6.1 Sol review/fix cycles. Cycle 1
covers the entire backend; later cycles cover only changes introduced afterward.
All fixtures and repros use synthetic values.

## Cycle 1 — comprehensive

Reviewed implementation commit `e60f22586a04be65322d24e97faf9ec1e88ed0fe` against
`cf52c58283c8a2fe49b3c79140b10593035aeb0c`. The reviewer examined domain logic,
contracts, HTTP, authentication, configuration, transactional persistence and tests.

Five findings were independently reproduced by the primary agent and accepted:

| Finding | Resolution |
| --- | --- |
| Missing statement draft payable became zero on acceptance | Draft candidate keeps a nullable amount; acceptance rejects unknown while preserving explicit zero |
| Unrelated movement consumed a card-specific funding plan | Match validation enforces plan account restrictions for entries and statements |
| Account maintenance changed closed-month coverage without revision | Closing captures reviewed card identities; reopening uses current inventory |
| Statement reporting override bypassed closed due-month checks | Create/revise/carry/settlement/credit check and bump every affected due/reporting month |
| Negative planned remainder appeared complete | Any nonzero signed remainder remains pending |

Regression tests reproduce these boundaries, including HTTP staging/acceptance,
creation and archival, wrong-card payments, signed plan kinds, rejected command
rollback, and old/new due-month revisions. No findings were dismissed.

## Cycle 2 — changes from cycle 1

Reviewed only `e60f225..ccceb112d4aa75007c4a558ef912a2309c3f74ed`, covering the
five fixes, associated schemas, documentation and regression tests. The independent
GPT-6.1 Sol reviewer found no actionable issues after checking interactions with
plan matching, statement carry/revision/settlement, draft serialization, and month
closure. Its domain and HTTP race tests passed. The primary agent inspected the
patch and confirmed full Go race tests with real PostgreSQL, vet, build, and
OpenAPI/fixture validation passed. No additional code changes were required.

## Validation evidence

- `python scripts/validate_contract.py`: generated contract current; OpenAPI 3.1,
  52 operations and 74 schemas valid; both synthetic fixtures valid.
- `go test -race -count=1 ./...` with PostgreSQL 17.6: all packages pass, including
  HTTP JSON Schema checks, concurrent retry/rollback, owner isolation and RLS.
- `go vet ./...`, formatting check, and server build pass.
- Initial container image builds; packaged non-root runtime connects to PostgreSQL
  and serves authenticated readiness/settings/month-summary smoke requests.

No hosted infrastructure or provider integration was provisioned. The local
service uses PostgreSQL; imports, Gmail and text capture remain explicit 501
contracts. Frontend, native Android, browser sessions, and restore are follow-up
work described in the API and service documentation.
