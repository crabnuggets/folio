package domain

import "testing"

func scExpense(s *State, id string, amount Money) Entry {
	entry := Entry{Meta: Meta{ID: id, Version: 1}, EntryInput: EntryInput{Kind: "direct_expense", AmountMinor: amount, Currency: "SGD", EffectiveDate: "2026-10-10", ReportingMonth: "2026-10", SourceAccountID: scString("bank")}, Active: true}
	s.Entries[id] = entry
	return entry
}
func scReceipt(s *State, id string, amount Money) Entry {
	entry := Entry{Meta: Meta{ID: id, Version: 1}, EntryInput: EntryInput{Kind: "reimbursement_receipt", AmountMinor: amount, Currency: "SGD", EffectiveDate: "2026-11-10", ReportingMonth: "2026-11", DestinationAccountID: scString("bank")}, Active: true}
	s.Entries[id] = entry
	return entry
}
func scClaimInput(expense *string, amount Money) ClaimInput {
	return ClaimInput{AmountMinor: amount, Currency: "SGD", Description: "Synthetic reimbursement", ExpenseEntryID: expense, State: "pending"}
}

func TestClaimPendingCreatesNoCashAndPartialReceiptIsSingleRecovery(t *testing.T) {
	s := scState()
	expense := scExpense(s, "expense", 30000)
	beforeEntries := len(s.Entries)
	claim, e := s.CreateClaim(scClaimInput(&expense.ID, 30000))
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Entries) != beforeEntries {
		t.Fatal("pending claim created cash")
	}
	receipt := scReceipt(s, "receipt", 10000)
	match, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000}, claim.Version)
	if e != nil {
		t.Fatal(e)
	}
	if match.RecoveryMonth == nil || *match.RecoveryMonth != "2026-11" || s.Claims[claim.ID].State != "partially_received" || len(s.Entries) != beforeEntries+1 {
		t.Fatalf("match %+v, claim %+v", match, s.Claims[claim.ID])
	}
	_, e = s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000}, s.Claims[claim.ID].Version)
	scCode(t, e, "RECEIPT_MATCH_EXISTS")
	if len(s.ReceiptMatches) != 1 {
		t.Fatal("duplicate match was recorded")
	}
	second := scReceipt(s, "second", 20000)
	_, e = s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: second.ID, AmountMinor: 20000}, s.Claims[claim.ID].Version)
	if e != nil {
		t.Fatal(e)
	}
	if s.Claims[claim.ID].State != "received" {
		t.Fatalf("claim %+v", s.Claims[claim.ID])
	}
}

func TestReceiptAllocationClaimAndSharedExpenseCaps(t *testing.T) {
	t.Run("receipt cap", func(t *testing.T) {
		s := scState()
		expense := scExpense(s, "expense", 30000)
		a, _ := s.CreateClaim(scClaimInput(&expense.ID, 20000))
		b, _ := s.CreateClaim(scClaimInput(&expense.ID, 20000))
		receipt := scReceipt(s, "receipt", 10000)
		if _, e := s.MatchReceipt(a.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 7000}, a.Version); e != nil {
			t.Fatal(e)
		}
		_, e := s.MatchReceipt(b.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 4000}, b.Version)
		scCode(t, e, "RECEIPT_LIMIT")
		if len(s.ReceiptMatches) != 1 {
			t.Fatal("cap failure mutated matches")
		}
	})
	t.Run("claim cap", func(t *testing.T) {
		s := scState()
		claim, _ := s.CreateClaim(scClaimInput(nil, 10000))
		receipt := scReceipt(s, "receipt", 20000)
		_, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10001}, claim.Version)
		scCode(t, e, "CLAIM_LIMIT")
	})
	t.Run("cost cap across claims", func(t *testing.T) {
		s := scState()
		expense := scExpense(s, "expense", 10000)
		a, _ := s.CreateClaim(scClaimInput(&expense.ID, 10000))
		b, _ := s.CreateClaim(scClaimInput(&expense.ID, 10000))
		first := scReceipt(s, "first", 7000)
		second := scReceipt(s, "second", 4000)
		if _, e := s.MatchReceipt(a.ID, ReceiptMatchInput{ReceiptEntryID: first.ID, AmountMinor: 7000}, a.Version); e != nil {
			t.Fatal(e)
		}
		_, e := s.MatchReceipt(b.ID, ReceiptMatchInput{ReceiptEntryID: second.ID, AmountMinor: 4000}, b.Version)
		scCode(t, e, "RECOVERY_LIMIT")
	})
	t.Run("refund reduces recoverable cost", func(t *testing.T) {
		s := scState()
		expense := scExpense(s, "expense", 10000)
		refund := Entry{Meta: Meta{ID: "refund"}, EntryInput: EntryInput{Kind: "refund", AmountMinor: 3000, RelatedEntryID: &expense.ID}, Active: true}
		s.Entries[refund.ID] = refund
		claim, _ := s.CreateClaim(scClaimInput(&expense.ID, 10000))
		receipt := scReceipt(s, "receipt", 10000)
		_, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 7001}, claim.Version)
		scCode(t, e, "RECOVERY_LIMIT")
	})
}

