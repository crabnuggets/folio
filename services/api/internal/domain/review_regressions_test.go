package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func closeReviewMonth(t *testing.T, s *State, month string) Month {
	t.Helper()
	m, err := s.GetMonth(month)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.CloseMonth(month, MonthTransition{Checklist: Checklist{IncomeReviewed: true, CardsReviewed: true, ExpensesReviewed: true, AllocationsReviewed: true}}, m.Version)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestClosedCardCoverageSurvivesAccountMaintenance(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new card", true: "archived card"}[existing], func(t *testing.T) {
			s := NewState()
			var card Account
			if existing {
				card = summaryTestAccount(t, s, "credit_card", false)
			}
			closed := closeReviewMonth(t, s, "2026-10")
			before, err := s.Summarize("2026-10")
			if err != nil {
				t.Fatal(err)
			}
			if existing {
				in := card.AccountInput
				in.Active = false
				if _, err = s.UpdateAccount(card.ID, in, card.Version); err != nil {
					t.Fatal(err)
				}
			} else {
				summaryTestAccount(t, s, "credit_card", false)
			}
			after, err := s.Summarize("2026-10")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("account maintenance restated a closed review:\nbefore %+v\nafter %+v", before, after)
			}
			if _, err = s.ReopenMonth("2026-10", MonthTransition{Reason: "Review account coverage"}, closed.Version); err != nil {
				t.Fatal(err)
			}
			opened, err := s.Summarize("2026-10")
			if err != nil {
				t.Fatal(err)
			}
			if opened.SpendingComplete == before.SpendingComplete {
				t.Fatal("explicit reopen did not use current card inventory")
			}
		})
	}
}

func TestPlanAccountRestrictionsProtectCardFunding(t *testing.T) {
	s := summaryTestState(t)
	bank := summaryTestAccount(t, s, "bank", true)
	card := summaryTestAccount(t, s, "credit_card", false)
	otherCard := summaryTestAccount(t, s, "credit_card", false)
	investment := summaryTestAccount(t, s, "investment", false)
	id := NewID()
	if _, err := s.PutPlan("2026-10", PlanInput{Lines: []PlanLine{{ID: id, Kind: "funding_outflow", ExpectedMinor: 10000, AccountID: &card.ID}}}, 1); err != nil {
		t.Fatal(err)
	}
	allocation := EntryInput{Kind: "allocation", AmountMinor: 10000, Currency: "SGD", EffectiveDate: "2026-10-10", SourceAccountID: &bank.ID, DestinationAccountID: &investment.ID, FundingPlanLineID: &id}
	if _, err := s.CreateEntry(allocation); err == nil {
		t.Fatal("investment allocation consumed card payment plan")
	}
	payment := allocation
	payment.Kind = "card_settlement"
	payment.DestinationAccountID = &otherCard.ID
	if _, err := s.CreateEntry(payment); err == nil {
		t.Fatal("other card consumed payment plan")
	}
	payment.DestinationAccountID = &card.ID
	if _, err := s.CreateEntry(payment); err != nil {
		t.Fatal(err)
	}
	spendingID := NewID()
	plan, _ := s.GetPlan("2026-10")
	plan.Lines = append(plan.Lines, PlanLine{ID: spendingID, Kind: "spending", AccountID: &otherCard.ID})
	if _, err := s.PutPlan("2026-10", PlanInput{Lines: plan.Lines}, plan.Version); err != nil {
		t.Fatal(err)
	}
	in := scStatementInput()
	in.AccountID = card.ID
	in.PlanLineID = &spendingID
	if _, err := s.CreateStatement(in); err == nil {
		t.Fatal("statement consumed another card spending plan")
	}
}

