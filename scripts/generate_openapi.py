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
    """Return a local component-schema reference without checking its existence."""
    return {"$ref": f"#/components/schemas/{name}"}

def nullable(schema):
    """Allow either a copied schema or JSON null."""
    return {"anyOf": [copy.deepcopy(schema), {"type": "null"}]}

def array(schema):
    """Build an array schema with an independent copy of its item schema."""
    return {"type": "array", "items": copy.deepcopy(schema)}

def obj(properties, required=None, description=None):
    """Build a closed object schema, omitting required when it is None."""
    result = {"type": "object", "additionalProperties": False, "properties": properties}
    if required is not None:
        result["required"] = required
    if description:
        result["description"] = description
    return result

def add(name, properties, required=None, description=None):
    """Register or replace a schema and return its reference.

    All properties are required by default; an explicit list overrides that default.
    """
    SCHEMAS[name] = obj(properties, list(properties) if required is None else required, description)
    return ref(name)

def resource(name, input_name, extra=None):
    """Register a resource with metadata, input fields, and optional extra fields.

    All merged fields are required. Missing source schemas raise KeyError.
    """
    properties = {**SCHEMAS["Meta"]["properties"], **SCHEMAS[input_name]["properties"], **(extra or {})}
    return add(name, properties)

S = {"type": "string"}
BOOL = {"type": "boolean"}
VERSION = {"type": "integer", "minimum": 1, "format": "int64"}
DATE = {"type": "string", "format": "date", "description": "ISO local calendar date, not a UTC instant."}
INSTANT = {"type": "string", "format": "date-time"}
MONEY = {"type": "string", "pattern": r"^(0|-?[1-9][0-9]*)$", "description": "Canonical signed 64-bit decimal minor-unit integer string. Values outside -9223372036854775808..9223372036854775807 are invalid. Currency exponent comes from settings.", "examples": ["500000", "0", "-10000"]}
SCHEMAS["Money"] = MONEY
SCHEMAS["UUID"] = {"type": "string", "format": "uuid"}
SCHEMAS["MonthValue"] = {"type": "string", "pattern": r"^(19[0-9]{2}|[2-9][0-9]{3})-(0[1-9]|1[0-2])$", "examples": ["2026-10"]}
SCHEMAS["Currency"] = {"type": "string", "pattern": "^[A-Z]{3}$", "description": "Supported ISO 4217 currency equal to configured settings. Unknown currencies are rejected, never converted.", "examples": ["SGD"]}
ID = ref("UUID")
MONTH = ref("MonthValue")
CURRENCY = ref("Currency")
AMOUNT = ref("Money")
MAP_STR = {"type": "object", "additionalProperties": S}
add("Meta", {"id": ID, "version": VERSION, "created_at": INSTANT, "updated_at": INSTANT})
add("SettingsInput", {"currency": CURRENCY, "timezone": {"type": "string", "description": "IANA timezone name.", "examples": ["Asia/Singapore"]}})
add("Settings", {**SCHEMAS["SettingsInput"]["properties"], "version": VERSION, "currency_exponent": {"type": "integer", "minimum": 0, "maximum": 4}})
add("EmptyInput", {}, [])
add("Audit", {"id": ID, "action": S, "entity_id": S, "reason": S, "at": INSTANT, "before": {}, "after": {}})
add("ErrorDetail", {"code": S, "message": S, "fields": MAP_STR, "request_id": S})
add("ErrorEnvelope", {"error": ref("ErrorDetail")})
add("Health", {"status": S})
direction = {"type": "string", "enum": ["in", "out"]}
label = {"type": "string", "enum": ["Amount", "Bill amount"]}
monthly_fields = {"section_id": ID, "name": {"type": "string", "minLength": 1, "maxLength": 120}, "amount_minor": nullable(AMOUNT), "note": S, "repeat_monthly": BOOL, "usual_amount_minor": nullable(AMOUNT)}
add("MonthlyItemInput", {**monthly_fields, "item_id": nullable(ID)}, ["section_id", "name", "amount_minor"])
add("MonthlyExplanation", {"name": monthly_fields["name"], "amount_minor": AMOUNT}, description="A named positive amount explaining part of one monthly outgoing item. No nesting. The sum cannot exceed the parent monthly amount.")
explanations = array(ref("MonthlyExplanation"))
add("MonthlyItemUpdate", {**monthly_fields, "explanations": {**explanations, "description": "Replaces this month's explanations when supplied. An empty array removes all explanations. Omit to preserve existing explanations."}}, list(monthly_fields))
add("MonthlyItem", {"id": ID, **monthly_fields, "recurrence_since": {**nullable(MONTH), "description": "Month the item's current recurrence settings (repeat_monthly, usual_amount_minor and section) started."}, "explanations": explanations, "unexplained_minor": {**AMOUNT, "description": "Remaining amount, present only when explanations exist and their sum is less than the parent amount."}}, ["id", *monthly_fields, "recurrence_since"])
add("MonthlySectionDefinition", {"id": ID, "name": S, "direction": direction, "amount_label": label})
add("MonthlySection", {**SCHEMAS["MonthlySectionDefinition"]["properties"], "total_minor": AMOUNT, "missing_count": {"type": "integer", "minimum": 0}, "items": array(ref("MonthlyItem"))})
add("MonthlySheet", {"month": MONTH, "version": VERSION, "currency": CURRENCY, "currency_exponent": {"type": "integer", "minimum": 0, "maximum": 4}, "currency_locked": BOOL, "undo_token": nullable(ID), "money_in_minor": AMOUNT, "money_out_minor": AMOUNT, "remainder_minor": AMOUNT, "missing_count": {"type": "integer", "minimum": 0}, "sections": array(ref("MonthlySection"))})
add("MonthlyItemSuggestion", {"id": ID, "name": S, "section_id": ID, "section_name": S, "direction": direction, "repeat_monthly": BOOL, "usual_amount_minor": nullable(AMOUNT), "current_month_item_id": nullable(ID)})
add("MonthlyItemSuggestions", {"items": array(ref("MonthlyItemSuggestion"))})
add("MonthlyUsualInput", {"section_id": nullable(ID)}, [])
add("MonthlyUndoInput", {"undo_token": ID})
add("MonthlySectionInput", {"name": {"type": "string", "minLength": 1, "maxLength": 120}, "direction": direction})
add("MonthlySectionRename", {"name": {"type": "string", "minLength": 1, "maxLength": 120}})
add("MonthlyPreference", {"month": MONTH, "section_id": ID, "repeat_monthly": BOOL, "usual_amount_minor": nullable(AMOUNT)})
add("MonthlyItemIdentity", {"id": ID, "name": S, "direction": direction, "aliases": array(S), "preferences": array(ref("MonthlyPreference"))})
add("MonthlyStoredRow", {"item_id": ID, "section_id": ID, "amount_minor": nullable(AMOUNT), "note": S, "removed": BOOL, "generated": BOOL, "touched": BOOL})
SCHEMAS["MonthlyStoredRow"]["properties"]["explanations"] = explanations
monthly_data = {"sections": array(ref("MonthlySectionDefinition")), "items": {"type": "object", "additionalProperties": ref("MonthlyItemIdentity")}, "rows": {"type": "object", "additionalProperties": {"type": "object", "additionalProperties": ref("MonthlyStoredRow")}}}
add("MonthlySheetData", monthly_data)
add("MonthlyStoredUndo", {"token": ID, "version": VERSION, "before": ref("MonthlySheetData")})
add("MonthlySheetState", {**monthly_data, "version": VERSION, "currency_locked": BOOL, "undo": nullable(ref("MonthlyStoredUndo"))})
draft_fields = {"month": MONTH, "section_id": ID, "name": {"type":"string","minLength":1,"maxLength":120}, "amount_minor": {**nullable(AMOUNT), "description": "Proposed amount for the month. Blank only for a monthly recurring item proposed without one, and for a recurring item change."}, "note": S, "evidence": S, "source_id": S, "external_id": S, "sheet_version": VERSION, "explanations": {**explanations, "description": "Breakdown of a money-out proposal, copied to the item when accepted. The sum cannot exceed amount_minor."}, "repeat_monthly": {**BOOL, "description": "Makes a new item a monthly recurring item from month. For a recurring item change, whether the item repeats after the change."}, "usual_amount_minor": {**nullable(AMOUNT), "description": "Usual amount suggested for the recurring item. Requires repeat_monthly."}}
add("MonthlyDraftInput", draft_fields)
add("MonthlyAmendment", {"item_id": ID, "base_amount_minor": AMOUNT, "base_explanations": explanations}, description="The entered amount and explanations an amendment was proposed against. Acceptance replaces them and is refused once they differ.")
add("MonthlyRecurrenceChange", {"item_id": ID, "base_section_id": ID, "base_repeat_monthly": BOOL, "base_usual_amount_minor": nullable(AMOUNT)}, description="The recurrence settings a recurring item change was proposed against. The proposal's section_id, repeat_monthly and usual_amount_minor hold the settings from its month onward. Acceptance is refused once the base settings differ.")
resource("MonthlyDraft", "MonthlyDraftInput", {"amendment":nullable(ref("MonthlyAmendment")), "recurrence_change":nullable(ref("MonthlyRecurrenceChange")), "state":{"type":"string","enum":["pending","accepted","rejected"]}, "accepted_item_id":nullable(ID), "proposal_hash":S})
draft_edit = ["section_id","name","amount_minor","note"]
add("MonthlyDraftEdit", {**{key:draft_fields[key] for key in draft_edit}, "explanations": {**explanations, "description": "Replaces the proposal's breakdown when supplied. An empty array removes it. Omit to keep it."}}, draft_edit)
add("MonthlyDraftAccept", {"sheet_version":VERSION})
for name in ["MonthlyDraft", "Audit"]:
    add(name + "Page", {"items": array(ref(name)), "next_cursor": nullable(ID)})
