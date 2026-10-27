#!/usr/bin/env python3
"""Build the reviewed Folio wire contract; --check detects uncommitted drift."""
from __future__ import annotations
import argparse
import copy
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCHEMAS: dict[str, dict] = {}

def ref(name):
    return {"$ref": f"#/components/schemas/{name}"}

def nullable(schema):
    return {"anyOf": [copy.deepcopy(schema), {"type": "null"}]}

def array(schema):
    return {"type": "array", "items": copy.deepcopy(schema)}

def obj(properties, required=None, description=None):
    result = {"type": "object", "additionalProperties": False, "properties": properties}
    if required is not None:
        result["required"] = required
    if description:
        result["description"] = description
    return result

def add(name, properties, required=None, description=None):
    SCHEMAS[name] = obj(properties, list(properties) if required is None else required, description)
    return ref(name)

def resource(name, input_name, extra=None):
    properties = {**SCHEMAS["Meta"]["properties"], **SCHEMAS[input_name]["properties"], **(extra or {})}
    return add(name, properties)

S = {"type": "string"}
BOOL = {"type": "boolean"}
VERSION = {"type": "integer", "minimum": 1, "format": "int64"}
DATE = {"type": "string", "format": "date", "description": "ISO local calendar date, not a UTC instant."}
INSTANT = {"type": "string", "format": "date-time"}
MONEY = {"type": "string", "pattern": r"^-?(0|[1-9][0-9]*)$", "description": "Canonical signed 64-bit decimal minor-unit integer string. Values outside -9223372036854775808..9223372036854775807 are invalid. Currency exponent comes from settings.", "examples": ["500000", "0", "-10000"]}
SCHEMAS["Money"] = MONEY
SCHEMAS["UUID"] = {"type": "string", "format": "uuid"}
SCHEMAS["MonthValue"] = {"type": "string", "pattern": r"^(19[0-9]{2}|[2-9][0-9]{3})-(0[1-9]|1[0-2])$", "examples": ["2026-10"]}
SCHEMAS["Currency"] = {"type": "string", "pattern": "^[A-Z]{3}$", "description": "Supported ISO 4217 currency equal to configured settings. Unknown currencies are rejected, never converted.", "examples": ["SGD"]}
SCHEMAS["PlanKind"] = {"type": "string", "enum": ["income", "spending", "allocation", "funding_inflow", "funding_outflow"]}
SCHEMAS["EntryKind"] = {"type": "string", "enum": ["income", "direct_expense", "refund", "reimbursement_receipt", "transfer", "envelope_funding", "envelope_return", "allocation", "allocation_withdrawal", "card_settlement"]}
ID = ref("UUID")
MONTH = ref("MonthValue")
CURRENCY = ref("Currency")
AMOUNT = ref("Money")
MAP_STR = {"type": "object", "additionalProperties": S}
add("Meta", {"id": ID, "version": VERSION, "created_at": INSTANT, "updated_at": INSTANT})
add("SettingsInput", {"currency": CURRENCY, "timezone": {"type": "string", "description": "IANA timezone name.", "examples": ["Asia/Singapore"]}})
add("Settings", {**SCHEMAS["SettingsInput"]["properties"], "version": VERSION, "currency_exponent": {"type": "integer", "minimum": 0, "maximum": 4}})
add("Boundary", {"effective_month": MONTH, "included": BOOL})
add("AccountInput", {"alias": S, "kind": {"type": "string", "enum": ["bank", "cash", "credit_card", "investment", "savings"]}, "currency": CURRENCY, "include_in_liquid_cash": BOOL, "active": BOOL, "funding_boundary": array(ref("Boundary"))}, ["alias", "kind", "currency", "active"], "PATCH supplies full account configuration. Funding boundary dates cannot silently restate existing history. Archive by setting active=false.")
resource("Account", "AccountInput")
add("EntryInput", {"funding_plan_line_id": nullable(ID), "kind": ref("EntryKind"), "amount_minor": AMOUNT, "currency": CURRENCY, "effective_date": DATE, "reporting_month": MONTH, "reporting_month_reason": S, "source_account_id": nullable(ID), "destination_account_id": nullable(ID), "category": S, "related_entry_id": nullable(ID), "plan_line_id": nullable(ID)}, ["kind", "amount_minor", "currency", "effective_date"], "Amount must be positive. Kind supplies financial sign. Reporting month defaults to effective month; override requires reason. One external source/destination is represented by null; owned transfers require both. Recognized plan and cash funding plan are independently matched. Refund/withdrawal require a related tracked entry and enforce cumulative caps.")
resource("Entry", "EntryInput", {"active": BOOL, "supersedes_id": nullable(ID), "correction_reason": S})
add("EntryCorrection", {"reason": S, "replacement": nullable(ref("EntryInput"))}, ["reason"], "Null replacement voids the original. Replacement appends a new record and retains original history. Linked records cannot be silently invalidated.")
add("StatementInput", {"account_id": ID, "currency": CURRENCY, "cycle_start": DATE, "cycle_end": DATE, "issue_date": DATE, "due_date": DATE, "reporting_month": MONTH, "reporting_month_reason": S, "amount_due_minor": AMOUNT, "minimum_due_minor": nullable(AMOUNT), "new_charges_minor": nullable(AMOUNT), "fees_minor": nullable(AMOUNT), "interest_minor": nullable(AMOUNT), "refunds_minor": nullable(AMOUNT), "net_spending_minor": nullable(AMOUNT), "spending_evidence": S, "carries_statement_id": nullable(ID), "plan_line_id": nullable(ID)}, ["account_id", "currency", "cycle_start", "cycle_end", "issue_date", "due_date", "amount_due_minor"], "Provide either all four nonnegative cycle components or an evidenced signed net spending amount, or no spending amounts (unknown). Amount due is never inferred spending. Reporting defaults to due month; override needs reason. Carry deactivates prior payable without removing historical spending.")
resource("Statement", "StatementInput", {"active": BOOL, "payable_active": BOOL, "supersedes_id": nullable(ID), "revision_reason": S})
add("StatementRevision", {"reason": S, "replacement": ref("StatementInput")})
add("SettlementInput", {"entry_id": ID, "amount_minor": AMOUNT}, description="Positive allocation of an existing card_settlement entry after statement cutoff. Does not create another cash entry. Sum across links cannot exceed payment amount.")
resource("Settlement", "SettlementInput", {"statement_id": ID})
add("ObligationAdjustmentInput", {"amount_minor": AMOUNT, "effective_date": DATE, "reason": S, "refund_entry_id": nullable(ID)}, ["amount_minor", "effective_date", "reason"], "Positive obligation credit after cutoff; linked refund identity cannot be used twice. Does not independently change recognized spending.")
resource("ObligationAdjustment", "ObligationAdjustmentInput", {"statement_id": ID})
add("SnapshotInput", {"account_id": ID, "amount_minor": AMOUNT, "currency": CURRENCY, "observed_at": INSTANT, "source": S}, ["account_id", "amount_minor", "currency", "observed_at"], "Dated observation, never an inferred balance from partial transactions.")
resource("Snapshot", "SnapshotInput")
add("ClaimInput", {"amount_minor": AMOUNT, "currency": CURRENCY, "expense_entry_id": nullable(ID), "statement_id": nullable(ID), "expected_receipt_date": nullable(DATE), "description": S, "state": {"type": "string", "enum": ["pending", "partially_received", "received", "disputed", "waived"]}}, ["amount_minor", "currency", "description"], "Pending claim is never received cash. At most one recognized cost link. Received state is derived from confirmed receipt matches; partial/received cannot be fabricated.")
resource("Claim", "ClaimInput")
add("ReceiptMatchInput", {"receipt_entry_id": ID, "amount_minor": AMOUNT, "recovery_month": nullable(MONTH)}, ["receipt_entry_id", "amount_minor"], "Positive receipt allocation capped by available receipt, claim and known unrecovered cost. Recovery month is separate from cash receipt month and must be open. Unmatched cost leaves recovery month null. Repeating an unrecovered allocation with the same receipt/claim/amount and explicit recovery_month classifies that existing match once; its ID is retained and version increases, never a second cash allocation.")
resource("ReceiptMatch", "ReceiptMatchInput", {"claim_id": ID})
add("PlanLine", {"id": ID, "kind": ref("PlanKind"), "expected_minor": AMOUNT, "estimated_remaining_minor": nullable(AMOUNT), "complete": BOOL, "account_id": nullable(ID), "recurrence_id": nullable(ID), "description": S}, ["id", "kind", "expected_minor", "complete"], "Stable line identity used by actuals. Incomplete forecast=actual+explicit remaining, or max(expected-actual,0) for ordinary positive amounts. Signed actuals/targets require explicit remaining. Complete forecast=actual. Funding and recognized spending lines are separate.")
add("PlanInput", {"lines": array(ref("PlanLine"))})
add("Plan", {"version": VERSION, "lines": array(ref("PlanLine"))})
add("Checklist", {"income_reviewed": BOOL, "cards_reviewed": BOOL, "expenses_reviewed": BOOL, "allocations_reviewed": BOOL, "no_statement_reasons": MAP_STR}, [], "All four assertions are required to be true at close. Missing statement is different from explicit account UUID→reason no-statement assertion.")
add("Month", {"month": MONTH, "version": VERSION, "revision": {"type": "integer", "format": "int64", "minimum": 0}, "state": {"type": "string", "enum": ["open", "closed"]}, "checklist": ref("Checklist"), "closed_at": nullable(INSTANT)})
add("MonthTransition", {"reason": S, "checklist": ref("Checklist")}, [], "Close requires the complete checklist. Reopen requires a nonblank reason; subsequent changes increase revision.")
add("RecurrenceInput", {"kind": ref("PlanKind"), "expected_minor": AMOUNT, "start_month": MONTH, "end_month": nullable(MONTH), "description": S, "account_id": nullable(ID)}, ["kind", "expected_minor", "start_month", "description"], "Effective-dated expectation only, never a posting. End month is inclusive. Revisions preserve historical targets.")
resource("Recurrence", "RecurrenceInput", {"supersedes_id": nullable(ID)})
add("PeriodInput", {"start_date": DATE, "end_date": DATE, "reporting_month": MONTH, "additions_minor": AMOUNT, "deductions_minor": AMOUNT, "evidence": S}, ["start_date", "end_date", "reporting_month", "additions_minor", "deductions_minor"], "Reconciliation observation only. Does not automatically post a movement or expense.")
resource("Period", "PeriodInput", {"remainder_minor": AMOUNT, "proposed_top_up_minor": AMOUNT})
# Staged candidates intentionally retain missing/invalid fields for human review.
# Their scalar wire types remain bounded by Go decoding; financial validation is
# applied only at acceptance. Posted records use the strict schemas above.
for candidate, posted in [("DraftEntryCandidate", "EntryInput"), ("DraftStatementCandidate", "StatementInput")]:
    properties = {}
    for field, schema in SCHEMAS[posted]["properties"].items():
        if field.endswith("_minor"):
            properties[field] = nullable(AMOUNT) if "anyOf" in schema else AMOUNT
        else:
            properties[field] = nullable(S) if "anyOf" in schema else S
    add(candidate, properties, [], "Untrusted structured candidate. Missing strings may be empty on reads. Unknown/invalid dates, accounts and categories remain staged until acceptance enforces the posted-record contract. Monetary JSON must still be an integer string within signed 64-bit range.")
