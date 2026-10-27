package domain

import (
	"errors"
	"math"
	"testing"
)

func scMoney(v Money) *Money    { return &v }
func scString(v string) *string { return &v }
func scState() *State {
	s := NewState()
	for _, a := range []Account{{Meta: Meta{ID: "bank", Version: 1}, AccountInput: AccountInput{Alias: "Main", Kind: "bank", Currency: "SGD", Active: true}}, {Meta: Meta{ID: "card", Version: 1}, AccountInput: AccountInput{Alias: "Card", Kind: "credit_card", Currency: "SGD", Active: true}}} {
		s.Accounts[a.ID] = a
	}
	return s
}
func scStatementInput() StatementInput {
	return StatementInput{AccountID: "card", Currency: "SGD", CycleStart: "2026-09-01", CycleEnd: "2026-09-30", IssueDate: "2026-10-01", DueDate: "2026-10-20", AmountDueMinor: 120000, NetSpendingMinor: scMoney(120000), SpendingEvidence: "Synthetic statement net cycle spending"}
}
func scPayment(s *State, id, date string, amount Money) Entry {
	entry := Entry{Meta: Meta{ID: id, Version: 1}, EntryInput: EntryInput{Kind: "card_settlement", Currency: "SGD", EffectiveDate: date, ReportingMonth: date[:7], AmountMinor: amount, SourceAccountID: scString("bank"), DestinationAccountID: scString("card")}, Active: true}
	s.Entries[id] = entry
	return entry
}
func scCode(t *testing.T, e error, want string) {
	t.Helper()
	var de *Error
	if !errors.As(e, &de) || de.Code != want {
		t.Fatalf("error %v, want code %s", e, want)
	}
}

