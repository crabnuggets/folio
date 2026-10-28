#!/usr/bin/env python3
"""Validate OpenAPI, JSON Schemas, and the shared synthetic fixture structure."""
import json
import subprocess
import sys
import uuid
from pathlib import Path

from jsonschema import Draft202012Validator
from openapi_spec_validator import validate

ROOT = Path(__file__).resolve().parents[1]
subprocess.run([sys.executable, str(ROOT / "scripts/generate_openapi.py"), "--check"], check=True)
spec = json.loads((ROOT / "api/openapi.yaml").read_text())
validate(spec)
money = Draft202012Validator(spec["components"]["schemas"]["Money"])
for value in ["0", "1", "-1", "9223372036854775807", "-9223372036854775808"]:
    assert money.is_valid(value), f"canonical Money rejected: {value}"
for value in ["-0", "00", "-01", "+1", "1.0"]:
    assert not money.is_valid(value), f"noncanonical Money accepted: {value}"
for name, schema in spec["components"]["schemas"].items():
    Draft202012Validator.check_schema(schema)
operations = [value for path in spec["paths"].values() for method, value in path.items() if method in {"get", "post", "put", "patch", "delete"}]
assert len({op["operationId"] for op in operations}) == len(operations), "duplicate operationId"
assert all(op.get("x-implementation") in {"implemented", "deferred"} for op in operations)
for op in operations:
    if op["x-implementation"] == "deferred":
        assert "501" in op["responses"], op["operationId"]
for path in sorted((ROOT / "fixtures").glob("*.json")):
    fixture = json.loads(path.read_text())
    assert fixture["schema_version"] == 1
    assert fixture["currency"] == "SGD"
    assert fixture["expected_summary"], path
    identities = {}
    def resolve(value):
        """Recursively replace $ references with prior fixture resource identities.

        Unknown references raise AssertionError.
        """
        if isinstance(value, str) and value.startswith("$"):
            assert value[1:] in identities, f"unknown fixture reference {value}"
            return identities[value[1:]]
        if isinstance(value, dict):
            return {key: resolve(item) for key, item in value.items()}
        if isinstance(value, list):
            return [resolve(item) for item in value]
        return value
    def check(schema_name, value):
        """Validate a fixture value against a component schema, including formats.

        Schema reference and validation errors propagate.
        """
        schema = {"$ref": "#/components/schemas/" + schema_name, "components": spec["components"]}
        Draft202012Validator(schema, format_checker=Draft202012Validator.FORMAT_CHECKER).validate(value)
    for group, schema_name in [("accounts", "AccountInput"), ("entries", "EntryInput"), ("statements", "StatementInput"), ("claims", "ClaimInput")]:
        for item in fixture[group]:
            assert item["ref"] not in identities, "duplicate fixture resource reference"
            check(schema_name, resolve(item["input"]))
            identities[item["ref"]] = str(uuid.uuid5(uuid.NAMESPACE_URL, fixture["name"] + "/" + item["ref"]))
    check("Checklist", fixture["checklist"])
    for key, value in fixture["expected_summary"].items():
        assert key in spec["components"]["schemas"]["Summary"]["properties"], key
        schema = {**spec["components"]["schemas"]["Summary"]["properties"][key], "components": spec["components"]}
        Draft202012Validator(schema).validate(value)
    print(f"Validated synthetic fixture {path.name}")
print(f"Validated OpenAPI 3.1: {len(operations)} operations, {len(spec['components']['schemas'])} schemas")
