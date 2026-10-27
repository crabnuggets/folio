package domain

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

func summaryTestState(t *testing.T) *State {
	t.Helper()
	s := NewState()
	s.Months["2026-10"] = Month{Month: "2026-10", Version: 1, State: "open", Checklist: Checklist{IncomeReviewed: true, CardsReviewed: true, ExpensesReviewed: true, AllocationsReviewed: true, NoStatementReasons: map[string]string{}}}
	return s
}
func summaryTestAccount(t *testing.T, s *State, kind string, funding bool) Account {
	t.Helper()
	a, e := s.CreateAccount(AccountInput{Alias: kind, Kind: kind, Currency: "SGD", Active: true, IncludeInLiquidCash: kind != "credit_card", FundingBoundary: []Boundary{{EffectiveMonth: "2026-01", Included: funding}}})
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func summaryTestEntry(s *State, kind string, amount Money, from, to *string) Entry {
	e := Entry{Meta: NewMeta(), Active: true, EntryInput: EntryInput{Kind: kind, AmountMinor: amount, Currency: "SGD", EffectiveDate: "2026-10-25", ReportingMonth: "2026-10", SourceAccountID: from, DestinationAccountID: to}}
	s.Entries[e.ID] = e
	return e
}
func summaryTestStatement(s *State, account string, due, spending Money) Statement {
	st := Statement{Meta: NewMeta(), Active: true, PayableActive: true, StatementInput: StatementInput{AccountID: account, Currency: "SGD", CycleStart: "2026-09-01", CycleEnd: "2026-09-30", IssueDate: "2026-10-01", DueDate: "2026-10-25", ReportingMonth: "2026-10", AmountDueMinor: due, NetSpendingMinor: summaryPointer(spending), SpendingEvidence: "synthetic bank net cycle spending"}}
	s.Statements[st.ID] = st
	return st
}
func TestSummaryWorkedMonthAndSettlementDoesNotSpendTwice(t *testing.T) {
	s := summaryTestState(t)
	cash := summaryTestAccount(t, s, "bank", true)
	investment := summaryTestAccount(t, s, "investment", false)
	cardA := summaryTestAccount(t, s, "credit_card", false)
	cardB := summaryTestAccount(t, s, "credit_card", false)
	summaryTestEntry(s, "income", 500000, nil, &cash.ID)
	summaryTestEntry(s, "direct_expense", 100000, &cash.ID, nil)
	summaryTestEntry(s, "allocation", 80000, &cash.ID, &investment.ID)
	stA := summaryTestStatement(s, cardA.ID, 120000, 120000)
	summaryTestStatement(s, cardB.ID, 30000, 30000)
	before, e := s.Summarize("2026-10")
	if e != nil {
		t.Fatal(e)
	}
	if before.KnownSpendingMinor != 250000 || before.AllocationMinor != 80000 || before.MonthlySurplusMinor == nil || *before.MonthlySurplusMinor != 170000 {
		t.Fatalf("unexpected worked month: %+v", before)
	}
	payment := summaryTestEntry(s, "card_settlement", 120000, &cash.ID, &cardA.ID)
	settlement := Settlement{Meta: NewMeta(), StatementID: stA.ID, SettlementInput: SettlementInput{EntryID: payment.ID, AmountMinor: 120000}}
	s.Settlements[settlement.ID] = settlement
	after, e := s.Summarize("2026-10")
	if e != nil {
		t.Fatal(e)
	}
	if *after.MonthlySurplusMinor != 170000 || after.KnownSpendingMinor != before.KnownSpendingMinor {
		t.Fatalf("settlement double counted spending: %+v", after)
	}
	if after.FundingRemainderMinor != 200000 {
		t.Fatalf("payment funding: %v", after.FundingRemainderMinor)
	}
	for _, balance := range after.Statements {
		if balance.StatementID == stA.ID && balance.RemainingMinor != 0 {
			t.Fatal(balance)
		}
	}
}
func TestSummaryFundingFixtureAndIndependentCoverage(t *testing.T) {
	s := summaryTestState(t)
	main := summaryTestAccount(t, s, "bank", true)
	other := summaryTestAccount(t, s, "cash", false)
	savings := summaryTestAccount(t, s, "savings", false)
	investment := summaryTestAccount(t, s, "investment", false)
	card := summaryTestAccount(t, s, "credit_card", false)
	summaryTestEntry(s, "income", 500000, nil, &main.ID)
	summaryTestEntry(s, "envelope_funding", 80000, &main.ID, &other.ID)
	summaryTestEntry(s, "envelope_return", 15000, &other.ID, &main.ID)
	summaryTestEntry(s, "card_settlement", 100000, &main.ID, &card.ID)
	summaryTestEntry(s, "allocation", 60000, &main.ID, &savings.ID)
	summaryTestEntry(s, "allocation", 40000, &main.ID, &investment.ID)
	summaryTestEntry(s, "direct_expense", 20000, &main.ID, nil)
	summaryTestEntry(s, "reimbursement_receipt", 10000, nil, &main.ID)
	st := summaryTestStatement(s, card.ID, 100000, 100000)
	st.NetSpendingMinor = nil
	s.Statements[st.ID] = st
	claim := Claim{Meta: NewMeta(), ClaimInput: ClaimInput{AmountMinor: 30000, Currency: "SGD", State: "pending", ExpectedReceiptDate: func() *string { v := "2026-10-29"; return &v }()}}
	s.Claims[claim.ID] = claim
	result, e := s.Summarize("2026-10")
	if e != nil {
		t.Fatal(e)
	}
	if result.FundingInflowsMinor != 525000 || result.FundingOutflowsMinor != 300000 || result.FundingRemainderMinor != 225000 {
		t.Fatalf("funding fixture: %+v", result)
	}
	if result.IncomeMinor != 500000 || result.UnmatchedClaimsMinor != 30000 {
		t.Fatal(result)
	}
	if result.SpendingComplete || !result.FundingComplete || result.MonthlySurplusMinor != nil {
		t.Fatalf("unknown spending invalidated known cash: %+v", result)
	}
}
func TestSummaryPlansReplaceExpectedAndSeparateCash(t *testing.T) {
	s := summaryTestState(t)
	main := summaryTestAccount(t, s, "bank", true)
	investment := summaryTestAccount(t, s, "investment", false)
	card := summaryTestAccount(t, s, "credit_card", false)
	incomeID, allocationID, paymentID := NewID(), NewID(), NewID()
	_, err := s.PutPlan("2026-10", PlanInput{Lines: []PlanLine{{ID: incomeID, Kind: "income", ExpectedMinor: 500000}, {ID: allocationID, Kind: "allocation", ExpectedMinor: 80000}, {ID: paymentID, Kind: "funding_outflow", ExpectedMinor: 120000, AccountID: &card.ID}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	income := summaryTestEntry(s, "income", 500000, nil, &main.ID)
	income.PlanLineID = &incomeID
	s.Entries[income.ID] = income
	allocation := summaryTestEntry(s, "allocation", 30000, &main.ID, &investment.ID)
	allocation.PlanLineID = &allocationID
	s.Entries[allocation.ID] = allocation
	st := summaryTestStatement(s, card.ID, 120000, 120000)
	payment := summaryTestEntry(s, "card_settlement", 40000, &main.ID, &card.ID)
	payment.FundingPlanLineID = &paymentID
	s.Entries[payment.ID] = payment
	settlement := Settlement{Meta: NewMeta(), StatementID: st.ID, SettlementInput: SettlementInput{EntryID: payment.ID, AmountMinor: 40000}}
	s.Settlements[settlement.ID] = settlement
	result, e := s.Summarize("2026-10")
	if e != nil {
		t.Fatal(e)
	}
	if result.ProjectedSurplusMinor == nil || *result.ProjectedSurplusMinor != 300000 {
		t.Fatalf("replacement forecasts: %+v", result)
	}
	if result.ProjectedFundingRemainderMinor == nil || *result.ProjectedFundingRemainderMinor != 350000 {
		t.Fatalf("cash plan/obligation double counted: %+v", result)
	}
	values := map[string]ForecastLine{}
	for _, v := range result.ForecastLines {
		values[v.ID] = v
	}
	if values[incomeID].ForecastMinor != 500000 || values[allocationID].ForecastMinor != 80000 || values[paymentID].ForecastMinor != 120000 {
		t.Fatal(values)
	}
}
func TestSummaryUnknownAndReadDeterminism(t *testing.T) {
	s := NewState()
	main := summaryTestAccount(t, s, "bank", true)
	card := summaryTestAccount(t, s, "credit_card", false)
	summaryTestEntry(s, "income", 500000, nil, &main.ID)
	st := summaryTestStatement(s, card.ID, 120000, 0)
	st.NetSpendingMinor = nil
	s.Statements[st.ID] = st
	original, _ := json.Marshal(s)
	first, e := s.Summarize("2026-10")
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.Summarize("2026-10")
	if e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(s)
	if !reflect.DeepEqual(first, second) || string(original) != string(after) {
		t.Fatal("summary changed state or order")
	}
	if first.MonthlySurplusMinor != nil || first.ProjectedSurplusMinor != nil || !first.IsProvisional || len(first.MissingItems) < 5 {
		t.Fatal(first)
	}
	if len(first.Statements) != 1 || first.Statements[0].RemainingMinor != 120000 {
		t.Fatal(first.Statements)
	}
}
func TestSummaryOverflowAndSignedPlanEstimates(t *testing.T) {
	t.Run("aggregation", func(t *testing.T) {
		s := summaryTestState(t)
		main := summaryTestAccount(t, s, "bank", true)
		summaryTestEntry(s, "income", Money(math.MaxInt64), nil, &main.ID)
		summaryTestEntry(s, "income", 1, nil, &main.ID)
		if _, e := s.Summarize("2026-10"); e == nil {
			t.Fatal("overflow accepted")
		}
	})
	t.Run("signed forecast", func(t *testing.T) {
		s := summaryTestState(t)
		id := NewID()
		s.Plans["2026-10"] = Plan{Version: 2, Lines: []PlanLine{{ID: id, Kind: "spending", ExpectedMinor: 100}}}
		e := summaryTestEntry(s, "refund", 10, nil, nil)
		e.PlanLineID = &id
		s.Entries[e.ID] = e
		if _, err := s.Summarize("2026-10"); err == nil {
			t.Fatal("signed actual needs estimate")
		}
		p := s.Plans["2026-10"]
		p.Lines[0].EstimatedRemainingMinor = summaryPointer(0)
		s.Plans["2026-10"] = p
		if _, err := s.Summarize("2026-10"); err != nil {
			t.Fatal(err)
		}
	})
}
func TestSummarySnapshotsRespectLocalMonthCutoff(t *testing.T) {
	s := summaryTestState(t)
	account := summaryTestAccount(t, s, "bank", true)
	for _, observed := range []string{"2026-10-31T15:59:00Z", "2026-10-31T16:00:00Z"} {
		at, _ := time.Parse(time.RFC3339, observed)
		snap := Snapshot{Meta: NewMeta(), SnapshotInput: SnapshotInput{AccountID: account.ID, AmountMinor: 100, ObservedAt: at}}
		s.Snapshots[snap.ID] = snap
	}
	result, e := s.Summarize("2026-10")
	if e != nil {
		t.Fatal(e)
	}
	if len(result.Snapshots) != 1 || result.Snapshots[0].ObservedAt.Format(time.RFC3339) != "2026-10-31T15:59:00Z" {
		t.Fatal(result.Snapshots)
	}
}

func TestFundingForecastRequiresKnownBoundaryPaymentPlan(t *testing.T) {
	s := summaryTestState(t)
	main := summaryTestAccount(t, s, "bank", true)
	card := summaryTestAccount(t, s, "credit_card", false)
	summaryTestEntry(s, "income", 500000, nil, &main.ID)
	st := summaryTestStatement(s, card.ID, 100000, 100000)
	result, err := s.Summarize("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if !result.FundingComplete || result.FundingRemainderMinor != 500000 || result.ProjectedFundingRemainderMinor != nil {
		t.Fatalf("unpaid card's unspecified source was treated as funding outflow: %+v", result)
	}
	found := false
	for _, item := range result.MissingItems {
		if item == "obligation_funding_plan_missing:"+st.ID {
			found = true
		}
	}
	if !found {
		t.Fatal(result.MissingItems)
	}
	if _, err = s.PutPlan("2026-10", PlanInput{Lines: []PlanLine{{ID: NewID(), Kind: "funding_outflow", ExpectedMinor: 100000, AccountID: &card.ID}}}, 1); err != nil {
		t.Fatal(err)
	}
	result, err = s.Summarize("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectedFundingRemainderMinor == nil || *result.ProjectedFundingRemainderMinor != 400000 {
		t.Fatal(result)
	}
}

func TestUnreviewedMonthRetainsNumericallyKnownForecast(t *testing.T) {
	s := NewState()
	main := summaryTestAccount(t, s, "bank", true)
	summaryTestEntry(s, "income", 500000, nil, &main.ID)
	result, err := s.Summarize("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsProvisional || result.MonthlySurplusMinor != nil || result.ProjectedSurplusMinor == nil || *result.ProjectedSurplusMinor != 500000 {
		t.Fatalf("review assertions incorrectly hide known forecasts: %+v", result)
	}
}

func TestClaimOutstandingAnchorsMonthAndIgnoresFutureReceipt(t *testing.T) {
	s := summaryTestState(t)
	main := summaryTestAccount(t, s, "bank", true)
	date := "2026-10-20"
	claim := Claim{Meta: NewMeta(), ClaimInput: ClaimInput{AmountMinor: 10000, Currency: "SGD", State: "pending", ExpectedReceiptDate: &date}}
	s.Claims[claim.ID] = claim
	undated := Claim{Meta: NewMeta(), ClaimInput: ClaimInput{AmountMinor: 50000, Currency: "SGD", State: "pending"}}
	s.Claims[undated.ID] = undated
	before, err := s.Summarize("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if before.UnmatchedClaimsMinor != 10000 {
		t.Fatalf("undated global claim leaked into month: %+v", before)
	}
	receipt := summaryTestEntry(s, "reimbursement_receipt", 10000, nil, &main.ID)
	receipt.ReportingMonth = "2026-11"
	receipt.EffectiveDate = "2026-11-01"
	s.Entries[receipt.ID] = receipt
	match := ReceiptMatch{Meta: NewMeta(), ClaimID: claim.ID, ReceiptMatchInput: ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000}}
	s.ReceiptMatches[match.ID] = match
	claim.State = "received"
	s.Claims[claim.ID] = claim
	after, err := s.Summarize("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if after.UnmatchedClaimsMinor != 10000 {
		t.Fatal("future receipt restated historical outstanding")
	}
	november, err := s.Summarize("2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if november.UnmatchedClaimsMinor != 0 {
		t.Fatal("October claim moved into unrelated month")
	}
}

func TestPendingPlannedSalaryStaysUnknownAfterReview(t *testing.T) {
	s := summaryTestState(t)
	incomeID, fundingID := NewID(), NewID()
	if _, err := s.PutPlan("2026-10", PlanInput{Lines: []PlanLine{{ID: incomeID, Kind: "income", ExpectedMinor: 500000}, {ID: fundingID, Kind: "funding_inflow", ExpectedMinor: 500000}}}, 1); err != nil {
		t.Fatal(err)
	}
	result, err := s.Summarize("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsProvisional || result.SpendingComplete || result.FundingComplete || result.MonthlySurplusMinor != nil || result.ConsumptionSavingsMinor != nil || result.SavingsRate != nil {
		t.Fatalf("unreceived planned salary advertised as reviewed zero: %+v", result)
	}
	if result.ProjectedSurplusMinor == nil || *result.ProjectedSurplusMinor != 500000 || result.ProjectedFundingRemainderMinor == nil || *result.ProjectedFundingRemainderMinor != 500000 {
		t.Fatalf("numeric pending forecasts lost: %+v", result)
	}
	if _, err = s.PutPlan("2026-10", PlanInput{Lines: []PlanLine{{ID: incomeID, Kind: "income", ExpectedMinor: 500000, Complete: true}, {ID: fundingID, Kind: "funding_inflow", ExpectedMinor: 500000, Complete: true}}}, 2); err != nil {
		t.Fatal(err)
	}
	result, err = s.Summarize("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if result.IsProvisional || result.MonthlySurplusMinor == nil || *result.MonthlySurplusMinor != 0 || result.SavingsRate != nil {
		t.Fatalf("explicit completed actual zero remained unknown: %+v", result)
	}
}