add("DraftInput", {"source_id": S, "external_id": S, "entry": nullable(ref("DraftEntryCandidate")), "statement": nullable(ref("DraftStatementCandidate")), "evidence": S}, ["source_id", "external_id", "evidence"], "Exactly one entry or statement candidate. Durable source+external identity prevents repeat observation from making another posting. Staging is allowed in closed months; acceptance applies all posting validation atomically.")
resource("Draft", "DraftInput", {"state": {"type": "string", "enum": ["pending", "accepted", "rejected"]}, "accepted_record_id": nullable(ID), "rejection_reason": S})
add("ReasonInput", {"reason": {"type": "string", "minLength": 1}})
add("EmptyInput", {}, [])
add("Audit", {"id": ID, "action": S, "entity_id": S, "reason": S, "at": INSTANT, "before": {}, "after": {}})
resource("LegacySummary", "EmptyInput", {"month": MONTH, "amount_minor": AMOUNT, "source_coordinates": S, "complete": BOOL})
add("StatementBalance", {"statement_id": ID, "amount_due_minor": AMOUNT, "remaining_minor": AMOUNT, "credit_minor": AMOUNT, "spending_minor": nullable(AMOUNT), "payable_active": BOOL})
add("ForecastLine", {"id": ID, "kind": ref("PlanKind"), "actual_minor": AMOUNT, "remaining_minor": AMOUNT, "forecast_minor": AMOUNT})
summary_props = {"month": MONTH, "currency": CURRENCY, "revision": {"type": "integer", "minimum": 0}}
for name in ["income_minor", "gross_spending_minor", "confirmed_recoveries_minor", "known_spending_minor", "net_personal_cost_minor", "allocation_minor", "provisional_surplus_minor", "funding_inflows_minor", "funding_outflows_minor", "funding_remainder_minor", "unmatched_claims_minor", "unmatched_receipts_minor"]:
    summary_props[name] = AMOUNT
