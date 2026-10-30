package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

const monthlyPath = "/api/v1/monthly-sheets/2026-10"
const cardSection = "00000000-0000-4000-8000-000000000004"

func sheetResponse(t *testing.T, response wireResponse) domain.MonthlySheet {
	t.Helper()
	expect(t, response, 200, "MonthlySheet")
	var sheet domain.MonthlySheet
	if err := json.Unmarshal(response.body, &sheet); err != nil {
		t.Fatal(err)
	}
	if response.headers.Get("ETag") != `"`+strconv.FormatInt(sheet.Version, 10)+`"` {
		t.Fatal("sheet version and ETag disagree")
	}
	return sheet
}

func firstMonthlyItem(t *testing.T, sheet domain.MonthlySheet) domain.MonthlyItem {
	t.Helper()
	for _, section := range sheet.Sections {
		if len(section.Items) > 0 {
			return section.Items[0]
		}
	}
	t.Fatal("missing monthly item")
	return domain.MonthlyItem{}
}

func TestMonthlyHTTPContractLifecycleAndRetry(t *testing.T) {
	h := newHarness(t)
	initialSettings := h.request(t, http.MethodGet, "/api/v1/settings", nil, "", "")
	assertError(t, h.request(t, http.MethodPatch, "/api/v1/settings", domain.SettingsInput{Currency: "USD", Timezone: "Asia/Singapore"}, domain.NewID(), initialSettings.headers.Get("ETag")), 422, "VALIDATION_FAILED")
	opened := h.request(t, http.MethodPost, monthlyPath+"/open", map[string]any{}, domain.NewID(), "")
	sheet := sheetResponse(t, opened)
	if len(sheet.Sections) != 5 || sheet.MissingCount != 0 {
		t.Fatal("missing empty suggested sections")
	}
	amount := domain.Money(1234)
	repeat := true
	input := domain.MonthlyItemInput{SectionID: cardSection, Name: "Synthetic Card", AmountMinor: &amount, RepeatMonthly: &repeat}
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	assertSchema(t, "MonthlyItemInput", b)
	key := domain.NewID()
	created := h.request(t, http.MethodPost, monthlyPath+"/items", input, key, opened.headers.Get("ETag"))
	sheet = sheetResponse(t, created)
	item := firstMonthlyItem(t, sheet)
	if sheet.MoneyOutMinor != amount || sheet.UndoToken == nil {
		t.Fatal("card bill did not count once")
	}
	replayed := h.request(t, http.MethodPost, monthlyPath+"/items", input, key, opened.headers.Get("ETag"))
	if string(replayed.body) != string(created.body) || replayed.headers.Get("Idempotency-Replayed") != "true" {
		t.Fatal("lost response retry changed result")
	}
	assertError(t, h.request(t, http.MethodPost, monthlyPath+"/items", input, domain.NewID(), opened.headers.Get("ETag")), 412, "STALE_VERSION")
	assertError(t, h.request(t, http.MethodPost, monthlyPath+"/items", input, domain.NewID(), ""), 428, "PRECONDITION_REQUIRED")
	duplicate := h.request(t, http.MethodPost, monthlyPath+"/items", map[string]any{"section_id": cardSection, "name": " synthetic   CARD ", "amount_minor": "9999"}, domain.NewID(), created.headers.Get("ETag"))
	sheet = sheetResponse(t, duplicate)
	if sheet.MoneyOutMinor != amount || len(sheet.Sections[3].Items) != 1 {
		t.Fatal("normalized duplicate overwrote or duplicated cell")
	}
	suggestions := h.request(t, http.MethodGet, "/api/v1/monthly-item-suggestions?month=2026-10&section_id="+cardSection+"&q=synthetic", nil, "", "")
	got := expect(t, suggestions, 200, "MonthlyItemSuggestions")
	if got["items"].([]any)[0].(map[string]any)["current_month_item_id"] != item.ID {
		t.Fatal("suggestion lost current identity")
	}
	update := domain.MonthlyItemUpdate{SectionID: cardSection, Name: item.Name, AmountMinor: nil, Note: "Blank is unknown", RepeatMonthly: true}
	cleared := h.request(t, http.MethodPut, monthlyPath+"/items/"+item.ID, update, domain.NewID(), duplicate.headers.Get("ETag"))
	sheet = sheetResponse(t, cleared)
	if sheet.MissingCount != 1 || sheet.MoneyOutMinor != 0 || sheet.UndoToken == nil {
		t.Fatal("clear did not leave missing amount")
	}
	undone := h.request(t, http.MethodPost, monthlyPath+"/undo", domain.MonthlyUndoInput{UndoToken: *sheet.UndoToken}, domain.NewID(), cleared.headers.Get("ETag"))
	sheet = sheetResponse(t, undone)
	if sheet.MoneyOutMinor != amount || sheet.UndoToken == nil {
		t.Fatal("undo did not restore original amount")
	}
	removed := h.request(t, http.MethodPost, monthlyPath+"/items/"+item.ID+"/remove", map[string]any{}, domain.NewID(), undone.headers.Get("ETag"))
	sheet = sheetResponse(t, removed)
	if sheet.MoneyOutMinor != 0 || len(sheet.Sections[3].Items) != 0 {
		t.Fatal("remove left bill in total")
	}
	restored := h.request(t, http.MethodPost, monthlyPath+"/items/"+item.ID+"/restore", map[string]any{}, domain.NewID(), removed.headers.Get("ETag"))
	sheet = sheetResponse(t, restored)
	if sheet.MoneyOutMinor != amount {
		t.Fatal("restore lost entered amount")
	}
	nextPath := "/api/v1/monthly-sheets/2026-11"
	next := h.request(t, http.MethodPost, nextPath+"/open", map[string]any{}, domain.NewID(), "")
	sheet = sheetResponse(t, next)
	if sheet.MissingCount != 1 || sheet.MoneyOutMinor != 0 || firstMonthlyItem(t, sheet).ID != item.ID {
		t.Fatal("new month copied amount or lost identity")
	}
	reopened := h.request(t, http.MethodPost, nextPath+"/open", map[string]any{}, domain.NewID(), "")
	if sheetResponse(t, reopened).Version != sheet.Version {
		t.Fatal("reopen changed version without new rows")
	}
	get := h.request(t, http.MethodGet, monthlyPath, nil, "", "")
	if sheetResponse(t, get).MoneyOutMinor != amount {
		t.Fatal("opening month rewrote old amount")
	}
	settings := h.request(t, http.MethodGet, "/api/v1/settings", nil, "", "")
	assertError(t, h.request(t, http.MethodPatch, "/api/v1/settings", domain.SettingsInput{Currency: "USD", Timezone: "Asia/Singapore"}, domain.NewID(), settings.headers.Get("ETag")), 422, "VALIDATION_FAILED")
	export := h.request(t, http.MethodGet, "/api/v1/exports", nil, "", "")
	exported := expect(t, export, 200, "Export")
	state := exported["state"].(map[string]any)
	if state["monthly_sheet"].(map[string]any)["items"].(map[string]any)[item.ID] == nil || state["entries"] == nil {
		t.Fatal("export lost monthly or legacy state")
	}
}