func TestUnclassifiedReceiptMatchDoesNotInventNegativeSpending(t *testing.T) {
	s := scState()
	claim, _ := s.CreateClaim(scClaimInput(nil, 10000))
	receipt := scReceipt(s, "receipt", 10000)
	match, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000}, claim.Version)
	if e != nil {
		t.Fatal(e)
	}
	if match.RecoveryMonth != nil {
		t.Fatal("unclassified recovery was posted")
	}
	s2 := scState()
	claim, _ = s2.CreateClaim(scClaimInput(nil, 10000))
	receipt = scReceipt(s2, "receipt", 10000)
	_, e = s2.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000, RecoveryMonth: scString("2026-11")}, claim.Version)
	scCode(t, e, "VALIDATION_FAILED")
}

func TestReceiptRecoveryCannotChangeClosedMonth(t *testing.T) {
	s := scState()
	expense := scExpense(s, "expense", 10000)
	claim, _ := s.CreateClaim(scClaimInput(&expense.ID, 10000))
	receipt := scReceipt(s, "receipt", 10000)
	s.Months["2026-10"] = Month{Month: "2026-10", State: "closed"}
	_, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000, RecoveryMonth: scString("2026-10")}, claim.Version)
	scCode(t, e, "MONTH_CLOSED")
	if len(s.ReceiptMatches) != 0 {
		t.Fatal("closed recovery was posted")
	}
	if _, e = s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000}, claim.Version); e != nil {
		t.Fatal("receipt in open month must not require reopening original cost month", e)
	}
}

func TestClaimUpdateCannotRewriteReceivedCostOrUndercutReceipts(t *testing.T) {
	s := scState()
	expense := scExpense(s, "expense", 10000)
	other := scExpense(s, "other", 10000)
	in := scClaimInput(&expense.ID, 10000)
	claim, _ := s.CreateClaim(in)
	receipt := scReceipt(s, "receipt", 7000)
	if _, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 7000}, claim.Version); e != nil {
		t.Fatal(e)
	}
	in.ExpenseEntryID = &other.ID
	_, e := s.UpdateClaim(claim.ID, in, s.Claims[claim.ID].Version)
	scCode(t, e, "LINKED_RECORD")
	in.ExpenseEntryID = &expense.ID
	in.AmountMinor = 6000
	_, e = s.UpdateClaim(claim.ID, in, s.Claims[claim.ID].Version)
	scCode(t, e, "CLAIM_LIMIT")
	_, e = s.UpdateClaim(claim.ID, in, claim.Version)
	scCode(t, e, "STALE_VERSION")
}

func TestStatementRevisionCannotUndercutRecoveredSpending(t *testing.T) {
	s := scState()
	in := scStatementInput()
	st, _ := s.CreateStatement(in)
	claimIn := scClaimInput(nil, 10000)
	claimIn.StatementID = &st.ID
	claim, _ := s.CreateClaim(claimIn)
	receipt := scReceipt(s, "receipt", 10000)
	if _, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000}, claim.Version); e != nil {
		t.Fatal(e)
	}
	in.NetSpendingMinor = scMoney(9999)
	_, e := s.ReviseStatement(st.ID, StatementRevision{Reason: "Correct spend", Replacement: in}, st.Version)
	scCode(t, e, "RECOVERY_LIMIT")
	if !s.Statements[st.ID].Active || len(s.Statements) != 1 {
		t.Fatal("failed revision mutated statement history")
	}
}