for name in ["monthly_surplus_minor", "consumption_savings_minor", "projected_surplus_minor", "projected_funding_remainder_minor"]:
    summary_props[name] = nullable(AMOUNT)
summary_props.update({"savings_rate": nullable(S), "is_provisional": BOOL, "spending_complete": BOOL, "funding_complete": BOOL, "missing_items": array(S), "statements": array(ref("StatementBalance")), "forecast_lines": array(ref("ForecastLine")), "snapshots": array(ref("Snapshot"))})
add("Summary", summary_props, description="Server-calculated figures; monthly surplus is null while spending coverage is incomplete. Known subtotal and provisional surplus remain labeled. Funding remainder excludes opening cash and is not bank balance. Unknown statement spending never silently becomes zero. Unpaid card obligations without an explicit funding_outflow plan identifying that card leave projected funding null; a payer is never inferred. Outstanding claims are scoped by expected receipt month, else linked cost month, and undated unlinked claims are excluded from month totals.")
add("Insight", {"code": S, "message": S, "values": {"type": "object", "additionalProperties": AMOUNT}})
add("Insights", {"items": array(ref("Insight"))})
state_props = {"settings": ref("Settings")}
for field, schema in [("accounts", "Account"), ("entries", "Entry"), ("statements", "Statement"), ("settlements", "Settlement"), ("obligation_adjustments", "ObligationAdjustment"), ("snapshots", "Snapshot"), ("claims", "Claim"), ("receipt_matches", "ReceiptMatch"), ("plans", "Plan"), ("months", "Month"), ("recurrences", "Recurrence"), ("periods", "Period"), ("drafts", "Draft")]:
    state_props[field] = {"type": "object", "additionalProperties": ref(schema)}
