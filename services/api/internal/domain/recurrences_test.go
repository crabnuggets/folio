package domain

import "testing"

func TestRecurrenceHistoryAndExplicitPlans(t *testing.T) {
	s := NewState()
	r, e := s.CreateRecurrence(RecurrenceInput{Kind: "allocation", ExpectedMinor: 10000, StartMonth: "2026-01", Description: "Savings"})
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.GetPlan("2026-10")
	if e != nil || len(p.Lines) != 0 {
		t.Fatal("template silently mutated plan")
	}
	next, e := s.ReviseRecurrence(r.ID, RecurrenceInput{Kind: "allocation", ExpectedMinor: 20000, StartMonth: "2026-11"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	old := s.Recurrences[r.ID]
	if old.EndMonth == nil || *old.EndMonth != "2026-10" || next.SupersedesID == nil || *next.SupersedesID != r.ID {
		t.Fatal(old, next)
	}
	if _, e = s.PutPlan("2026-11", PlanInput{Lines: []PlanLine{{ID: NewID(), Kind: "allocation", ExpectedMinor: 10000, RecurrenceID: &r.ID}}}, 1); e == nil {
		t.Fatal("expired recurrence materialized")
	}
	if _, e = s.PutPlan("2026-11", PlanInput{Lines: []PlanLine{{ID: NewID(), Kind: "allocation", ExpectedMinor: 20000, RecurrenceID: &next.ID}}}, 1); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ReviseRecurrence(next.ID, RecurrenceInput{Kind: "allocation", ExpectedMinor: 30000, StartMonth: "2026-11"}, 1); e == nil {
		t.Fatal("overlapping revision")
	}
}
func TestRecurrenceCannotRestateMaterializedOrClosedHistory(t *testing.T) {
	s := NewState()
	r, e := s.CreateRecurrence(RecurrenceInput{Kind: "income", ExpectedMinor: 500000, StartMonth: "2026-01"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.PutPlan("2026-11", PlanInput{Lines: []PlanLine{{ID: NewID(), Kind: "income", ExpectedMinor: 500000, RecurrenceID: &r.ID}}}, 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ReviseRecurrence(r.ID, RecurrenceInput{Kind: "income", ExpectedMinor: 600000, StartMonth: "2026-10"}, 1); e == nil {
		t.Fatal("restated materialized future")
	}
	s.Months["2026-10"] = Month{Month: "2026-10", State: "closed", Version: 2}
	if _, e = s.CreateRecurrence(RecurrenceInput{Kind: "income", ExpectedMinor: 100, StartMonth: "2026-01"}); e == nil {
		t.Fatal("restated closed expectations")
	}
}
