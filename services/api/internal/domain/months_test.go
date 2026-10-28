package domain

import "testing"

func TestMonthLifecycleVersionsAndAssertions(t *testing.T) {
	s := NewState()
	if _, e := s.CloseMonth("2026-10", MonthTransition{}, 1); e == nil {
		t.Fatal("closed without assertions")
	}
	transition := MonthTransition{Checklist: Checklist{IncomeReviewed: true, CardsReviewed: true, ExpensesReviewed: true, AllocationsReviewed: true}}
	closed, e := s.CloseMonth("2026-10", transition, 1)
	if e != nil {
		t.Fatal(e)
	}
	if closed.State != "closed" || closed.Version != 2 || closed.ClosedAt == nil {
		t.Fatal(closed)
	}
	if _, e = s.PutPlan("2026-10", PlanInput{}, 1); e == nil {
		t.Fatal("changed closed plan")
	}
	if _, e = s.ReopenMonth("2026-10", MonthTransition{Reason: "Correction"}, 1); e == nil {
		t.Fatal("stale reopen")
	}
	opened, e := s.ReopenMonth("2026-10", MonthTransition{Reason: "Correction"}, 2)
	if e != nil {
		t.Fatal(e)
	}
	if opened.State != "open" || opened.Revision != closed.Revision+1 || opened.Version != 3 {
		t.Fatal(opened)
	}
	if _, e = s.ReopenMonth("2026-10", MonthTransition{Reason: "Again"}, 3); e == nil {
		t.Fatal("reopened open month")
	}
}
func TestPlanStableMatchesCannotBeRemoved(t *testing.T) {
	s := NewState()
	id := NewID()
	_, e := s.PutPlan("2026-10", PlanInput{Lines: []PlanLine{{ID: id, Kind: "income", ExpectedMinor: 500000}}}, 1)
	if e != nil {
		t.Fatal(e)
	}
	entry := summaryTestEntry(s, "income", 500000, nil, nil)
	entry.PlanLineID = &id
	s.Entries[entry.ID] = entry
	if _, e = s.PutPlan("2026-10", PlanInput{}, 2); e == nil {
		t.Fatal("removed matched identity")
	}
	if _, e = s.PutPlan("2026-10", PlanInput{Lines: []PlanLine{{ID: id, Kind: "spending", ExpectedMinor: 100}}}, 2); e == nil {
		t.Fatal("changed matched meaning")
	}
	if _, e = s.PutPlan("2026-11", PlanInput{Lines: []PlanLine{{ID: id, Kind: "income", ExpectedMinor: 500000}}}, 1); e == nil {
		t.Fatal("reused identity")
	}
	if _, e = s.PutPlan("2026-10", PlanInput{Lines: []PlanLine{{ID: id, Kind: "income", ExpectedMinor: 500000, Complete: true}}}, 2); e != nil {
		t.Fatal(e)
	}
}
func TestMonthExplicitNoStatementAndZeroIncome(t *testing.T) {
	s := NewState()
	a := summaryTestAccount(t, s, "credit_card", false)
	assertions := Checklist{IncomeReviewed: true, CardsReviewed: true, ExpensesReviewed: true, AllocationsReviewed: true, NoStatementReasons: map[string]string{a.ID: "No cycle"}}
	if _, e := s.CloseMonth("2026-10", MonthTransition{Checklist: assertions}, 1); e != nil {
		t.Fatal(e)
	}
	sum, e := s.Summarize("2026-10")
	if e != nil {
		t.Fatal(e)
	}
	if sum.IsProvisional || sum.SavingsRate != nil || sum.MonthlySurplusMinor == nil || *sum.MonthlySurplusMinor != 0 {
		t.Fatal(sum)
	}
}

func TestMonthCloseRejectsInterveningFinancialMutation(t *testing.T) {
	s := NewState()
	before, _ := s.GetMonth("2026-10")
	if _, err := s.PutPlan("2026-10", PlanInput{}, 1); err != nil {
		t.Fatal(err)
	}
	assertions := MonthTransition{Checklist: Checklist{IncomeReviewed: true, CardsReviewed: true, ExpensesReviewed: true, AllocationsReviewed: true}}
	if _, err := s.CloseMonth("2026-10", assertions, before.Version); err == nil {
		t.Fatal("stale close silently accepted intervening financial changes")
	}
	current, _ := s.GetMonth("2026-10")
	if _, err := s.CloseMonth("2026-10", assertions, current.Version); err != nil {
		t.Fatal(err)
	}
}