func TestSignedPendingPlanRemainsProvisional(t *testing.T) {
	for _, kind := range []string{"spending", "allocation", "income", "funding_inflow", "funding_outflow"} {
		t.Run(kind, func(t *testing.T) {
			s := summaryTestState(t)
			negative := Money(-10000)
			id := NewID()
			if _, err := s.PutPlan("2026-10", PlanInput{Lines: []PlanLine{{ID: id, Kind: kind, ExpectedMinor: negative, EstimatedRemainingMinor: &negative}}}, 1); err != nil {
				t.Fatal(err)
			}
			out, err := s.Summarize("2026-10")
			if err != nil {
				t.Fatal(err)
			}
			if !out.IsProvisional {
				t.Fatal("uncompleted signed remainder advertised complete")
			}
			if kind == "spending" || kind == "allocation" || kind == "income" {
				if out.SpendingComplete || out.MonthlySurplusMinor != nil {
					t.Fatal(out)
				}
			} else if out.FundingComplete {
				t.Fatal(out)
			}
			found := false
			for _, v := range out.MissingItems {
				found = found || v == "planned_item_pending:"+id
			}
			if !found {
				t.Fatal(out.MissingItems)
			}
		})
	}
}

func TestStatementDueMonthLifecycleProtectsEveryMutation(t *testing.T) {
	for _, op := range []string{"create", "revise", "carry", "settle", "credit"} {
		t.Run(op, func(t *testing.T) {
			s := scState()
			in := scStatementInput()
			in.ReportingMonth = "2026-11"
			in.ReportingMonthReason = "Synthetic spending override"
			var st Statement
			var err error
			if op != "create" {
				st, err = s.CreateStatement(in)
				if err != nil {
					t.Fatal(err)
				}
			}
			payment := scPayment(s, "payment", "2026-11-02", 1000)
			closeReviewMonth(t, s, "2026-10")
			before, _ := json.Marshal(s)
			switch op {
			case "create":
				_, err = s.CreateStatement(in)
			case "revise":
				_, err = s.ReviseStatement(st.ID, StatementRevision{Reason: "Correct due date", Replacement: in}, st.Version)
			case "carry":
				next := in
				next.CycleStart = "2026-10-01"
				next.CycleEnd = "2026-10-31"
				next.IssueDate = "2026-11-01"
				next.DueDate = "2026-11-20"
				next.CarriesStatementID = &st.ID
				_, err = s.CreateStatement(next)
			case "settle":
				_, err = s.CreateSettlement(st.ID, SettlementInput{EntryID: payment.ID, AmountMinor: 1000}, st.Version)
			case "credit":
				_, err = s.CreateObligationAdjustment(st.ID, ObligationAdjustmentInput{AmountMinor: 1000, EffectiveDate: "2026-11-03", Reason: "Synthetic credit"}, st.Version)
			}
			scCode(t, err, "MONTH_CLOSED")
			after, _ := json.Marshal(s)
			if string(before) != string(after) {
				t.Fatal("rejected due-month mutation changed aggregate")
			}
		})
	}
}
func TestStatementDueAndReportingRevisionsAdvance(t *testing.T) {
	s := scState()
	in := scStatementInput()
	in.ReportingMonth = "2026-11"
	in.ReportingMonthReason = "Synthetic override"
	st, err := s.CreateStatement(in)
	if err != nil {
		t.Fatal(err)
	}
	oct, _ := s.GetMonth("2026-10")
	nov, _ := s.GetMonth("2026-11")
	if oct.Revision == 0 || nov.Revision == 0 {
		t.Fatal("creation omitted affected month revision")
	}
	changed := in
	changed.DueDate = "2026-12-20"
	if _, err = s.ReviseStatement(st.ID, StatementRevision{Reason: "Correct due date", Replacement: changed}, st.Version); err != nil {
		t.Fatal(err)
	}
	afterOct, _ := s.GetMonth("2026-10")
	afterNov, _ := s.GetMonth("2026-11")
	dec, _ := s.GetMonth("2026-12")
	if afterOct.Revision <= oct.Revision || afterNov.Revision <= nov.Revision || dec.Revision == 0 {
		t.Fatal("revision did not advance old due, new due, and reporting months")
	}
}