add("State", {"settings":ref("Settings"),"monthly_sheet":ref("MonthlySheetState"),"monthly_drafts":{"type":"object","additionalProperties":ref("MonthlyDraft")},"audit_events":array(ref("Audit"))})
add("Export", {"schema_version":{"type":"integer","const":2},"exported_at":INSTANT,"state":ref("State")}, description="Monthly sheets, proposal history, and audit. Private provider state and retired aggregate fields are excluded.")

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
    """Build a JSON response with common headers and an optional version ETag."""
    headers = copy.deepcopy(HEADERS)
    if etag:
        headers["ETag"] = {"description": "Strong optimistic concurrency resource version.", "schema": {"type": "string", "pattern": '^"[1-9][0-9]*"$'}}
    return {"description": description, "headers": headers, "content": {"application/json": {"schema": schema}}}

def operation(path, method, name, result, request=None, status=200, version=False, listing=False, month_filter=False, summary=None):
    """Register a path operation in PATHS with result and optional request schema names.

    Flags add version, pagination, or month parameters. Mutating methods get retry
    keys.
    An existing operation at the same path and method is replaced.
    """
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
    responses = {str(status): response(ref(result), "Success", etag=result in ["Settings", "MonthlyDraft", "MonthlySheet"])}
    responses.update({code: response(ref("ErrorEnvelope"), desc) for code, desc in ERROR_STATUSES.items()})
    value = {"operationId": name, "summary": summary or name, "parameters": parameters, "responses": responses, "x-implementation": "implemented"}
    if request:
        value["requestBody"] = {"required": True, "content": {"application/json": {"schema": ref(request)}}}
    PATHS.setdefault(path, {})[method] = value