func TestStatementSpendingKnownUnknownComponentsAndOverflow(t *testing.T) {
	s := scState()
	tests := []struct {
		name    string
		in      Statement
		want    *Money
		invalid bool
	}{
		{"unknown", Statement{}, nil, false},
		{"net", Statement{StatementInput: StatementInput{NetSpendingMinor: scMoney(1234), SpendingEvidence: "net"}}, scMoney(1234), false},
		{"negative net", Statement{StatementInput: StatementInput{NetSpendingMinor: scMoney(-100), SpendingEvidence: "net"}}, scMoney(-100), false},
		{"components", Statement{StatementInput: StatementInput{NewChargesMinor: scMoney(1000), FeesMinor: scMoney(20), InterestMinor: scMoney(10), RefundsMinor: scMoney(100)}}, scMoney(930), false},
		{"incomplete", Statement{StatementInput: StatementInput{NewChargesMinor: scMoney(1000)}}, nil, true},
		{"no evidence", Statement{StatementInput: StatementInput{NetSpendingMinor: scMoney(1000)}}, nil, true},
		{"double convention", Statement{StatementInput: StatementInput{NewChargesMinor: scMoney(1000), NetSpendingMinor: scMoney(1000), SpendingEvidence: "net"}}, nil, true},
		{"negative component", Statement{StatementInput: StatementInput{NewChargesMinor: scMoney(-1), FeesMinor: scMoney(0), InterestMinor: scMoney(0), RefundsMinor: scMoney(0)}}, nil, true},
		{"overflow", Statement{StatementInput: StatementInput{NewChargesMinor: scMoney(Money(math.MaxInt64)), FeesMinor: scMoney(1), InterestMinor: scMoney(0), RefundsMinor: scMoney(0)}}, nil, true},
		{"safe final despite intermediate overflow", Statement{StatementInput: StatementInput{NewChargesMinor: scMoney(Money(math.MaxInt64)), FeesMinor: scMoney(1), InterestMinor: scMoney(0), RefundsMinor: scMoney(1)}}, scMoney(Money(math.MaxInt64)), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, e := s.StatementSpending(tt.in)
			if tt.invalid {
				if e == nil {
					t.Fatal("expected error")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestSettlementSeparateFromSpendingCutoffCapsAndExcessCredit(t *testing.T) {
	s := scState()
	st, e := s.CreateStatement(scStatementInput())
	if e != nil {
		t.Fatal(e)
	}
	entry := scPayment(s, "payment", "2026-10-10", 40000)
	if _, e = s.CreateSettlement(st.ID, SettlementInput{EntryID: entry.ID, AmountMinor: 40000}, st.Version); e != nil {
		t.Fatal(e)
	}
	balance, e := s.StatementBalance(s.Statements[st.ID])
	if e != nil {
		t.Fatal(e)
	}
	if balance.RemainingMinor != 80000 || *balance.SpendingMinor != 120000 {
		t.Fatalf("balance %+v", balance)
	}
	_, e = s.CreateSettlement(st.ID, SettlementInput{EntryID: entry.ID, AmountMinor: 1}, st.Version)
	scCode(t, e, "STALE_VERSION")
	_, e = s.CreateSettlement(st.ID, SettlementInput{EntryID: entry.ID, AmountMinor: 1}, s.Statements[st.ID].Version)
	scCode(t, e, "SETTLEMENT_EXISTS")
	early := scPayment(s, "early", "2026-09-30", 10000)
	_, e = s.CreateSettlement(st.ID, SettlementInput{EntryID: early.ID, AmountMinor: 10000}, s.Statements[st.ID].Version)
	scCode(t, e, "VALIDATION_FAILED")
	small := scPayment(s, "small", "2026-10-11", 10000)
	_, e = s.CreateSettlement(st.ID, SettlementInput{EntryID: small.ID, AmountMinor: 10001}, s.Statements[st.ID].Version)
	scCode(t, e, "SETTLEMENT_LIMIT")
	extra := scPayment(s, "extra", "2026-10-12", 90000)
	_, e = s.CreateSettlement(st.ID, SettlementInput{EntryID: extra.ID, AmountMinor: 90000}, s.Statements[st.ID].Version)
	if e != nil {
		t.Fatal(e)
	}
	balance, e = s.StatementBalance(s.Statements[st.ID])
	if e != nil {
		t.Fatal(e)
	}
	if balance.RemainingMinor != 0 || balance.CreditMinor != 10000 || *balance.SpendingMinor != 120000 {
		t.Fatalf("balance %+v", balance)
	}
}

func TestStatementCarryRetiresPayableWithoutRetiringSpending(t *testing.T) {
	s := scState()
	first := scStatementInput()
	first.AmountDueMinor = 30000
	first.NetSpendingMinor = scMoney(30000)
	old, e := s.CreateStatement(first)
	if e != nil {
		t.Fatal(e)
	}
	next := scStatementInput()
	next.CycleStart = "2026-10-01"
	next.CycleEnd = "2026-10-31"
	next.IssueDate = "2026-11-01"
	next.DueDate = "2026-11-20"
	next.AmountDueMinor = 50000
	next.NetSpendingMinor = scMoney(20000)
	next.CarriesStatementID = &old.ID
	newStatement, e := s.CreateStatement(next)
	if e != nil {
		t.Fatal(e)
	}
	prior := s.Statements[old.ID]
	if !prior.Active || prior.PayableActive {
		t.Fatalf("prior %+v", prior)
	}
	oldBalance, _ := s.StatementBalance(prior)
	newBalance, _ := s.StatementBalance(newStatement)
	if oldBalance.RemainingMinor != 0 || *oldBalance.SpendingMinor != 30000 || newBalance.RemainingMinor != 50000 || *newBalance.SpendingMinor != 20000 {
		t.Fatalf("balances %+v %+v", oldBalance, newBalance)
	}
	_, e = s.ReviseStatement(old.ID, StatementRevision{Reason: "adjust old", Replacement: first}, prior.Version)
	scCode(t, e, "LINKED_RECORD")
}

func TestStatementRevisionMovesAssociationsAndKeepsHistory(t *testing.T) {
	s := scState()
	input := scStatementInput()
	old, e := s.CreateStatement(input)
	if e != nil {
		t.Fatal(e)
	}
	payment := scPayment(s, "payment", "2026-10-10", 40000)
	settlement, e := s.CreateSettlement(old.ID, SettlementInput{EntryID: payment.ID, AmountMinor: 40000}, old.Version)
	if e != nil {
		t.Fatal(e)
	}
	input.AmountDueMinor = 110000
	input.NetSpendingMinor = scMoney(110000)
	replacement, e := s.ReviseStatement(old.ID, StatementRevision{Reason: "Correct printed amount", Replacement: input}, s.Statements[old.ID].Version)
	if e != nil {
		t.Fatal(e)
	}
	if s.Statements[old.ID].Active || replacement.SupersedesID == nil || *replacement.SupersedesID != old.ID || s.Settlements[settlement.ID].StatementID != replacement.ID {
		t.Fatal("revision did not preserve history and settlement")
	}
	balance, e := s.StatementBalance(replacement)
	if e != nil {
		t.Fatal(e)
	}
	if balance.RemainingMinor != 70000 || *balance.SpendingMinor != 110000 {
		t.Fatalf("balance %+v", balance)
	}
}

func TestStatementValidationAndClosedMonth(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*State, *StatementInput)
		code   string
	}{
		{"wrong currency", func(s *State, i *StatementInput) { i.Currency = "USD" }, "VALIDATION_FAILED"},
		{"unknown card", func(s *State, i *StatementInput) { i.AccountID = "foreign" }, "NOT_FOUND"},
		{"not card", func(s *State, i *StatementInput) { i.AccountID = "bank" }, "VALIDATION_FAILED"},
		{"reversed dates", func(s *State, i *StatementInput) { i.CycleStart = "2026-10-02" }, "VALIDATION_FAILED"},
		{"override without reason", func(s *State, i *StatementInput) { i.ReportingMonth = "2026-11" }, "VALIDATION_FAILED"},
		{"negative due", func(s *State, i *StatementInput) { i.AmountDueMinor = -1 }, "VALIDATION_FAILED"},
		{"minimum above due", func(s *State, i *StatementInput) { i.MinimumDueMinor = scMoney(130000) }, "VALIDATION_FAILED"},
		{"closed month", func(s *State, i *StatementInput) { s.Months["2026-10"] = Month{Month: "2026-10", State: "closed"} }, "MONTH_CLOSED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := scState()
			in := scStatementInput()
			tc.mutate(s, &in)
			_, e := s.CreateStatement(in)
			scCode(t, e, tc.code)
			if len(s.Statements) != 0 {
				t.Fatal("invalid command mutated state")
			}
		})
	}
}

func TestObligationAdjustmentIsNotSecondSpendingRefund(t *testing.T) {
	s := scState()
	st, e := s.CreateStatement(scStatementInput())
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.CreateObligationAdjustment(st.ID, ObligationAdjustmentInput{AmountMinor: 10000, EffectiveDate: "2026-10-10", Reason: "Bank obligation credit"}, st.Version)
	if e != nil {
		t.Fatal(e)
	}
	balance, e := s.StatementBalance(s.Statements[st.ID])
	if e != nil {
		t.Fatal(e)
	}
	if balance.RemainingMinor != 110000 || *balance.SpendingMinor != 120000 {
		t.Fatalf("balance %+v", balance)
	}
	_, e = s.CreateObligationAdjustment(st.ID, ObligationAdjustmentInput{AmountMinor: 100, EffectiveDate: "2026-09-30", Reason: "Already included"}, s.Statements[st.ID].Version)
	scCode(t, e, "VALIDATION_FAILED")
}

func TestCarryCountsEarlierLinkedPaymentOnlyAfterLatestCutoff(t *testing.T) {
	s := scState()
	in := scStatementInput()
	in.AmountDueMinor = 30000
	in.NetSpendingMinor = scMoney(30000)
	old, e := s.CreateStatement(in)
	if e != nil {
		t.Fatal(e)
	}
	included := scPayment(s, "included", "2026-10-20", 10000)
	late := scPayment(s, "late", "2026-11-05", 5000)
	for _, entry := range []Entry{included, late} {
		if _, e = s.CreateSettlement(old.ID, SettlementInput{EntryID: entry.ID, AmountMinor: entry.AmountMinor}, s.Statements[old.ID].Version); e != nil {
			t.Fatal(e)
		}
	}
	in.CycleStart = "2026-10-01"
	in.CycleEnd = "2026-10-31"
	in.IssueDate = "2026-11-01"
	in.DueDate = "2026-11-20"
	in.AmountDueMinor = 40000
	in.NetSpendingMinor = scMoney(20000)
	in.CarriesStatementID = &old.ID
	next, e := s.CreateStatement(in)
	if e != nil {
		t.Fatal(e)
	}
	balance, e := s.StatementBalance(next)
	if e != nil {
		t.Fatal(e)
	}
	if balance.RemainingMinor != 35000 {
		t.Fatalf("remaining %d: must deduct only payment after new cutoff", balance.RemainingMinor)
	}
	if s.Settlements == nil || len(s.Settlements) != 2 {
		t.Fatal("carry changed historical settlement links")
	}
}

func TestStatementCreditOverflowRejectsBeforeMutation(t *testing.T) {
	s := scState()
	in := scStatementInput()
	in.AmountDueMinor = 0
	st, e := s.CreateStatement(in)
	if e != nil {
		t.Fatal(e)
	}
	a := scPayment(s, "a", "2026-10-05", Money(math.MaxInt64))
	b := scPayment(s, "b", "2026-10-06", 1)
	if _, e = s.CreateSettlement(st.ID, SettlementInput{EntryID: a.ID, AmountMinor: a.AmountMinor}, st.Version); e != nil {
		t.Fatal(e)
	}
	_, e = s.CreateSettlement(st.ID, SettlementInput{EntryID: b.ID, AmountMinor: b.AmountMinor}, s.Statements[st.ID].Version)
	scCode(t, e, "VALIDATION_FAILED")
	if len(s.Settlements) != 1 {
		t.Fatal("overflowing settlement committed")
	}
}
