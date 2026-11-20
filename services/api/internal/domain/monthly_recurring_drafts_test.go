package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func recurringProposal(s *State, name, external string, amount, usual *Money) MonthlyDraftInput {
	return MonthlyDraftInput{Month: "2026-11", SectionID: monthlyRecurring, Name: name, AmountMinor: amount, Note: "Asked in chat", SourceID: "chat", ExternalID: external, SheetVersion: s.MonthlySheet.Version, RepeatMonthly: true, UsualAmountMinor: usual}
}

func previewItem(t *testing.T, s *State, month, id string) *MonthlyItem {
	t.Helper()
	sheet, err := s.PreviewMonthlySheet(month)
	monthlyCheck(t, err)
	for _, section := range sheet.Sections {
		for _, item := range section.Items {
			if item.ID == id {
				return &item
			}
		}
	}
	return nil
}

func TestAmountProposalKeepsItsStoredFingerprint(t *testing.T) {
	s := NewState()
	d, _, err := s.CreateMonthlyDraft(proposal(s, "legacy"))
	monthlyCheck(t, err)
	data, _ := json.Marshal([]string{"2026-11", monthlyCards, "sample card", "", "1250"})
	hash := sha256.Sum256(data)
	if d.ProposalHash != hex.EncodeToString(hash[:]) {
		t.Fatal("amount proposals no longer match proposals stored before recurrence")
	}
}

