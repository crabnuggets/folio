package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func proposal(s *State, external string) MonthlyDraftInput {
	return MonthlyDraftInput{Month: "2026-11", SectionID: monthlyCards, Name: "Sample card", AmountMinor: monthlyAmount(1250), Note: "November bill", Evidence: "Synthetic statement page 1", SourceID: "test:statement", ExternalID: external, SheetVersion: s.MonthlySheet.Version}
}
func TestMonthlyDraftReviewBoundaryRetryAndUndo(t *testing.T) {
	s := NewState()
	in := proposal(s, "one")
	d, _, err := s.CreateMonthlyDraft(in)
	monthlyCheck(t, err)
	sheet, err := s.GetMonthlySheet(in.Month)
	monthlyCheck(t, err)
	if sheet.MoneyOutMinor != 0 || sheet.Version != 1 || len(s.MonthlySheet.Items) != 0 {
		t.Fatal("staging changed the sheet")
	}
	edited, err := s.EditMonthlyDraft(d.ID, MonthlyDraftEdit{SectionID: monthlyRecurring, Name: "Edited item", AmountMinor: monthlyAmount(2000), Note: "Owner correction"}, d.Version)
	monthlyCheck(t, err)
	retry, _, err := s.CreateMonthlyDraft(in)
	monthlyCheck(t, err)
	if retry.ID != d.ID || retry.Name != "Edited item" {
		t.Fatal("source retry lost owner edits")
	}
	changed := in
	changed.AmountMinor = monthlyAmount(1251)
	_, _, err = s.CreateMonthlyDraft(changed)
	aeErrorCode(t, err, "SOURCE_CONFLICT")
	_, err = s.AcceptMonthlyDraft(d.ID, d.Version, 1)
	aeErrorCode(t, err, "STALE_VERSION")
	_, err = s.AddMonthlyItem(in.Month, MonthlyItemInput{SectionID: monthlyIncome, Name: "Salary", AmountMinor: monthlyAmount(10000)}, 1)
	monthlyCheck(t, err)
	_, err = s.AcceptMonthlyDraft(d.ID, edited.Version, 1)
	aeErrorCode(t, err, "STALE_VERSION")
	sheet, err = s.AcceptMonthlyDraft(d.ID, edited.Version, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if sheet.MoneyOutMinor != 2000 || sheet.RemainderMinor != 8000 || sheet.UndoToken == nil {
		t.Fatal("acceptance did not use edited proposal")
	}
	events := len(s.AuditEvents)
	version := sheet.Version
	again, err := s.AcceptMonthlyDraft(d.ID, edited.Version, 1)
	monthlyCheck(t, err)
	if again.Version != version || len(s.AuditEvents) != events {
		t.Fatal("accept retry changed state")
	}
	sheet, err = s.UndoMonthlySheet(in.Month, MonthlyUndoInput{*sheet.UndoToken}, sheet.Version)
	monthlyCheck(t, err)
	if sheet.MoneyOutMinor != 0 {
		t.Fatal("Undo did not remove accepted amount")
	}
	_, err = s.AcceptMonthlyDraft(d.ID, edited.Version, 1)
	monthlyCheck(t, err)
	sheet, _ = s.GetMonthlySheet(in.Month)
	if sheet.MoneyOutMinor != 0 {
		t.Fatal("reviewed source re-added after Undo")
	}
	rejected, _, err := s.CreateMonthlyDraft(proposal(s, "two"))
	monthlyCheck(t, err)
	_, err = s.RejectMonthlyDraft(rejected.ID, rejected.Version)
	monthlyCheck(t, err)
	_, err = s.AcceptMonthlyDraft(rejected.ID, rejected.Version+1, s.MonthlySheet.Version)
	aeErrorCode(t, err, "DRAFT_RESOLVED")
	sheet, _ = s.GetMonthlySheet(in.Month)
	if sheet.MoneyOutMinor != 0 {
		t.Fatal("rejection changed totals")
	}
}

func TestMonthlyDraftFillsRecurringBlankAndRejectsEnteredDuplicate(t *testing.T) {
	s := NewState()
	id := monthlyAdd(t, s, "2026-10", monthlyCards, "Sample card", monthlyAmount(999), true, monthlyAmount(800))
	_, err := s.OpenMonthlySheet("2026-11")
	monthlyCheck(t, err)
	d, _, err := s.CreateMonthlyDraft(proposal(s, "one"))
	monthlyCheck(t, err)
	sheet, err := s.AcceptMonthlyDraft(d.ID, d.Version, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if sheet.MoneyOutMinor != 1250 || len(s.MonthlySheet.Rows["2026-11"]) != 1 || *s.MonthlyDrafts[d.ID].AcceptedItemID != id {
		t.Fatal("did not reuse recurring blank identity")
	}
	if p := s.MonthlySheet.Items[id].preference("2026-11"); !p.RepeatMonthly || *p.UsualAmountMinor != 800 {
		t.Fatal("proposal altered recurrence")
	}
	d, _, err = s.CreateMonthlyDraft(proposal(s, "two"))
	monthlyCheck(t, err)
	_, err = s.AcceptMonthlyDraft(d.ID, d.Version, s.MonthlySheet.Version)
	aeErrorCode(t, err, "ITEM_ALREADY_ENTERED")
	if s.MonthlyDrafts[d.ID].State != "pending" {
		t.Fatal("failed acceptance resolved proposal")
	}
}

func TestMonthlyDraftValidation(t *testing.T) {
	for _, mutate := range []func(*MonthlyDraftInput){
		func(in *MonthlyDraftInput) { in.Month = "2026-13" }, func(in *MonthlyDraftInput) { in.SectionID = NewID() },
		func(in *MonthlyDraftInput) { in.AmountMinor = monthlyAmount(-1) }, func(in *MonthlyDraftInput) { in.Name = "  " }, func(in *MonthlyDraftInput) { in.ExternalID = "" },
	} {
		s := NewState()
		in := proposal(s, "one")
		mutate(&in)
		if _, _, err := s.CreateMonthlyDraft(in); err == nil || len(s.MonthlyDrafts) != 0 {
			t.Fatal("invalid proposal persisted")
		}
	}
	s := NewState()
	in := proposal(s, "one")
	in.SheetVersion = 2
	_, _, err := s.CreateMonthlyDraft(in)
	aeErrorCode(t, err, "STALE_VERSION")
}

func TestStoredOwnerAggregateRetainsRetiredFieldsWithoutLoadingTheirTypes(t *testing.T) {
	s := NewState()
	monthlyAdd(t, s, "2026-11", monthlyIncome, "Salary", monthlyAmount(50000), true, nil)
	data, err := json.Marshal(s)
	monthlyCheck(t, err)
	var fields map[string]json.RawMessage
	monthlyCheck(t, json.Unmarshal(data, &fields))
	fields["entries"] = json.RawMessage(`{"retired":{"amount_minor":123.45,"unknown_shape":true}}`)
	fields["drafts"] = json.RawMessage(`{"retired":{"entry":{"kind":"obsolete"}}}`)
	fields["months"] = json.RawMessage(`{"2026-11":{"state":"closed"}}`)
	before := fields["entries"]
	data, err = json.Marshal(fields)
	monthlyCheck(t, err)
	loaded := NewState()
	monthlyCheck(t, json.Unmarshal(data, loaded))
	sheet, err := loaded.OpenMonthlySheet("2026-12")
	monthlyCheck(t, err)
	if sheet.MoneyInMinor != 0 || sheet.MissingCount != 1 {
		t.Fatal("recurrence or legacy separation broken")
	}
	sheet, err = loaded.GetMonthlySheet("2026-11")
	monthlyCheck(t, err)
	if sheet.MoneyInMinor != 50000 {
		t.Fatal("existing monthly amounts were lost")
	}
	data, err = json.Marshal(loaded)
	monthlyCheck(t, err)
	monthlyCheck(t, json.Unmarshal(data, &fields))
	if !reflect.DeepEqual(fields["entries"], before) {
		t.Fatal("write deleted or converted retired data")
	}
}

func TestMonthlyDraftExplanationsCarryToAcceptedRow(t *testing.T) {
	s := NewState()
	in := proposal(s, "breakdown")
	in.Explanations = []MonthlyExplanation{{Name: " Grocer ", AmountMinor: 700}, {Name: "Cafe", AmountMinor: 300}}
	d, _, err := s.CreateMonthlyDraft(in)
	monthlyCheck(t, err)
	if d.Explanations[0].Name != "Grocer" {
		t.Fatal("explanation names were not normalized")
	}
	over := proposal(s, "over")
	over.Explanations = []MonthlyExplanation{{Name: "Too much", AmountMinor: 1251}}
	_, _, err = s.CreateMonthlyDraft(over)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	income := proposal(s, "income")
	income.SectionID, income.Explanations = monthlyIncome, []MonthlyExplanation{{Name: "Bonus", AmountMinor: 1}}
	_, _, err = s.CreateMonthlyDraft(income)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	_, err = s.EditMonthlyDraft(d.ID, MonthlyDraftEdit{SectionID: d.SectionID, Name: d.Name, AmountMinor: monthlyAmount(900), Note: d.Note}, d.Version)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	sheet, err := s.AcceptMonthlyDraft(d.ID, d.Version, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	var row MonthlyItem
	for _, section := range sheet.Sections {
		for _, item := range section.Items {
			if item.Name == "Sample card" {
				row = item
			}
		}
	}
	if len(row.Explanations) != 2 || row.UnexplainedMinor == nil || *row.UnexplainedMinor != 250 {
		t.Fatalf("accepted row lost its breakdown: %+v", row)
	}
	sheet, err = s.UndoMonthlySheet(in.Month, MonthlyUndoInput{*sheet.UndoToken}, sheet.Version)
	monthlyCheck(t, err)
	if sheet.MoneyOutMinor != 0 {
		t.Fatal("Undo did not remove the accepted amount and breakdown together")
	}
}

func TestMonthlyDraftIdentityIgnoresWordingAndNamesTheExistingProposal(t *testing.T) {
	s := NewState()
	in := proposal(s, "one")
	in.Explanations = []MonthlyExplanation{{Name: "Grocer", AmountMinor: 700}}
	d, existing, err := s.CreateMonthlyDraft(in)
	monthlyCheck(t, err)
	if existing {
		t.Fatal("first proposal reported as existing")
	}
	reworded := in
	reworded.Note, reworded.Evidence, reworded.Name = "Re-read of the same statement", "Page 1 again", " sample  CARD "
	reworded.Explanations = []MonthlyExplanation{{Name: "grocer", AmountMinor: 700}}
	again, existing, err := s.CreateMonthlyDraft(reworded)
	monthlyCheck(t, err)
	if !existing || again.ID != d.ID || len(s.MonthlyDrafts) != 1 {
		t.Fatal("rewording the same proposal created a duplicate")
	}
	_, err = s.RejectMonthlyDraft(d.ID, d.Version)
	monthlyCheck(t, err)
	again, existing, err = s.CreateMonthlyDraft(reworded)
	monthlyCheck(t, err)
	if !existing || again.State != "rejected" {
		t.Fatal("a reviewed source did not report its state")
	}
	changed := in
	changed.AmountMinor = monthlyAmount(1300)
	_, _, err = s.CreateMonthlyDraft(changed)
	aeErrorCode(t, err, "SOURCE_CONFLICT")
	if !strings.Contains(err.Error(), d.ID) || !strings.Contains(err.Error(), "rejected") || !strings.Contains(err.Error(), "1250") {
		t.Fatalf("conflict does not describe the existing proposal: %v", err)
	}
}

func TestMonthlyAmendmentReplacesAmountAndBreakdownOnly(t *testing.T) {
	s := NewState()
	id := monthlyAdd(t, s, "2026-11", monthlyCards, "Sample card", monthlyAmount(1250), true, monthlyAmount(1000))
	sheet, err := s.UpdateMonthlyItem("2026-11", id, MonthlyItemUpdate{SectionID: monthlyCards, Name: "Sample card", AmountMinor: monthlyAmount(1250), Note: "Owner note", RepeatMonthly: true, UsualAmountMinor: monthlyAmount(1000)}, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	in := proposal(s, "amend")
	in.SectionID, in.Name = monthlyIncome, "Ignored"
	in.Explanations = []MonthlyExplanation{{Name: "Grocer", AmountMinor: 700}, {Name: "Cafe", AmountMinor: 300}}
	d, _, err := s.CreateMonthlyAmendment(in, id)
	monthlyCheck(t, err)
	if d.Amendment == nil || d.Amendment.BaseAmountMinor != 1250 || d.Name != "Sample card" || d.SectionID != monthlyCards {
		t.Fatalf("amendment did not record the entered item: %+v", d)
	}
	if sheet, _ = s.GetMonthlySheet("2026-11"); sheet.MoneyOutMinor != 1250 || len(sheet.Sections[3].Items[0].Explanations) != 0 {
		t.Fatal("staging an amendment changed the sheet")
	}
	_, err = s.EditMonthlyDraft(d.ID, MonthlyDraftEdit{SectionID: d.SectionID, Name: "Other card", AmountMinor: d.AmountMinor, Note: d.Note}, d.Version)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	sheet, err = s.AcceptMonthlyDraft(d.ID, d.Version, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	row := s.MonthlySheet.Rows["2026-11"][id]
	if *row.AmountMinor != 1250 || len(row.Explanations) != 2 || row.Note != "Owner note" || *s.MonthlyDrafts[d.ID].AcceptedItemID != id {
		t.Fatalf("amendment applied wrongly: %+v", row)
	}
	if p := s.MonthlySheet.Items[id].preference("2026-11"); !p.RepeatMonthly || *p.UsualAmountMinor != 1000 {
		t.Fatal("amendment altered recurrence")
	}
	sheet, err = s.UndoMonthlySheet("2026-11", MonthlyUndoInput{*sheet.UndoToken}, sheet.Version)
	monthlyCheck(t, err)
	if len(s.MonthlySheet.Rows["2026-11"][id].Explanations) != 0 {
		t.Fatal("Undo kept the amended breakdown")
	}
}

func TestMonthlyAmendmentRefusals(t *testing.T) {
	s := NewState()
	id := monthlyAdd(t, s, "2026-11", monthlyCards, "Sample card", monthlyAmount(1250), false, nil)
	blank := monthlyAdd(t, s, "2026-11", monthlyCards, "Blank card", nil, false, nil)
	_, _, err := s.CreateMonthlyAmendment(proposal(s, "blank"), blank)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	_, _, err = s.CreateMonthlyAmendment(proposal(s, "same"), id)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	_, _, err = s.CreateMonthlyAmendment(proposal(s, "missing"), NewID())
	aeErrorCode(t, err, "NOT_FOUND")
	in := proposal(s, "late")
	in.AmountMinor = monthlyAmount(1300)
	d, _, err := s.CreateMonthlyAmendment(in, id)
	monthlyCheck(t, err)
	_, err = s.UpdateMonthlyItem("2026-11", id, MonthlyItemUpdate{SectionID: monthlyCards, Name: "Sample card", AmountMinor: monthlyAmount(1400)}, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	_, err = s.AcceptMonthlyDraft(d.ID, d.Version, s.MonthlySheet.Version)
	aeErrorCode(t, err, "ITEM_CHANGED")
	if *s.MonthlySheet.Rows["2026-11"][id].AmountMinor != 1400 || s.MonthlyDrafts[d.ID].State != "pending" {
		t.Fatal("refused amendment changed state")
	}
}