state_props.update({"audit_events": array(ref("Audit")), "legacy_summaries": array(ref("LegacySummary"))})
add("State", state_props)
add("Export", {"schema_version": {"type": "integer", "const": 1}, "exported_at": INSTANT, "state": ref("State")}, description="Lossless JSON export with financial histories, sources, timestamps and versions. Contains sensitive financial data. No provider secrets or HTTP retry records.")
add("ErrorDetail", {"code": S, "message": S, "fields": MAP_STR, "request_id": S})
add("ErrorEnvelope", {"error": ref("ErrorDetail")})
add("Health", {"status": S})
# Deferred capabilities specify intended transitions without claiming live providers.
add("ImportRequest", {"source_kind": {"type": "string", "enum": ["csv", "statement_text", "statement_file"]}, "external_id": S, "content": S, "filename": nullable(S), "content_type": S}, ["source_kind", "external_id", "content", "content_type"], "P5. Content is text or base64 file content according to content_type; bounded payload. Source uniqueness persists beyond HTTP retry retention. Parsed values remain drafts until explicit acceptance.")
add("ImportJob", {"id": ID, "source_id": ID, "status": {"type": "string", "enum": ["queued", "running", "needs_review", "failed"]}, "draft_ids": array(ID), "retry_at": nullable(INSTANT), "failure": nullable(S), "created_at": INSTANT, "updated_at": INSTANT}, description="P5/P6. Durable job, redacted failure, draft review boundary. Retry-After on queued/running reads prevents polling storms.")
add("GmailConnectRequest", {"redirect_uri": {"type": "string", "format": "uri"}}, description="P6. Redirect must be on backend allowlist. Gmail authorization is separate from Folio login. Read-only Gmail scopes only.")
add("GmailConnectResponse", {"authorization_url": {"type": "string", "format": "uri"}, "expires_at": INSTANT}, description="P6. Server stores one-time state and PKCE verifier. Callback validates both before exchanging code.")
add("GmailConnection", {"state": {"type": "string", "enum": ["connected", "disconnected", "needs_reauthorization"]}, "connected_at": nullable(INSTANT), "last_sync_at": nullable(INSTANT)})
add("GmailSyncRequest", {"from_date": DATE, "to_date": DATE, "max_messages": {"type": "integer", "minimum": 1, "maximum": 100, "default": 50}}, ["from_date", "to_date"], "P6. Bounded interval and count; source IDs deduplicate on-demand retries. Unattended scheduling is not selected.")
add("CaptureRequest", {"text": {"type": "string", "minLength": 1, "maxLength": 10000}, "captured_at": INSTANT}, description="P7. Resolve relative dates from capture instant and configured timezone; ambiguous fields remain review questions. Never posts received income merely from future-tense text.")
add("CaptureResponse", {"draft_ids": array(ID), "questions": array(S)}, description="P7. Drafts only; requires the ordinary explicit acceptance boundary.")

