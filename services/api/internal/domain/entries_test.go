package domain

import "testing"

func TestEntryDirectionsCurrencyAndClosedMonth(t *testing.T) {
	s := NewState()
	bank := aeAccount(t, s, "bank", true)
	cash := aeAccount(t, s, "cash", false)
	card := aeAccount(t, s, "credit_card", false)
	investment := aeAccount(t, s, "investment", false)
	cases := []struct {
		kind                string
		source, destination *string
		valid               bool
	}{
		{"income", nil, &bank.ID, true}, {"income", &cash.ID, &bank.ID, false},
		{"direct_expense", &bank.ID, nil, true}, {"direct_expense", &card.ID, nil, false},
		{"reimbursement_receipt", nil, &bank.ID, true}, {"transfer", &bank.ID, &cash.ID, true},
		{"transfer", &bank.ID, &bank.ID, false}, {"transfer", &bank.ID, &card.ID, false},
		{"envelope_funding", &bank.ID, &cash.ID, true}, {"envelope_funding", &cash.ID, &bank.ID, false},
		{"envelope_return", &cash.ID, &bank.ID, true}, {"allocation", &bank.ID, &investment.ID, true},
		{"allocation", &bank.ID, &cash.ID, false}, {"card_settlement", &bank.ID, &card.ID, true},
		{"card_settlement", &card.ID, &bank.ID, false}, {"unsupported", nil, &bank.ID, false},
	}
	for _, tc := range cases {
		t.Run(tc.kind+"/"+func() string {
			if tc.valid {
				return "valid"
			}
			return "invalid"
		}(), func(t *testing.T) {
			_, err := s.CreateEntry(EntryInput{Kind: tc.kind, AmountMinor: 100, Currency: "SGD", EffectiveDate: "2026-10-10", SourceAccountID: tc.source, DestinationAccountID: tc.destination})
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && err == nil {
				t.Fatal("invalid direction accepted")
			}
		})
	}
	for _, tc := range []struct {
		amount                        Money
		currency, date, month, reason string
	}{{0, "SGD", "2026-10-10", "", ""}, {-1, "SGD", "2026-10-10", "", ""}, {100, "USD", "2026-10-10", "", ""}, {100, "SGD", "2026-02-30", "", ""}, {100, "SGD", "2026-10-10", "2026-09", ""}} {
		_, err := s.CreateEntry(EntryInput{Kind: "income", AmountMinor: tc.amount, Currency: tc.currency, EffectiveDate: tc.date, ReportingMonth: tc.month, ReportingMonthReason: tc.reason, DestinationAccountID: &bank.ID})
		aeErrorCode(t, err, "VALIDATION_FAILED")
	}
	overridden, err := s.CreateEntry(EntryInput{Kind: "income", AmountMinor: 100, Currency: "SGD", EffectiveDate: "2026-10-10", ReportingMonth: "2026-09", ReportingMonthReason: "Assigned to review month", DestinationAccountID: &bank.ID})
	if err != nil || overridden.ReportingMonth != "2026-09" {
		t.Fatal("reasoned override failed", err)
	}
	s.Months["2026-10"] = Month{Month: "2026-10", State: "closed", Version: 1}
	before := len(s.Entries)
	_, err = s.CreateEntry(EntryInput{Kind: "income", AmountMinor: 100, Currency: "SGD", EffectiveDate: "2026-10-10", DestinationAccountID: &bank.ID})
	aeErrorCode(t, err, "MONTH_CLOSED")
	if len(s.Entries) != before {
		t.Fatal("closed-month write mutated state")
	}
}