func TestMonthlyHTTPValidationAndOverflowRollback(t *testing.T) {
	h := newHarness(t)
	opened := h.request(t, http.MethodPost, monthlyPath+"/open", map[string]any{}, domain.NewID(), "")
	sheetResponse(t, opened)
	etag := opened.headers.Get("ETag")
	for _, input := range []map[string]any{
		{"section_id": cardSection, "name": "Missing amount"},
		{"section_id": cardSection, "name": "Null repeat", "amount_minor": "1", "repeat_monthly": nil},
		{"section_id": cardSection, "name": "Unknown field", "amount_minor": "1", "account_id": domain.NewID()},
		{"section_id": cardSection, "name": "Numeric money", "amount_minor": 123},
	} {
		assertError(t, h.request(t, http.MethodPost, monthlyPath+"/items", input, domain.NewID(), etag), 400, "MALFORMED_JSON")
	}
	assertError(t, h.request(t, http.MethodPost, monthlyPath+"/items", map[string]any{"section_id": cardSection, "name": "Negative", "amount_minor": "-1"}, domain.NewID(), etag), 422, "VALIDATION_FAILED")
	maximum := h.request(t, http.MethodPost, monthlyPath+"/items", map[string]any{"section_id": cardSection, "name": "Max", "amount_minor": "9223372036854775807"}, domain.NewID(), etag)
	sheet := sheetResponse(t, maximum)
	assertError(t, h.request(t, http.MethodPost, monthlyPath+"/items", map[string]any{"section_id": cardSection, "name": "Overflow", "amount_minor": "1"}, domain.NewID(), maximum.headers.Get("ETag")), 422, "VALIDATION_FAILED")
	read := sheetResponse(t, h.request(t, http.MethodGet, monthlyPath, nil, "", ""))
	if read.Version != sheet.Version || len(read.Sections[3].Items) != 1 || read.MoneyOutMinor != sheet.MoneyOutMinor {
		t.Fatal("overflow command committed partial state")
	}
	assertError(t, h.request(t, http.MethodPost, "/api/v1/monthly-sheets/not-month/open", map[string]any{}, domain.NewID(), ""), 422, "VALIDATION_FAILED")
}