for name in ["Account", "Entry", "Statement", "Snapshot", "Claim", "Recurrence", "Period", "Draft", "Audit", "LegacySummary"]:
    add(name + "Page", {"items": array(ref(name)), "next_cursor": nullable(ID)})

HEADERS = {
    "X-Request-ID": {"description": "Request correlation identifier.", "schema": S},
    "Cache-Control": {"description": "Financial API responses are never cached.", "schema": {"type": "string", "const": "no-store"}},
}
PARAMS = {
    "IdempotencyKey": {"name": "Idempotency-Key", "in": "header", "required": True, "schema": ID, "description": "Client UUID scoped to owner. Same method/path/body/precondition replays committed status/body/ETag. Different content with same key returns 409. Retained at least 30 days (indefinitely initially)."},
    "IfMatch": {"name": "If-Match", "in": "header", "required": True, "schema": {"type": "string", "pattern": '^"[1-9][0-9]*"$'}, "description": "Strong version ETag returned by current resource read. Missing 428, stale 412."},
    "Limit": {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1, "maximum": 100, "default": 50}},
    "Cursor": {"name": "cursor", "in": "query", "schema": ID, "description": "Last returned UUID, exclusive, ordered lexicographically. Null next_cursor marks end."},
    "MonthFilter": {"name": "month", "in": "query", "schema": MONTH},
}
PATHS = {}
ERROR_STATUSES = {"400": "Malformed request", "401": "Missing or invalid authentication", "403": "Authenticated subject is not configured owner", "404": "Missing or inaccessible record", "409": "Domain conflict, closed month, or reused idempotency key", "412": "Stale resource version", "422": "Invalid financial data", "428": "Missing version precondition", "413": "Request body exceeds 1 MiB", "415": "Unsupported media type", "500": "Internal error", "503": "Dependency unavailable"}

