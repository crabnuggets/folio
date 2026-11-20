package httpapi

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

// Required-field checks distinguish omitted fields from explicit zero/false.
var requiredFields = map[string][]string{
	"MonthlyExplanation":   {"name", "amount_minor"},
	"MonthlyItemInput":     {"section_id", "name", "amount_minor"},
	"MonthlyItemUpdate":    {"section_id", "name", "amount_minor", "note", "repeat_monthly", "usual_amount_minor"},
	"MonthlyUndoInput":     {"undo_token"},
	"MonthlySectionInput":  {"name", "direction"},
	"MonthlySectionRename": {"name"},
	"SettingsInput":        {"currency", "timezone"},
	"MonthlyDraftEdit":     {"section_id", "name", "amount_minor", "note"},
	"MonthlyDraftAccept":   {"sheet_version"},
}

// validateRequired recursively checks registered required fields and rejects null
// non-pointer fields in request structs. Invalid shapes, missing required fields,
// or disallowed nulls return MALFORMED_JSON;
// scalar value decoding and unknown-field checks are left to readJSON.
func validateRequired(body []byte, t reflect.Type) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice {
		var items []json.RawMessage
		if err := json.Unmarshal(body, &items); err != nil {
			return domain.Fail(400, "MALFORMED_JSON", "Expected a JSON array")
		}
		for _, b := range items {
			if err := validateRequired(b, t.Elem()); err != nil {
				return err
			}
		}
		return nil
	}
	if t.Kind() != reflect.Struct || t.PkgPath() == "time" {
		return nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(body, &values); err != nil || values == nil {
		return domain.Fail(400, "MALFORMED_JSON", "Expected a JSON object")
	}
	for _, field := range requiredFields[t.Name()] {
		v, ok := values[field]
		nullableMonthly := (t.Name() == "MonthlyItemInput" || t.Name() == "MonthlyItemUpdate") && (field == "amount_minor" || field == "usual_amount_minor") ||
			t.Name() == "MonthlyDraftEdit" && field == "amount_minor"
		if !ok || string(v) == "null" && !nullableMonthly {
			return domain.Fail(400, "MALFORMED_JSON", "Required field is missing or null: "+field)
		}
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" {
			continue
		}
		if b, ok := values[name]; ok {
			if string(b) == "null" {
				if t.Name() == "MonthlyItemInput" && name == "repeat_monthly" {
					return domain.Fail(400, "MALFORMED_JSON", "Field cannot be null: "+name)
				}
				if field.Type.Kind() != reflect.Pointer {
					return domain.Fail(400, "MALFORMED_JSON", "Field cannot be null: "+name)
				}
				continue
			}
			if err := validateRequired(b, field.Type); err != nil {
				return err
			}
		}
	}
	return nil
}
