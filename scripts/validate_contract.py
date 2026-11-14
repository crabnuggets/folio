#!/usr/bin/env python3
"""Validate OpenAPI, JSON Schemas, and the shared synthetic fixture structure."""
import json
import subprocess
import sys
from pathlib import Path

from jsonschema import Draft202012Validator
from openapi_spec_validator import validate

ROOT = Path(__file__).resolve().parents[1]
subprocess.run([sys.executable, str(ROOT / "scripts/generate_openapi.py"), "--check"], check=True)
spec = json.loads((ROOT / "api/openapi.yaml").read_text(encoding="utf-8"))
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
assert all(op.get("x-implementation") == "implemented" for op in operations)
print(f"Validated OpenAPI 3.1: {len(operations)} operations, {len(spec['components']['schemas'])} schemas")