def response(schema, description="Success", etag=False):
    headers = copy.deepcopy(HEADERS)
    if etag:
        headers["ETag"] = {"description": "Strong optimistic concurrency resource version.", "schema": {"type": "string", "pattern": '^"[1-9][0-9]*"$'}}
    return {"description": description, "headers": headers, "content": {"application/json": {"schema": schema}}}

def operation(path, method, name, result, request=None, status=200, version=False, listing=False, month_filter=False, deferred=False, summary=None):
    parameters = []
    for field in ["id", "month"]:
        if "{" + field + "}" in path:
            parameters.append({"name": field, "in": "path", "required": True, "schema": ID if field == "id" else MONTH})
    mutation = method in ["post", "put", "patch", "delete"]
    if mutation:
        parameters.append({"$ref": "#/components/parameters/IdempotencyKey"})
    if version:
        parameters.append({"$ref": "#/components/parameters/IfMatch"})
    if listing:
        parameters += [{"$ref": "#/components/parameters/Limit"}, {"$ref": "#/components/parameters/Cursor"}]
    if month_filter:
        parameters.append({"$ref": "#/components/parameters/MonthFilter"})
    responses = {str(status): response(ref(result), "Planned successful response (capability deferred)" if deferred else "Success", etag=result in ["Account", "Settings", "Entry", "Statement", "Claim", "Plan", "Month", "Recurrence", "Draft"])}
    responses.update({code: response(ref("ErrorEnvelope"), desc) for code, desc in ERROR_STATUSES.items()})
    if deferred:
        responses["501"] = response(ref("ErrorEnvelope"), "FEATURE_NOT_AVAILABLE: integration is explicitly deferred; currently the only capability response after authentication/request validation")
    value = {"operationId": name, "summary": summary or name, "parameters": parameters, "responses": responses, "x-implementation": "deferred" if deferred else "implemented"}
    if request:
        value["requestBody"] = {"required": True, "content": {"application/json": {"schema": ref(request)}}}
    PATHS.setdefault(path, {})[method] = value

operation("/settings", "get", "getSettings", "Settings")
operation("/settings", "patch", "updateSettings", "Settings", "SettingsInput", version=True)
for path, singular, input_name in [("accounts", "Account", "AccountInput"), ("entries", "Entry", "EntryInput"), ("statements", "Statement", "StatementInput"), ("balance-snapshots", "Snapshot", "SnapshotInput"), ("claims", "Claim", "ClaimInput"), ("recurrences", "Recurrence", "RecurrenceInput"), ("reconciliation-periods", "Period", "PeriodInput"), ("drafts", "Draft", "DraftInput")]:
    operation("/" + path, "get", "list" + singular + "s", singular + "Page", listing=True, month_filter=path in ["entries", "statements"])
    operation("/" + path, "post", "create" + singular, singular, input_name, status=201)
for path, singular in [("accounts", "Account"), ("entries", "Entry"), ("statements", "Statement"), ("claims", "Claim"), ("drafts", "Draft")]:
    operation("/" + path + "/{id}", "get", "get" + singular, singular)
