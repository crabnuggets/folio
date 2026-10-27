package domain

import "testing"

func TestDraftStagingPreservesClosedMonthAndSourceIdentity(t *testing.T) {
	s := NewState()
	s.Months["2026-10"] = Month{Month: "2026-10", State: "closed", Version: 2, Revision: 9}
	in := DraftInput{SourceID: "manual", ExternalID: "synthetic-1", Evidence: "Synthetic manual candidate", Entry: &EntryInput{Kind: "income", AmountMinor: 500000, Currency: "SGD", EffectiveDate: "2026-10-25"}}
	draft, err := s.CreateDraft(in)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := s.CreateDraft(in)
	if err != nil || repeated.ID != draft.ID || len(s.Drafts) != 1 {
		t.Fatalf("source duplicated: %#v %v", repeated, err)
	}
	if len(s.Entries) != 0 || s.Months["2026-10"].Revision != 9 {
		t.Fatal("staging changed financial data")
	}
	if _, err := s.AcceptDraft(draft.ID, 1); err == nil {
		t.Fatal("accepted closed-month candidate")
	}
	if s.Drafts[draft.ID].State != "pending" || len(s.Entries) != 0 {
		t.Fatal("failed accept changed draft")
	}
	in.Evidence = "Different candidate"
	if _, err := s.CreateDraft(in); err == nil {
		t.Fatal("source conflict accepted")
	}
}
func TestDraftAcceptanceUsesPostingValidationAndFinalState(t *testing.T) {
	s := NewState()
	account, err := s.CreateAccount(AccountInput{Alias: "Synthetic cash", Kind: "cash", Currency: "SGD", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	in := DraftInput{SourceID: "manual", ExternalID: "synthetic-2", Evidence: "Synthetic", Entry: &EntryInput{Kind: "income", AmountMinor: 500000, Currency: "SGD", EffectiveDate: "2026-10-25", DestinationAccountID: &account.ID}}
	d, err := s.CreateDraft(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.AcceptDraft(d.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "accepted" || out.AcceptedRecordID == nil || len(s.Entries) != 1 {
		t.Fatalf("bad accepted draft %#v", out)
	}
	if _, err = s.AcceptDraft(d.ID, out.Version); err == nil {
		t.Fatal("accepted final draft twice")
	}
	if _, err = s.RejectDraft(d.ID, "not needed", out.Version); err == nil {
		t.Fatal("rejected accepted draft")
	}
	d, err = s.CreateDraft(DraftInput{SourceID: "manual", ExternalID: "synthetic-3", Evidence: "Synthetic", Entry: in.Entry})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RejectDraft(d.ID, "", d.Version); err == nil {
		t.Fatal("empty reason accepted")
	}
	if _, err = s.RejectDraft(d.ID, "Duplicate candidate", d.Version); err != nil {
		t.Fatal(err)
	}
}