operation("/settings", "get", "getSettings", "Settings")
operation("/settings", "patch", "updateSettings", "Settings", "SettingsInput", version=True)
operation("/monthly-drafts", "get", "listPendingMonthlyDrafts", "MonthlyDraftPage", listing=True, month_filter=True)
operation("/monthly-drafts/{id}", "put", "editMonthlyDraft", "MonthlyDraft", "MonthlyDraftEdit", version=True)
operation("/monthly-drafts/{id}/accept", "post", "acceptMonthlyDraft", "MonthlySheet", "MonthlyDraftAccept", version=True)
operation("/monthly-drafts/{id}/reject", "post", "rejectMonthlyDraft", "MonthlyDraft", "EmptyInput", version=True)
operation("/audit-events", "get", "listAuditEvents", "AuditPage", listing=True)
operation("/exports", "get", "exportFinancialState", "Export")
operation("/monthly-sheets/{month}", "get", "getMonthlySheet", "MonthlySheet")
operation("/monthly-sheets/{month}/open", "post", "openMonthlySheet", "MonthlySheet", "EmptyInput")
operation("/monthly-sheets/{month}/items", "post", "addMonthlyItem", "MonthlySheet", "MonthlyItemInput", version=True)
operation("/monthly-sheets/{month}/items/{id}", "put", "updateMonthlyItem", "MonthlySheet", "MonthlyItemUpdate", version=True)
for action in ["remove", "restore"]:
    operation("/monthly-sheets/{month}/items/{id}/" + action, "post", action + "MonthlyItem", "MonthlySheet", "EmptyInput", version=True)