operation("/accounts/{id}", "patch", "updateAccount", "Account", "AccountInput", version=True)
operation("/claims/{id}", "patch", "updateClaim", "Claim", "ClaimInput", version=True)
operation("/entries/{id}/corrections", "post", "correctEntry", "Entry", "EntryCorrection", version=True)
operation("/statements/{id}/revisions", "post", "reviseStatement", "Statement", "StatementRevision", status=201, version=True)
operation("/statements/{id}/settlements", "post", "createSettlement", "Settlement", "SettlementInput", status=201, version=True)
operation("/statements/{id}/obligation-adjustments", "post", "createObligationAdjustment", "ObligationAdjustment", "ObligationAdjustmentInput", status=201, version=True)
operation("/claims/{id}/receipt-matches", "post", "matchReceipt", "ReceiptMatch", "ReceiptMatchInput", status=201, version=True)
operation("/months/{month}", "get", "getMonth", "Month")
operation("/months/{month}/plan", "get", "getPlan", "Plan")
operation("/months/{month}/plan", "put", "putPlan", "Plan", "PlanInput", version=True)
operation("/months/{month}/summary", "get", "getSummary", "Summary")
operation("/months/{month}/close", "post", "closeMonth", "Month", "MonthTransition", version=True)
operation("/months/{month}/reopen", "post", "reopenMonth", "Month", "MonthTransition", version=True)
operation("/months/{month}/insights", "get", "getInsights", "Insights")
operation("/recurrences/{id}/revisions", "post", "reviseRecurrence", "Recurrence", "RecurrenceInput", status=201, version=True)
operation("/drafts/{id}/accept", "post", "acceptDraft", "Draft", "EmptyInput", version=True)
operation("/drafts/{id}/reject", "post", "rejectDraft", "Draft", "ReasonInput", version=True)
operation("/legacy-month-summaries", "get", "listLegacyMonthSummaries", "LegacySummaryPage", listing=True)
operation("/audit-events", "get", "listAuditEvents", "AuditPage", listing=True)
operation("/exports", "get", "exportFinancialState", "Export")
operation("/imports", "post", "createImport", "ImportJob", "ImportRequest", status=202, deferred=True)
operation("/imports/{id}", "get", "getImport", "ImportJob", deferred=True)
operation("/gmail/connect", "post", "connectGmail", "GmailConnectResponse", "GmailConnectRequest", deferred=True)
operation("/gmail/callback", "get", "gmailCallback", "GmailConnection", deferred=True)
PATHS["/gmail/callback"]["get"]["parameters"] += [{"name": "code", "in": "query", "required": True, "schema": S}, {"name": "state", "in": "query", "required": True, "schema": S}]
operation("/gmail/sync", "post", "syncGmail", "ImportJob", "GmailSyncRequest", status=202, deferred=True)
operation("/gmail/connection", "delete", "disconnectGmail", "GmailConnection", deferred=True)
operation("/capture/text", "post", "captureText", "CaptureResponse", "CaptureRequest", deferred=True)
for name in ["healthz", "readyz"]:
    PATHS["/" + name] = {"get": {"operationId": name, "summary": "Process liveness" if name == "healthz" else "Database readiness", "servers": [{"url": "/"}], "security": [], "responses": {"200": response(ref("Health")), "503": response(ref("ErrorEnvelope"), "Dependency unavailable")}, "x-implementation": "implemented"}}

DOCUMENT = {"openapi": "3.1.0", "info": {"title": "Folio shared API", "version": "1.0.0", "description": "Shared server-computed financial contracts for React and Kotlin Android. Manual financial core is implemented. Explicit x-implementation=deferred routes return 501 until provider milestones. See api/README.md for accounting and transition rules."}, "jsonSchemaDialect": "https://json-schema.org/draft/2020-12/schema", "servers": [{"url": "/api/v1"}], "security": [{"BearerAuth": []}], "paths": PATHS, "components": {"securitySchemes": {"BearerAuth": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT or explicit local dev token", "description": "Configured single owner only. Local bypass token requires explicit development auth and cannot start in production."}}, "parameters": PARAMS, "schemas": SCHEMAS}}

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    target = ROOT / "api/openapi.yaml"
    text = json.dumps(DOCUMENT, indent=2, ensure_ascii=False) + "\n"
    if args.check:
        if not target.exists() or target.read_text() != text:
            raise SystemExit("api/openapi.yaml differs from reviewed generator; run python scripts/generate_openapi.py")
        print("OpenAPI generation is current")
    else:
        target.write_text(text)
        print(f"Wrote {len(PATHS)} paths and {len(SCHEMAS)} schemas")

if __name__ == "__main__":
    main()