func TestRecurringItemDraftAddsABlankRecurringItem(t *testing.T) {
	s := NewState()
	for _, bad := range []func(*MonthlyDraftInput){
		func(in *MonthlyDraftInput) { in.RepeatMonthly = false; in.UsualAmountMinor = nil },
		func(in *MonthlyDraftInput) { in.RepeatMonthly = false },
		func(in *MonthlyDraftInput) { in.UsualAmountMinor = monthlyAmount(-1) },
	} {
		in := recurringProposal(s, "Streaming", "bad", nil, monthlyAmount(1598))
		bad(&in)
		_, _, err := s.CreateMonthlyDraft(in)
		aeErrorCode(t, err, "VALIDATION_FAILED")
	}
	in := recurringProposal(s, "Streaming", "streaming-2026-11", nil, monthlyAmount(1598))
	d, _, err := s.CreateMonthlyDraft(in)
	monthlyCheck(t, err)
	if d.AmountMinor != nil || !d.RepeatMonthly || *d.UsualAmountMinor != 1598 || len(s.MonthlySheet.Items) != 0 {
		t.Fatal("recurring proposal not staged as proposed")
	}
	changed := in
	changed.UsualAmountMinor = monthlyAmount(1798)
	_, _, err = s.CreateMonthlyDraft(changed)
	aeErrorCode(t, err, "SOURCE_CONFLICT")
	d, err = s.EditMonthlyDraft(d.ID, MonthlyDraftEdit{SectionID: monthlyRecurring, Name: "Streaming service", Note: "Owner rename"}, d.Version)
	monthlyCheck(t, err)
	if !d.RepeatMonthly || d.AmountMinor != nil {
		t.Fatal("edit dropped the recurrence")
	}
	sheet, err := s.AcceptMonthlyDraft(d.ID, d.Version, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if sheet.MoneyOutMinor != 0 || sheet.MissingCount != 1 || sheet.UndoToken == nil {
		t.Fatal("accepted recurring item should be blank")
	}
	id := *s.MonthlyDrafts[d.ID].AcceptedItemID
	if p := s.MonthlySheet.Items[id].preference("2026-11"); !p.RepeatMonthly || *p.UsualAmountMinor != 1598 || p.SectionID != monthlyRecurring {
		t.Fatalf("recurrence not applied: %+v", p)
	}
	next := previewItem(t, s, "2027-01", id)
	if next == nil || next.AmountMinor != nil || next.Name != "Streaming service" || *next.RecurrenceSince != "2026-11" {
		t.Fatalf("recurring item missing from an unopened month: %+v", next)
	}
	if opened, _ := s.GetMonthlySheet("2027-01"); opened.MissingCount != 0 || s.MonthlySheet.Rows["2027-01"] != nil {
		t.Fatal("preview stored rows")
	}

	// Recurrence on an existing name is a recurring item change, never a draft.
	_, _, err = s.CreateMonthlyDraft(recurringProposal(s, "streaming service", "again", monthlyAmount(1598), nil))
	if err == nil || !strings.Contains(err.Error(), "propose_recurring_item_change") {
		t.Fatalf("existing item accepted a recurrence draft: %v", err)
	}
	gym, _, err := s.CreateMonthlyDraft(recurringProposal(s, "Gym", "gym-2026-11", monthlyAmount(5000), nil))
	monthlyCheck(t, err)
	monthlyAdd(t, s, "2026-11", monthlyRecurring, "Gym", nil, false, nil)
	_, err = s.AcceptMonthlyDraft(gym.ID, gym.Version, s.MonthlySheet.Version)
	aeErrorCode(t, err, "ITEM_EXISTS")
	plain, _, err := s.CreateMonthlyDraft(proposal(s, "plain"))
	monthlyCheck(t, err)
	_, err = s.EditMonthlyDraft(plain.ID, MonthlyDraftEdit{SectionID: plain.SectionID, Name: plain.Name, Note: plain.Note}, plain.Version)
	aeErrorCode(t, err, "VALIDATION_FAILED")
}

func TestRecurringItemChangeLifecycle(t *testing.T) {
	s := NewState()
	id := monthlyAdd(t, s, "2026-11", monthlyRecurring, "Streaming", monthlyAmount(1598), true, monthlyAmount(1598))
	_, err := s.OpenMonthlySheet("2026-12")
	monthlyCheck(t, err)
	change := func(month, section string, repeat bool, usual *Money) MonthlyRecurrenceInput {
		return MonthlyRecurrenceInput{Month: month, ItemID: id, SectionID: section, RepeatMonthly: repeat, UsualAmountMinor: usual, Note: "Price rise", SheetVersion: s.MonthlySheet.Version}
	}
	for _, tc := range []struct {
		in   MonthlyRecurrenceInput
		code string
	}{
		{change("2026-12", monthlyRecurring, true, monthlyAmount(1598)), "VALIDATION_FAILED"},
		{change("2026-12", monthlyIncome, true, monthlyAmount(1798)), "VALIDATION_FAILED"},
		{change("2026-12", monthlyRecurring, false, monthlyAmount(1798)), "VALIDATION_FAILED"},
		{change("2026-10", monthlyRecurring, true, monthlyAmount(1798)), "VALIDATION_FAILED"},
		{change("2026-12", "00000000-0000-4000-8000-000000000099", true, nil), "NOT_FOUND"},
		{MonthlyRecurrenceInput{Month: "2026-12", ItemID: NewID(), SectionID: monthlyRecurring, SheetVersion: s.MonthlySheet.Version}, "NOT_FOUND"},
	} {
		_, _, err := s.CreateMonthlyRecurrenceChange(tc.in)
		aeErrorCode(t, err, tc.code)
	}
	version := s.MonthlySheet.Version
	d, existing, err := s.CreateMonthlyRecurrenceChange(change("2026-12", monthlyRecurring, true, monthlyAmount(1798)))
	monthlyCheck(t, err)
	if existing || d.AmountMinor != nil || d.Name != "Streaming" || d.SourceID != "" || d.RecurrenceChange.ItemID != id || *d.RecurrenceChange.BaseUsualAmountMinor != 1598 || s.MonthlySheet.Version != version {
		t.Fatalf("recurring item change not staged: %+v", d)
	}
	reworded := change("2026-12", monthlyRecurring, true, monthlyAmount(1798))
	reworded.Note = "Same change asked again"
	again, existing, err := s.CreateMonthlyRecurrenceChange(reworded)
	monthlyCheck(t, err)
	if !existing || again.ID != d.ID {
		t.Fatal("same change not reported as already proposed")
	}
	_, _, err = s.CreateMonthlyRecurrenceChange(change("2026-12", monthlyRecurring, true, monthlyAmount(1898)))
	aeErrorCode(t, err, "PROPOSAL_PENDING")
	if !strings.Contains(err.Error(), d.ID) {
		t.Fatal("pending conflict does not name the proposal")
	}
	_, err = s.EditMonthlyDraft(d.ID, MonthlyDraftEdit{SectionID: monthlyRecurring, Name: "Streaming", Note: "x"}, d.Version)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	_, err = s.AcceptMonthlyDraft(d.ID, d.Version, version-1)
	aeErrorCode(t, err, "STALE_VERSION")

	sheet, err := s.AcceptMonthlyDraft(d.ID, d.Version, version)
	monthlyCheck(t, err)
	item := s.MonthlySheet.Items[id]
	if *item.preference("2026-11").UsualAmountMinor != 1598 || *item.preference("2026-12").UsualAmountMinor != 1798 || sheet.MoneyOutMinor != 0 || sheet.UndoToken == nil {
		t.Fatal("change did not apply from its month only")
	}
	if accepted := s.MonthlyDrafts[d.ID]; accepted.State != "accepted" || *accepted.AcceptedItemID != id {
		t.Fatal("proposal not marked accepted")
	}
	sheet, err = s.UndoMonthlySheet("2026-12", MonthlyUndoInput{*sheet.UndoToken}, sheet.Version)
	monthlyCheck(t, err)
	if *s.MonthlySheet.Items[id].preference("2026-12").UsualAmountMinor != 1598 {
		t.Fatal("Undo did not restore the usual amount")
	}

	// A rejected change can be proposed again; an owner edit makes a pending one stale.
	rejected, _, err := s.CreateMonthlyRecurrenceChange(change("2026-12", monthlyOther, true, monthlyAmount(1598)))
	monthlyCheck(t, err)
	_, err = s.RejectMonthlyDraft(rejected.ID, rejected.Version)
	monthlyCheck(t, err)
	moved, existing, err := s.CreateMonthlyRecurrenceChange(change("2026-12", monthlyOther, true, monthlyAmount(1598)))
	monthlyCheck(t, err)
	if existing || moved.ID == rejected.ID {
		t.Fatal("rejected change blocked a new proposal")
	}
	update := monthlyUpdate(t, s, "2026-12", id)
	update.UsualAmountMinor = monthlyAmount(1700)
	_, err = s.UpdateMonthlyItem("2026-12", id, update, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	_, err = s.AcceptMonthlyDraft(moved.ID, moved.Version, s.MonthlySheet.Version)
	aeErrorCode(t, err, "ITEM_CHANGED")
	_, err = s.RejectMonthlyDraft(moved.ID, moved.Version)
	monthlyCheck(t, err)

	// Moving and stopping take the month's untouched blank row along; entered rows stay.
	_, err = s.OpenMonthlySheet("2027-01")
	monthlyCheck(t, err)
	moved, _, err = s.CreateMonthlyRecurrenceChange(change("2027-01", monthlyOther, true, monthlyAmount(1700)))
	monthlyCheck(t, err)
	_, err = s.AcceptMonthlyDraft(moved.ID, moved.Version, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if row := s.MonthlySheet.Rows["2027-01"][id]; row.SectionID != monthlyOther || s.MonthlySheet.Rows["2026-12"][id].SectionID != monthlyRecurring {
		t.Fatal("section move did not start in its month")
	}
	stop, _, err := s.CreateMonthlyRecurrenceChange(change("2027-01", monthlyOther, false, nil))
	monthlyCheck(t, err)
	_, err = s.AcceptMonthlyDraft(stop.ID, stop.Version, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	if _, kept := s.MonthlySheet.Rows["2027-01"][id]; kept || previewItem(t, s, "2027-03", id) != nil {
		t.Fatal("stopped item still appears")
	}
	if previewItem(t, s, "2026-11", id) == nil || previewItem(t, s, "2026-12", id) == nil {
		t.Fatal("stopping changed earlier months")
	}
}

func TestPendingRecurringItemChangeIsFoundAfterTheOwnerMatchesIt(t *testing.T) {
	s := NewState()
	id := monthlyAdd(t, s, "2026-11", monthlyRecurring, "Streaming", monthlyAmount(1598), true, monthlyAmount(1598))
	in := MonthlyRecurrenceInput{Month: "2026-11", ItemID: id, SectionID: monthlyRecurring, RepeatMonthly: true, UsualAmountMinor: monthlyAmount(1798), SheetVersion: s.MonthlySheet.Version}
	d, _, err := s.CreateMonthlyRecurrenceChange(in)
	monthlyCheck(t, err)
	update := monthlyUpdate(t, s, "2026-11", id)
	update.UsualAmountMinor = monthlyAmount(1798)
	_, err = s.UpdateMonthlyItem("2026-11", id, update, s.MonthlySheet.Version)
	monthlyCheck(t, err)
	again, existing, err := s.CreateMonthlyRecurrenceChange(in)
	monthlyCheck(t, err)
	if !existing || again.ID != d.ID {
		t.Fatal("retry did not report the pending proposal")
	}
}