operation("/monthly-sheets/{month}/usual-amounts", "post", "useMonthlyUsualAmounts", "MonthlySheet", "MonthlyUsualInput", version=True)
operation("/monthly-sheets/{month}/undo", "post", "undoMonthlySheet", "MonthlySheet", "MonthlyUndoInput", version=True)
operation("/monthly-sheets/{month}/sections", "post", "createMonthlySection", "MonthlySheet", "MonthlySectionInput", version=True)
operation("/monthly-sheets/{month}/sections/{id}", "put", "renameMonthlySection", "MonthlySheet", "MonthlySectionRename", version=True)
operation("/monthly-item-suggestions", "get", "suggestMonthlyItems", "MonthlyItemSuggestions")
PATHS["/monthly-item-suggestions"]["get"]["parameters"] = [{"name": "month", "in": "query", "required": True, "schema": MONTH}, {"name": "section_id", "in": "query", "required": True, "schema": ID}, {"name": "q", "in": "query", "schema": S}]
for name in ["healthz", "readyz"]:
    PATHS["/" + name] = {"get": {"operationId": name, "summary": "Process liveness" if name == "healthz" else "Database readiness", "servers": [{"url": "/"}], "security": [], "responses": {"200": response(ref("Health")), "503": response(ref("ErrorEnvelope"), "Dependency unavailable")}, "x-implementation": "implemented"}}

DOCUMENT = {"openapi": "3.1.0", "info": {"title": "Folio shared API", "version": "1.0.0", "description": "Monthly sheet and pending proposal contracts. See docs/mcp.md for the authenticated MCP transport and provider setup."}, "jsonSchemaDialect": "https://json-schema.org/draft/2020-12/schema", "servers": [{"url": "/api/v1"}], "security": [{"BearerAuth": []}], "paths": PATHS, "components": {"securitySchemes": {"BearerAuth": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT or explicit local dev token", "description": "Configured single owner only. Local bypass token requires explicit development auth and cannot start in production."}}, "parameters": PARAMS, "schemas": SCHEMAS}}

def main():
    """Write the generated contract, or check it without writing with --check.

    Argument errors and missing or different output in check mode raise SystemExit;
    file access errors propagate.
    """
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    target = ROOT / "api/openapi.yaml"
    text = json.dumps(DOCUMENT, indent=2, ensure_ascii=False) + "\n"
    if args.check:
        if not target.exists() or target.read_text(encoding="utf-8") != text:
            raise SystemExit("api/openapi.yaml differs from reviewed generator; run python scripts/generate_openapi.py")
        print("OpenAPI generation is current")
    else:
        target.write_text(text, encoding="utf-8", newline="\n")
        print(f"Wrote {len(PATHS)} paths and {len(SCHEMAS)} schemas")

if __name__ == "__main__":
    main()