func TestRefundAndWithdrawalCapsAndCorrections(t *testing.T) {
	s := NewState()
	bank := aeAccount(t, s, "bank", true)
	investment := aeAccount(t, s, "investment", false)
	expense, err := s.CreateEntry(EntryInput{Kind: "direct_expense", AmountMinor: 1000, Currency: "SGD", EffectiveDate: "2026-10-01", SourceAccountID: &bank.ID})
	if err != nil {
		t.Fatal(err)
	}
	refund, err := s.CreateEntry(EntryInput{Kind: "refund", AmountMinor: 400, Currency: "SGD", EffectiveDate: "2026-10-02", DestinationAccountID: &bank.ID, RelatedEntryID: &expense.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateEntry(EntryInput{Kind: "refund", AmountMinor: 601, Currency: "SGD", EffectiveDate: "2026-10-03", DestinationAccountID: &bank.ID, RelatedEntryID: &expense.ID})
	aeErrorCode(t, err, "ORIGINAL_AMOUNT_EXCEEDED")
	replacement := refund.EntryInput
	replacement.AmountMinor = 600
	corrected, err := s.CorrectEntry(refund.ID, EntryCorrection{Reason: "Correct bank value", Replacement: &replacement}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.SupersedesID == nil || *corrected.SupersedesID != refund.ID || s.Entries[refund.ID].Active || !corrected.Active {
		t.Fatal("correction history lost")
	}
	_, err = s.CorrectEntry(expense.ID, EntryCorrection{Reason: "void"}, 1)
	aeErrorCode(t, err, "ENTRY_LINKED")
	_, err = s.CorrectEntry(corrected.ID, EntryCorrection{Reason: "void"}, 0)
	aeErrorCode(t, err, "STALE_VERSION")
	voided, err := s.CorrectEntry(corrected.ID, EntryCorrection{Reason: "Refund was reversed"}, 1)
	if err != nil || voided.Active {
		t.Fatal("void failed", err)
	}
	allocation, err := s.CreateEntry(EntryInput{Kind: "allocation", AmountMinor: 2000, Currency: "SGD", EffectiveDate: "2026-10-01", SourceAccountID: &bank.ID, DestinationAccountID: &investment.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateEntry(EntryInput{Kind: "allocation_withdrawal", AmountMinor: 1000, Currency: "SGD", EffectiveDate: "2026-10-03", SourceAccountID: &investment.ID, DestinationAccountID: &bank.ID, RelatedEntryID: &allocation.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateEntry(EntryInput{Kind: "allocation_withdrawal", AmountMinor: 1001, Currency: "SGD", EffectiveDate: "2026-10-03", SourceAccountID: &investment.ID, DestinationAccountID: &bank.ID, RelatedEntryID: &allocation.ID})
	aeErrorCode(t, err, "ORIGINAL_AMOUNT_EXCEEDED")
}

func TestCorrectionRetainsStateOnFailureAndProtectsLinks(t *testing.T) {
	s := NewState()
	bank := aeAccount(t, s, "bank", true)
	expense, err := s.CreateEntry(EntryInput{Kind: "direct_expense", AmountMinor: 1000, Currency: "SGD", EffectiveDate: "2026-10-01", SourceAccountID: &bank.ID})
	if err != nil {
		t.Fatal(err)
	}
	replacement := expense.EntryInput
	replacement.AmountMinor = -10
	_, err = s.CorrectEntry(expense.ID, EntryCorrection{Reason: "bad", Replacement: &replacement}, 1)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	if !s.Entries[expense.ID].Active || s.Entries[expense.ID].Version != 1 {
		t.Fatal("invalid correction changed original")
	}
	claim := Claim{Meta: NewMeta(), ClaimInput: ClaimInput{ExpenseEntryID: &expense.ID}}
	s.Claims[claim.ID] = claim
	_, err = s.CorrectEntry(expense.ID, EntryCorrection{Reason: "void"}, 1)
	aeErrorCode(t, err, "ENTRY_LINKED")
	s.ReceiptMatches["match"] = ReceiptMatch{ReceiptMatchInput: ReceiptMatchInput{AmountMinor: 900, RecoveryMonth: aePtr("2026-10")}, ClaimID: claim.ID}
	_, err = s.CreateEntry(EntryInput{Kind: "refund", AmountMinor: 101, Currency: "SGD", EffectiveDate: "2026-10-03", DestinationAccountID: &bank.ID, RelatedEntryID: &expense.ID})
	aeErrorCode(t, err, "ORIGINAL_AMOUNT_EXCEEDED")
}

func TestRecognizedAndFundingPlanMatchesAreSeparate(t *testing.T) {
	s := NewState()
	bank := aeAccount(t, s, "bank", true)
	cash := aeAccount(t, s, "cash", false)
	incomeLine, fundingLine, spendLine := NewID(), NewID(), NewID()
	s.Plans["2026-10"] = Plan{Version: 1, Lines: []PlanLine{{ID: incomeLine, Kind: "income"}, {ID: fundingLine, Kind: "funding_inflow"}, {ID: spendLine, Kind: "spending"}}}
	entry := EntryInput{Kind: "income", AmountMinor: 500000, Currency: "SGD", EffectiveDate: "2026-10-01", DestinationAccountID: &bank.ID, PlanLineID: &incomeLine, FundingPlanLineID: &fundingLine}
	if _, err := s.CreateEntry(entry); err != nil {
		t.Fatal(err)
	}
	entry.PlanLineID = &spendLine
	_, err := s.CreateEntry(entry)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	entry.PlanLineID = &incomeLine
	entry.DestinationAccountID = &cash.ID
	_, err = s.CreateEntry(entry)
	aeErrorCode(t, err, "VALIDATION_FAILED")
}