func TestClaimAnchorsProtectClosedMonthAndFutureReceiptDoesNotReviseIt(t *testing.T) {
	s := scState()
	expense := scExpense(s, "expense", 10000)
	claim, e := s.CreateClaim(scClaimInput(&expense.ID, 10000))
	if e != nil {
		t.Fatal(e)
	}
	s.Months["2026-10"] = Month{Month: "2026-10", State: "closed", Version: 7, Revision: 9}
	receipt := scReceipt(s, "receipt", 10000)
	if _, e = s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000}, claim.Version); e != nil {
		t.Fatal(e)
	}
	if s.Months["2026-10"].Version != 7 || s.Months["2026-10"].Revision != 9 {
		t.Fatal("future receipt rewrote closed claim anchor")
	}
	_, e = s.CreateClaim(scClaimInput(&expense.ID, 100))
	scCode(t, e, "MONTH_CLOSED")
	in := claim.ClaimInput
	in.Description = "Change historical claim"
	_, e = s.UpdateClaim(claim.ID, in, s.Claims[claim.ID].Version)
	scCode(t, e, "MONTH_CLOSED")
}

func TestUnmatchedReceiptCanBeClassifiedWithoutAnotherCashOrAllocation(t *testing.T) {
	s := scState()
	claim, e := s.CreateClaim(scClaimInput(nil, 10000))
	if e != nil {
		t.Fatal(e)
	}
	receipt := scReceipt(s, "receipt", 10000)
	first, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000}, claim.Version)
	if e != nil {
		t.Fatal(e)
	}
	expense := scExpense(s, "expense", 10000)
	in := s.Claims[claim.ID].ClaimInput
	in.ExpenseEntryID = &expense.ID
	updated, e := s.UpdateClaim(claim.ID, in, s.Claims[claim.ID].Version)
	if e != nil {
		t.Fatal(e)
	}
	promoted, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000, RecoveryMonth: scString("2026-11")}, updated.Version)
	if e != nil {
		t.Fatal(e)
	}
	if promoted.ID != first.ID || promoted.Version != first.Version+1 || promoted.RecoveryMonth == nil || len(s.ReceiptMatches) != 1 || len(s.Entries) != 2 {
		t.Fatalf("promotion created duplicate financial records: %+v", promoted)
	}
	_, e = s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: 10000, RecoveryMonth: scString("2026-11")}, s.Claims[claim.ID].Version)
	scCode(t, e, "RECEIPT_MATCH_EXISTS")
}

func TestReceiptClassificationPromotionChecksCostAndClosedMonths(t *testing.T) {
	for _, tc := range []struct {
		name   string
		close  bool
		amount Money
		code   string
	}{{"cost cap", false, 10001, "RECOVERY_LIMIT"}, {"closed recovery", true, 10000, "MONTH_CLOSED"}} {
		t.Run(tc.name, func(t *testing.T) {
			s := scState()
			claim, _ := s.CreateClaim(scClaimInput(nil, tc.amount))
			receipt := scReceipt(s, "receipt", tc.amount)
			first, e := s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: tc.amount}, claim.Version)
			if e != nil {
				t.Fatal(e)
			}
			expense := scExpense(s, "expense", 10000)
			in := s.Claims[claim.ID].ClaimInput
			in.ExpenseEntryID = &expense.ID
			updated, e := s.UpdateClaim(claim.ID, in, s.Claims[claim.ID].Version)
			if e != nil {
				t.Fatal(e)
			}
			if tc.close {
				s.Months["2026-12"] = Month{Month: "2026-12", State: "closed"}
			}
			_, e = s.MatchReceipt(claim.ID, ReceiptMatchInput{ReceiptEntryID: receipt.ID, AmountMinor: tc.amount, RecoveryMonth: scString("2026-12")}, updated.Version)
			scCode(t, e, tc.code)
			if s.ReceiptMatches[first.ID].RecoveryMonth != nil || len(s.ReceiptMatches) != 1 {
				t.Fatal("failed promotion changed financial state")
			}
		})
	}
}
