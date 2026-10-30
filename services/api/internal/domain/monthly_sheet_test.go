package domain

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

const monthlyIncome = "00000000-0000-4000-8000-000000000001"
const monthlyRecurring = "00000000-0000-4000-8000-000000000003"
const monthlyCards = "00000000-0000-4000-8000-000000000004"
const monthlyOther = "00000000-0000-4000-8000-000000000005"

func monthlyAmount(n Money) *Money { return &n }
func monthlyBool(b bool) *bool     { return &b }
func monthlyCheck(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func monthlyAdd(t *testing.T, s *State, month, section, name string, amount *Money, repeat bool, usual *Money) string {
	t.Helper()
	_, err := s.AddMonthlyItem(month, MonthlyItemInput{SectionID: section, Name: name, AmountMinor: amount, RepeatMonthly: monthlyBool(repeat), UsualAmountMinor: usual}, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	for id, item := range s.MonthlySheet.Items {
		if item.Name == name {
			return id
		}
	}
	t.Fatal("missing created item")
	return ""
}
func monthlyUpdate(t *testing.T, s *State, month, id string) MonthlyItemUpdate {
	t.Helper()
	row := s.MonthlySheet.Rows[month][id]
	item := s.MonthlySheet.Items[id]
	p := item.preference(month)
	return MonthlyItemUpdate{row.SectionID, item.Name, row.AmountMinor, row.Note, p.RepeatMonthly, p.UsualAmountMinor}
}

func TestMonthlySheetReportsCurrencyLockAcrossMonths(t *testing.T) {
	for _, setup := range []struct {
		name   string
		locked bool
		apply  func(*State)
	}{
		{"empty", false, func(s *State) {}},
		{"blank", false, func(s *State) { monthlyAdd(t, s, "2026-10", monthlyIncome, "Blank", nil, false, nil) }},
		{"historical zero", true, func(s *State) { monthlyAdd(t, s, "2026-10", monthlyIncome, "Zero", monthlyAmount(0), false, nil) }},
		{"removed", true, func(s *State) {
			id := monthlyAdd(t, s, "2026-10", monthlyIncome, "Removed", monthlyAmount(1), false, nil)
			_, err := s.RemoveMonthlyItem("2026-10", id, true, s.MonthlySheet.Version)
			monthlyCheck(t, err)
		}},
		{"usual only", true, func(s *State) { monthlyAdd(t, s, "2026-10", monthlyIncome, "Usual", nil, true, monthlyAmount(1)) }},
		{"legacy setup", true, func(s *State) { s.LegacySummaries = append(s.LegacySummaries, LegacySummary{Meta: NewMeta()}) }},
	} {
		t.Run(setup.name, func(t *testing.T) {
			s := NewState()
			setup.apply(s)
			sheet, err := s.OpenMonthlySheet("2026-11")
			monthlyCheck(t, err)
			encoded, err := json.Marshal(sheet)
			monthlyCheck(t, err)
			var fields map[string]json.RawMessage
			monthlyCheck(t, json.Unmarshal(encoded, &fields))
			if string(fields["currency_locked"]) != map[bool]string{true: "true", false: "false"}[setup.locked] {
				t.Fatalf("currency_locked missing or incorrect in later month: %s", encoded)
			}
			_, err = s.UpdateSettings(SettingsInput{"JPY", "UTC"}, s.Settings.Version)
			aeErrorCode(t, err, "VALIDATION_FAILED")
		})
	}
}

func TestMonthlySheetTotalsBlankZeroAndLegacySeparation(t *testing.T) {
	s := NewState()
	// Synthetic amounts; legacy review state must not affect monthly editing.
	s.LegacySummaries = append(s.LegacySummaries, LegacySummary{Meta: NewMeta(), Month: "2026-10", AmountMinor: 999})
	s.Months["2026-10"] = Month{Month: "2026-10", State: "closed"}
	monthlyAdd(t, s, "2026-10", monthlyIncome, "Salary", monthlyAmount(10000), false, nil)
	monthlyAdd(t, s, "2026-10", monthlyCards, "Sample card", monthlyAmount(2100), false, nil)
	monthlyAdd(t, s, "2026-10", monthlyRecurring, "House saving", monthlyAmount(1200), false, nil)
	monthlyAdd(t, s, "2026-10", monthlyOther, "Blank", nil, false, nil)
	monthlyAdd(t, s, "2026-10", monthlyOther, "Zero", monthlyAmount(0), false, nil)
	sheet, err := s.GetMonthlySheet("2026-10")
	monthlyCheck(t, err)
	if sheet.MoneyInMinor != 10000 || sheet.MoneyOutMinor != 3300 || sheet.RemainderMinor != 6700 || sheet.MissingCount != 1 || !sheet.HasLegacyData {
		t.Fatalf("wrong monthly totals: %+v", sheet)
	}
	if sheet.Sections[3].AmountLabel != "Bill amount" || sheet.Sections[3].TotalMinor != 2100 {
		t.Fatal("card amount counted incorrectly")
	}
	if len(s.Entries) != 0 || s.Months["2026-10"].State != "closed" || s.LegacySummaries[0].AmountMinor != 999 {
		t.Fatal("monthly sheet changed legacy data")
	}
	_, err = s.UpdateSettings(SettingsInput{"USD", "Asia/Singapore"}, s.Settings.Version)
	if err == nil {
		t.Fatal("monthly amounts did not lock currency")
	}
}

func TestMonthlyRecurrenceStartSkipStopResumeAndUsual(t *testing.T) {
	s := NewState()
	id := monthlyAdd(t, s, "2026-03", monthlyCards, "Sample card", monthlyAmount(400), true, nil)
	fixed := monthlyAdd(t, s, "2026-03", monthlyIncome, "Salary", monthlyAmount(1000), true, monthlyAmount(1000))
	_, err := s.OpenMonthlySheet("2026-02")
	monthlyCheck(t, err)
	if len(s.MonthlySheet.Rows["2026-02"]) != 0 {
		t.Fatal("generated rows before recurrence start")
	}
	first, err := s.OpenMonthlySheet("2026-04")
	monthlyCheck(t, err)
	if first.MissingCount != 2 || first.MoneyInMinor != 0 || first.MoneyOutMinor != 0 {
		t.Fatal("recurrence posted amounts")
	}
	second, err := s.OpenMonthlySheet("2026-04")
	monthlyCheck(t, err)
	if first.Version != second.Version || len(s.MonthlySheet.Rows["2026-04"]) != 2 {
		t.Fatal("open duplicated or rewrote rows")
	}
	_, err = s.UseMonthlyUsualAmounts("2026-04", MonthlyUsualInput{}, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if *s.MonthlySheet.Rows["2026-04"][fixed].AmountMinor != 1000 || s.MonthlySheet.Rows["2026-04"][id].AmountMinor != nil {
		t.Fatal("usual amount applied to varying card")
	}
	in := monthlyUpdate(t, s, "2026-04", fixed)
	in.AmountMinor = monthlyAmount(777)
	_, err = s.UpdateMonthlyItem("2026-04", fixed, in, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	_, err = s.UseMonthlyUsualAmounts("2026-04", MonthlyUsualInput{}, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if *s.MonthlySheet.Rows["2026-04"][fixed].AmountMinor != 777 {
		t.Fatal("usual overwrote edit")
	}
	_, err = s.RemoveMonthlyItem("2026-04", id, true, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	_, err = s.OpenMonthlySheet("2026-04")
	monthlyCheck(t, err)
	if !s.MonthlySheet.Rows["2026-04"][id].Removed {
		t.Fatal("opening lost skipped month")
	}
	for _, month := range []string{"2026-05", "2026-06", "2026-07"} {
		_, err = s.OpenMonthlySheet(month)
		monthlyCheck(t, err)
	}
	in = monthlyUpdate(t, s, "2026-06", id)
	in.AmountMinor = monthlyAmount(0)
	in.Note = "Known zero"
	_, err = s.UpdateMonthlyItem("2026-06", id, in, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	in = monthlyUpdate(t, s, "2026-03", id)
	in.RepeatMonthly = false
	_, err = s.UpdateMonthlyItem("2026-03", id, in, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if _, ok := s.MonthlySheet.Rows["2026-05"][id]; ok {
		t.Fatal("stop left untouched generated future row")
	}
	if _, ok := s.MonthlySheet.Rows["2026-07"][id]; ok {
		t.Fatal("stop left untouched later row")
	}
	if *s.MonthlySheet.Rows["2026-06"][id].AmountMinor != 0 || !s.MonthlySheet.Rows["2026-04"][id].Removed {
		t.Fatal("stop erased future entry or explicit skip")
	}
	// Resume from an entered future month without backfilling the stopped gap.
	in = monthlyUpdate(t, s, "2026-06", id)
	in.RepeatMonthly = true
	_, err = s.UpdateMonthlyItem("2026-06", id, in, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	_, err = s.OpenMonthlySheet("2026-05")
	monthlyCheck(t, err)
	if _, ok := s.MonthlySheet.Rows["2026-05"][id]; ok {
		t.Fatal("resume backfilled stopped month")
	}
	_, err = s.OpenMonthlySheet("2026-07")
	monthlyCheck(t, err)
	if s.MonthlySheet.Rows["2026-07"][id].AmountMinor != nil {
		t.Fatal("resume copied amount")
	}
}

func TestMonthlyHistoryIdentityRankingAndCollision(t *testing.T) {
	s := NewState()
	card := monthlyAdd(t, s, "2026-01", monthlyCards, "Sample Card", monthlyAmount(500), true, nil)
	monthlyAdd(t, s, "2026-01", monthlyOther, "Sample cost", monthlyAmount(50), false, nil)
	monthlyAdd(t, s, "2026-01", monthlyIncome, "Sample Card", monthlyAmount(20), false, nil)
	suggestions, err := s.SuggestMonthlyItems("2026-02", monthlyCards, " SAMPLE ")
	monthlyCheck(t, err)
	if len(suggestions.Items) != 2 || suggestions.Items[0].ID != card || suggestions.Items[0].UsualAmountMinor != nil || !suggestions.Items[0].RepeatMonthly {
		t.Fatalf("wrong suggestions: %+v", suggestions)
	}
	_, err = s.AddMonthlyItem("2026-02", MonthlyItemInput{SectionID: monthlyOther, Name: " sample   CARD ", AmountMinor: monthlyAmount(300)}, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	row := s.MonthlySheet.Rows["2026-02"][card]
	if row.SectionID != monthlyOther || *row.AmountMinor != 300 || !s.MonthlySheet.Items[card].preference("2026-02").RepeatMonthly {
		t.Fatal("history did not preserve identity/preferences/entered amount")
	}
	version := s.MonthlySheet.Version
	_, err = s.AddMonthlyItem("2026-02", MonthlyItemInput{SectionID: monthlyCards, Name: "SAMPLE CARD", AmountMinor: monthlyAmount(999)}, version)
	monthlyCheck(t, err)
	if len(s.MonthlySheet.Rows["2026-02"]) != 1 || *s.MonthlySheet.Rows["2026-02"][card].AmountMinor != 300 || s.MonthlySheet.Version != version {
		t.Fatal("duplicate history reuse overwrote existing cell")
	}
	if s.MonthlySheet.Rows["2026-01"][card].SectionID != monthlyCards {
		t.Fatal("reuse moved old month's section")
	}
	in := monthlyUpdate(t, s, "2026-02", card)
	in.Name = "Renamed card"
	_, err = s.UpdateMonthlyItem("2026-02", card, in, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	suggestions, err = s.SuggestMonthlyItems("2026-02", monthlyCards, "sample card")
	monthlyCheck(t, err)
	if len(suggestions.Items) != 1 || suggestions.Items[0].Name != "Renamed card" || suggestions.Items[0].CurrentMonthItemID == nil {
		t.Fatal("rename lost historical alias/current identity")
	}
	other := monthlyAdd(t, s, "2026-02", monthlyOther, "Other item", monthlyAmount(1), false, nil)
	in = monthlyUpdate(t, s, "2026-02", other)
	in.Name = " renamed   CARD "
	_, err = s.UpdateMonthlyItem("2026-02", other, in, s.MonthlySheet.Version)
	if err == nil {
		t.Fatal("rename allowed normalized name collision")
	}
}

func TestMonthlyUndoSerializationAndSections(t *testing.T) {
	s := NewState()
	id := monthlyAdd(t, s, "2026-01", monthlyCards, "Sample card", monthlyAmount(500), true, monthlyAmount(500))
	in := monthlyUpdate(t, s, "2026-01", id)
	in.AmountMinor = nil
	cleared, err := s.UpdateMonthlyItem("2026-01", id, in, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if cleared.UndoToken == nil || cleared.MissingCount != 1 {
		t.Fatal("clear lacks undo")
	}
	encoded, err := json.Marshal(s)
	monthlyCheck(t, err)
	restarted := NewState()
	monthlyCheck(t, json.Unmarshal(encoded, restarted))
	if !reflect.DeepEqual(s, restarted) {
		t.Fatal("serialization changed monthly identity/preferences/undo")
	}
	restored, err := restarted.UndoMonthlySheet("2026-01", MonthlyUndoInput{*cleared.UndoToken}, cleared.Version)
	monthlyCheck(t, err)
	if restored.MoneyOutMinor != 500 || restored.UndoToken == nil || restored.Version <= cleared.Version {
		t.Fatal("undo did not restore amount safely")
	}
	_, err = restarted.UndoMonthlySheet("2026-01", MonthlyUndoInput{*cleared.UndoToken}, restored.Version)
	if err == nil {
		t.Fatal("stale undo accepted")
	}
	redone, err := restarted.UndoMonthlySheet("2026-01", MonthlyUndoInput{*restored.UndoToken}, restored.Version)
	monthlyCheck(t, err)
	if redone.MoneyOutMinor != 0 || redone.UndoToken == nil {
		t.Fatal("redo did not restore cleared amount")
	}

	renamed, err := restarted.RenameMonthlySection("2026-01", monthlyCards, MonthlySectionRename{"Cards renamed"}, restarted.MonthlySheet.Version)
	monthlyCheck(t, err)
	if renamed.Sections[3].AmountLabel != "Bill amount" {
		t.Fatal("section rename changed card meaning")
	}
	_, err = restarted.CreateMonthlySection("2026-01", MonthlySectionInput{"New outgoing", "out"}, restarted.MonthlySheet.Version)
	monthlyCheck(t, err)
	if len(restarted.MonthlySheet.Sections) != 6 {
		t.Fatal("missing custom section")
	}
	_, err = restarted.RenameMonthlySection("2026-01", monthlyCards, MonthlySectionRename{" recurring   EXPENSES "}, restarted.MonthlySheet.Version)
	if err == nil {
		t.Fatal("section rename allowed duplicate heading")
	}
}

func TestMonthlyOverflowAndCurrencyLockAfterUndo(t *testing.T) {
	s := NewState()
	monthlyAdd(t, s, "2026-01", monthlyIncome, "Maximum", monthlyAmount(Money(math.MaxInt64)), false, nil)
	_, err := s.AddMonthlyItem("2026-01", MonthlyItemInput{SectionID: monthlyIncome, Name: "Overflow", AmountMinor: monthlyAmount(1)}, s.MonthlySheet.Version)
	if err == nil {
		t.Fatal("overflowing total accepted")
	}
	s = NewState()
	monthlyAdd(t, s, "2026-01", monthlyOther, "Zero", monthlyAmount(0), false, nil)
	u := s.MonthlySheet.Undo
	_, err = s.UndoMonthlySheet("2026-01", MonthlyUndoInput{u.Token}, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if !s.MonthlySheet.CurrencyLocked {
		t.Fatal("undo discarded currency lock for retained audit money")
	}
	_, err = s.UpdateSettings(SettingsInput{"USD", "Asia/Singapore"}, s.Settings.Version)
	if err == nil {
		t.Fatal("changed currency after historic amount")
	}
}

func TestMonthlyUsualChangesAndFutureNotePreservation(t *testing.T) {
	s := NewState()
	id := monthlyAdd(t, s, "2026-01", monthlyRecurring, "Fixed contribution", monthlyAmount(100), true, monthlyAmount(100))
	for _, month := range []string{"2026-02", "2026-03", "2026-04"} {
		_, err := s.OpenMonthlySheet(month)
		monthlyCheck(t, err)
	}
	in := monthlyUpdate(t, s, "2026-03", id)
	in.Note = "Waiting for confirmation"
	_, err := s.UpdateMonthlyItem("2026-03", id, in, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	in = monthlyUpdate(t, s, "2026-02", id)
	in.UsualAmountMinor = monthlyAmount(200)
	_, err = s.UpdateMonthlyItem("2026-02", id, in, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if *s.MonthlySheet.Items[id].preference("2026-01").UsualAmountMinor != 100 || *s.MonthlySheet.Rows["2026-01"][id].AmountMinor != 100 || s.MonthlySheet.Rows["2026-03"][id].AmountMinor != nil {
		t.Fatal("usual preference change rewrote old or blank amounts")
	}
	sectionID := monthlyCards
	_, err = s.UseMonthlyUsualAmounts("2026-02", MonthlyUsualInput{SectionID: &sectionID}, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if s.MonthlySheet.Rows["2026-02"][id].AmountMinor != nil {
		t.Fatal("section-scoped usual action filled another section")
	}
	in = monthlyUpdate(t, s, "2026-02", id)
	in.RepeatMonthly, in.UsualAmountMinor = false, nil
	_, err = s.UpdateMonthlyItem("2026-02", id, in, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if row, ok := s.MonthlySheet.Rows["2026-03"][id]; !ok || row.Note != "Waiting for confirmation" || row.AmountMinor != nil {
		t.Fatal("stop erased an entered future note")
	}
	if _, ok := s.MonthlySheet.Rows["2026-04"][id]; ok {
		t.Fatal("stop kept untouched generated row")
	}
}

func TestMonthlySuggestionsRankSectionFrequencyThenRecency(t *testing.T) {
	s := NewState()
	monthlyAdd(t, s, "2026-01", monthlyCards, "Local card", monthlyAmount(10), false, nil)
	frequent := monthlyAdd(t, s, "2026-01", monthlyOther, "Frequent card", monthlyAmount(10), false, nil)
	recent := monthlyAdd(t, s, "2026-02", monthlyOther, "Recent card", monthlyAmount(10), false, nil)
	monthlyAdd(t, s, "2026-01", monthlyOther, "Older card", monthlyAmount(10), false, nil)
	_, err := s.AddMonthlyItem("2026-03", MonthlyItemInput{SectionID: monthlyOther, ItemID: &frequent, Name: "Frequent card", AmountMinor: monthlyAmount(10)}, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	suggestions, err := s.SuggestMonthlyItems("2026-04", monthlyCards, "card")
	monthlyCheck(t, err)
	if len(suggestions.Items) != 4 || suggestions.Items[0].Name != "Local card" || suggestions.Items[1].ID != frequent || suggestions.Items[2].ID != recent || suggestions.Items[3].Name != "Older card" {
		t.Fatalf("wrong history ranking: %+v", suggestions.Items)
	}
}