func TestMonthlyHTTPSectionsAndLegacyFlag(t *testing.T) {
	h := newHarness(t)
	createAccount(t, h, "Legacy synthetic bank")
	opened := h.request(t, http.MethodPost, monthlyPath+"/open", map[string]any{}, domain.NewID(), "")
	if !sheetResponse(t, opened).HasLegacyData {
		t.Fatal("legacy data is hidden")
	}
	created := h.request(t, http.MethodPost, monthlyPath+"/sections", domain.MonthlySectionInput{Name: "Custom", Direction: "out"}, domain.NewID(), opened.headers.Get("ETag"))
	sheet := sheetResponse(t, created)
	if len(sheet.Sections) != 6 {
		t.Fatal("custom section missing")
	}
	renamed := h.request(t, http.MethodPut, monthlyPath+"/sections/"+cardSection, domain.MonthlySectionRename{Name: "My cards"}, domain.NewID(), created.headers.Get("ETag"))
	sheet = sheetResponse(t, renamed)
	if sheet.Sections[3].Name != "My cards" || sheet.Sections[3].AmountLabel != "Bill amount" {
		t.Fatal("rename changed bill amount label")
	}
	assertError(t, h.request(t, http.MethodPut, monthlyPath+"/sections/"+cardSection, domain.MonthlySectionRename{Name: "custom"}, domain.NewID(), renamed.headers.Get("ETag")), 409, "SECTION_NAME_EXISTS")
}

func TestMonthlyHTTPHistoryReuseAfterSkip(t *testing.T) {
	h := newHarness(t)
	const recurring = "00000000-0000-4000-8000-000000000003"
	const other = "00000000-0000-4000-8000-000000000005"
	const previous = "/api/v1/monthly-sheets/2026-09"
	opened := h.request(t, http.MethodPost, previous+"/open", map[string]any{}, domain.NewID(), "")
	created := h.request(t, http.MethodPost, previous+"/items", map[string]any{"section_id": recurring, "name": "Synthetic contribution", "amount_minor": "10000", "repeat_monthly": true, "usual_amount_minor": "10000"}, domain.NewID(), opened.headers.Get("ETag"))
	id := firstMonthlyItem(t, sheetResponse(t, created)).ID
	october := h.request(t, http.MethodPost, monthlyPath+"/open", map[string]any{}, domain.NewID(), "")
	if item := firstMonthlyItem(t, sheetResponse(t, october)); item.AmountMinor != nil || item.SectionID != recurring {
		t.Fatal("expected generated blank in recurring section")
	}
	removed := h.request(t, http.MethodPost, monthlyPath+"/items/"+id+"/remove", map[string]any{}, domain.NewID(), october.headers.Get("ETag"))
	if len(sheetResponse(t, removed).Sections[2].Items) != 0 {
		t.Fatal("remove did not skip this month")
	}
	reopened := h.request(t, http.MethodPost, monthlyPath+"/open", map[string]any{}, domain.NewID(), "")
	if len(sheetResponse(t, reopened).Sections[2].Items) != 0 {
		t.Fatal("open recreated skipped row")
	}
	reused := h.request(t, http.MethodPost, monthlyPath+"/items", map[string]any{"section_id": other, "item_id": id, "name": "Synthetic contribution", "amount_minor": "25000", "note": "Entered again"}, domain.NewID(), reopened.headers.Get("ETag"))
	sheet := sheetResponse(t, reused)
	item := firstMonthlyItem(t, sheet)
	if item.ID != id || item.SectionID != other || item.AmountMinor == nil || *item.AmountMinor != 25000 || item.Note != "Entered again" || !item.RepeatMonthly {
		t.Fatalf("history reuse after skip lost entered month fields: %+v", item)
	}
	if len(sheet.Sections[2].Items) != 0 || len(sheet.Sections[4].Items) != 1 || sheet.MoneyOutMinor != 25000 {
		t.Fatal("reused item did not move/count exactly once")
	}
	old := sheetResponse(t, h.request(t, http.MethodGet, previous, nil, "", ""))
	if prior := firstMonthlyItem(t, old); prior.SectionID != recurring || prior.AmountMinor == nil || *prior.AmountMinor != 10000 {
		t.Fatal("reuse changed earlier amount or section")
	}
	if sheet.UndoToken == nil {
		t.Fatal("reuse after skip needs Undo")
	}
	undone := h.request(t, http.MethodPost, monthlyPath+"/undo", domain.MonthlyUndoInput{UndoToken: *sheet.UndoToken}, domain.NewID(), reused.headers.Get("ETag"))
	sheet = sheetResponse(t, undone)
	if sheet.MoneyOutMinor != 0 || len(sheet.Sections[4].Items) != 0 {
		t.Fatal("Undo did not restore skipped month")
	}
	restored := h.request(t, http.MethodPost, monthlyPath+"/items/"+id+"/restore", map[string]any{}, domain.NewID(), undone.headers.Get("ETag"))
	item = firstMonthlyItem(t, sheetResponse(t, restored))
	if item.ID != id || item.SectionID != recurring || item.AmountMinor != nil || item.Note != "" || !item.RepeatMonthly || item.UsualAmountMinor == nil || *item.UsualAmountMinor != 10000 {
		t.Fatal("explicit Restore did not recover the saved skipped fields")
	}
}
