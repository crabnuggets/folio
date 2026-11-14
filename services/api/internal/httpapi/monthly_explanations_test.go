package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

func TestMonthlyExplanationsLifecycleAndExport(t *testing.T) {
	h := newHarness(t)
	amount, repeat := domain.Money(128000), true
	opened := h.request(t, http.MethodPost, monthlyPath+"/open", map[string]any{}, domain.NewID(), "")
	created := h.request(t, http.MethodPost, monthlyPath+"/items", domain.MonthlyItemInput{SectionID: cardSection, Name: "Synthetic cashback card", AmountMinor: &amount, RepeatMonthly: &repeat}, domain.NewID(), opened.headers.Get("ETag"))
	item := firstMonthlyItem(t, sheetResponse(t, created))
	path := monthlyPath + "/items/" + item.ID
	update := domain.MonthlyItemUpdate{SectionID: cardSection, Name: item.Name, AmountMinor: &amount, RepeatMonthly: true, Explanations: []domain.MonthlyExplanation{{Name: "Groceries", AmountMinor: 34280}, {Name: "Dining", AmountMinor: 18550}}}
	key := domain.NewID()
	saved := h.request(t, http.MethodPut, path, update, key, created.headers.Get("ETag"))
	sheet := sheetResponse(t, saved)
	item = firstMonthlyItem(t, sheet)
	if len(item.Explanations) != 2 || item.UnexplainedMinor == nil || *item.UnexplainedMinor != 75170 || sheet.MoneyOutMinor != amount {
		t.Fatalf("incorrect partial explanation: %+v", item)
	}
	replayed := h.request(t, http.MethodPut, path, update, key, created.headers.Get("ETag"))
	if string(replayed.body) != string(saved.body) {
		t.Fatal("explanation retry changed result")
	}
	expect(t, h.request(t, http.MethodGet, "/api/v1/exports", nil, "", ""), 200, "Export")
	update.Explanations = nil // Older clients preserve saved explanations.
	update.Note = "Updated note"
	preserved := h.request(t, http.MethodPut, path, update, domain.NewID(), saved.headers.Get("ETag"))
	if len(firstMonthlyItem(t, sheetResponse(t, preserved)).Explanations) != 2 {
		t.Fatal("omission discarded explanations")
	}
	// Export includes explanations in the retained Undo before-image as well.
	expect(t, h.request(t, http.MethodGet, "/api/v1/exports", nil, "", ""), 200, "Export")
	update.Explanations = []domain.MonthlyExplanation{}
	cleared := h.request(t, http.MethodPut, path, update, domain.NewID(), preserved.headers.Get("ETag"))
	sheet = sheetResponse(t, cleared)
	item = firstMonthlyItem(t, sheet)
	if len(item.Explanations) != 0 || item.UnexplainedMinor != nil {
		t.Fatal("explicit empty explanations did not clear them")
	}
	undone := h.request(t, http.MethodPost, monthlyPath+"/undo", domain.MonthlyUndoInput{UndoToken: *sheet.UndoToken}, domain.NewID(), cleared.headers.Get("ETag"))
	if len(firstMonthlyItem(t, sheetResponse(t, undone)).Explanations) != 2 {
		t.Fatal("Undo lost explanations")
	}
	next := sheetResponse(t, h.request(t, http.MethodPost, "/api/v1/monthly-sheets/2026-11/open", map[string]any{}, domain.NewID(), ""))
	if item := firstMonthlyItem(t, next); len(item.Explanations) != 0 || item.AmountMinor != nil || item.UnexplainedMinor != nil {
		t.Fatal("recurrence copied this month's explanations")
	}
}

func TestMonthlyExplanationsValidationAndRollback(t *testing.T) {
	h := newHarness(t)
	amount := domain.Money(100)
	opened := h.request(t, http.MethodPost, monthlyPath+"/open", map[string]any{}, domain.NewID(), "")
	created := h.request(t, http.MethodPost, monthlyPath+"/items", domain.MonthlyItemInput{SectionID: cardSection, Name: "Synthetic card", AmountMinor: &amount}, domain.NewID(), opened.headers.Get("ETag"))
	item := firstMonthlyItem(t, sheetResponse(t, created))
	path := monthlyPath + "/items/" + item.ID
	update := domain.MonthlyItemUpdate{SectionID: cardSection, Name: item.Name, AmountMinor: &amount}
	for _, entries := range [][]domain.MonthlyExplanation{
		{{Name: "Too much", AmountMinor: 101}},
		{{Name: "First", AmountMinor: 60}, {Name: "Second", AmountMinor: 41}},
		{{Name: "Overflow", AmountMinor: 9223372036854775807}, {Name: "More", AmountMinor: 1}},
		{{Name: "Zero", AmountMinor: 0}},
		{{Name: "Negative", AmountMinor: -1}},
		{{Name: "  ", AmountMinor: 1}},
	} {
		update.Explanations = entries
		assertError(t, h.request(t, http.MethodPut, path, update, domain.NewID(), created.headers.Get("ETag")), 422, "VALIDATION_FAILED")
	}
	update.Explanations = []domain.MonthlyExplanation{{Name: "Fully explained", AmountMinor: 100}}
	full := h.request(t, http.MethodPut, path, update, domain.NewID(), created.headers.Get("ETag"))
	if item := firstMonthlyItem(t, sheetResponse(t, full)); item.UnexplainedMinor != nil {
		t.Fatal("fully explained item has unexplained balance")
	}
	for _, parent := range []*domain.Money{nil, new(domain.Money(99))} {
		update.AmountMinor = parent
		assertError(t, h.request(t, http.MethodPut, path, update, domain.NewID(), full.headers.Get("ETag")), 422, "VALIDATION_FAILED")
	}
	read := sheetResponse(t, h.request(t, http.MethodGet, monthlyPath, nil, "", ""))
	if read.Version != sheetResponse(t, full).Version || read.MoneyOutMinor != amount {
		t.Fatal("invalid parent update committed")
	}
	update.AmountMinor = &amount
	assertError(t, h.request(t, http.MethodPut, path, map[string]any{
		"section_id": cardSection, "name": item.Name, "amount_minor": "100", "note": "", "repeat_monthly": false, "usual_amount_minor": nil,
		"explanations": []map[string]any{{"name": "Nested", "amount_minor": "1", "explanations": []any{}}},
	}, domain.NewID(), full.headers.Get("ETag")), 400, "MALFORMED_JSON")
}
