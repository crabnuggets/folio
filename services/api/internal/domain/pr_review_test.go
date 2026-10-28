package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestFundingBoundaryIgnoresUnrelatedHistory(t *testing.T) {
	for _, history := range []string{"income", "expense", "statement"} {
		t.Run(history, func(t *testing.T) {
			s := NewState()
			bank := aeAccount(t, s, "bank", true)
			switch history {
			case "income", "expense":
				in := EntryInput{Kind: "income", AmountMinor: 100, Currency: "SGD", EffectiveDate: "2026-10-01", DestinationAccountID: &bank.ID}
				if history == "expense" {
					in.Kind, in.SourceAccountID, in.DestinationAccountID = "direct_expense", &bank.ID, nil
				}
				if _, err := s.CreateEntry(in); err != nil {
					t.Fatal(err)
				}
			case "statement":
				card := aeAccount(t, s, "credit_card", false)
				in := scStatementInput()
				in.AccountID = card.ID
				if _, err := s.CreateStatement(in); err != nil {
					t.Fatal(err)
				}
			}
			unrelated := aeAccount(t, s, "bank", false)
			in := unrelated.AccountInput
			in.FundingBoundary = []Boundary{{EffectiveMonth: "2026-10", Included: true}}
			if _, err := s.UpdateAccount(unrelated.ID, in, unrelated.Version); err != nil {
				t.Fatalf("unrelated history blocked funding account setup: %v", err)
			}
			if history != "statement" {
				in = bank.AccountInput
				in.FundingBoundary = nil
				_, err := s.UpdateAccount(bank.ID, in, bank.Version)
				aeErrorCode(t, err, "BOUNDARY_RESTATEMENT_REQUIRED")
			}
			// Closed reviews still prohibit membership changes even for an
			// account that has never appeared in a posting.
			s.Months["2026-11"] = Month{Month: "2026-11", State: "closed", Version: 1}
			unused := aeAccount(t, s, "bank", false)
			in = unused.AccountInput
			in.FundingBoundary = []Boundary{{EffectiveMonth: "2026-11", Included: true}}
			_, err := s.UpdateAccount(unused.ID, in, unused.Version)
			aeErrorCode(t, err, "BOUNDARY_RESTATEMENT_REQUIRED")
		})
	}
}

func TestRecurrenceRevisionProtectsBothIntervals(t *testing.T) {
	for _, tc := range []struct {
		name           string
		oldEnd, newEnd *string
		closed         string
		blocked        bool
	}{
		{"unbounded extension", aePtr("2026-12"), nil, "2027-02", true},
		{"bounded extension", aePtr("2026-12"), aePtr("2027-03"), "2027-02", true},
		{"shortening", aePtr("2027-03"), aePtr("2026-12"), "2027-02", true},
		{"unbounded predecessor", nil, aePtr("2026-12"), "2027-02", true},
		{"outside both intervals", aePtr("2026-12"), aePtr("2027-01"), "2027-02", false},
		{"before revision", aePtr("2026-12"), nil, "2026-10", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewState()
			old, err := s.CreateRecurrence(RecurrenceInput{Kind: "income", ExpectedMinor: 100, StartMonth: "2026-01", EndMonth: tc.oldEnd})
			if err != nil {
				t.Fatal(err)
			}
			s.Months[tc.closed] = Month{Month: tc.closed, State: "closed", Version: 1}
			audits := len(s.AuditEvents)
			_, err = s.ReviseRecurrence(old.ID, RecurrenceInput{Kind: "income", ExpectedMinor: 200, StartMonth: "2026-11", EndMonth: tc.newEnd}, old.Version)
			if tc.blocked {
				aeErrorCode(t, err, "MONTH_CLOSED")
				if len(s.Recurrences) != 1 || !reflect.DeepEqual(s.Recurrences[old.ID], old) || len(s.AuditEvents) != audits {
					t.Fatal("rejected revision mutated history")
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMoneyCanonicalWireStrings(t *testing.T) {
	for _, value := range []string{"0", "1", "-1", "9223372036854775807", "-9223372036854775808"} {
		var amount Money
		wire, _ := json.Marshal(value)
		if err := json.Unmarshal(wire, &amount); err != nil {
			t.Fatal(err)
		}
		got, err := json.Marshal(amount)
		if err != nil || string(got) != string(wire) {
			t.Fatalf("%s did not round trip: %s, %v", wire, got, err)
		}
	}
	for _, value := range []string{"-0", "00", "-01", "+1", "1.0", "9223372036854775808", "-9223372036854775809"} {
		amount := Money(7)
		wire, _ := json.Marshal(value)
		if err := json.Unmarshal(wire, &amount); err == nil {
			t.Fatalf("accepted noncanonical or out-of-range money %s", wire)
		}
		if amount != 7 {
			t.Fatal("invalid money changed destination")
		}
	}
}

func TestObligationCreditRejectsCashRefundLink(t *testing.T) {
	s := NewState()
	bank := aeAccount(t, s, "bank", true)
	card := aeAccount(t, s, "credit_card", false)
	input := scStatementInput()
	input.AccountID = card.ID
	st, err := s.CreateStatement(input)
	if err != nil {
		t.Fatal(err)
	}
	expense, err := s.CreateEntry(EntryInput{Kind: "direct_expense", AmountMinor: 100, Currency: "SGD", EffectiveDate: "2026-09-10", SourceAccountID: &bank.ID})
	if err != nil {
		t.Fatal(err)
	}
	refund, err := s.CreateEntry(EntryInput{Kind: "refund", AmountMinor: 100, Currency: "SGD", EffectiveDate: "2026-10-10", DestinationAccountID: &bank.ID, RelatedEntryID: &expense.ID})
	if err != nil {
		t.Fatal(err)
	}
	credit := ObligationAdjustmentInput{AmountMinor: 100, EffectiveDate: refund.EffectiveDate, Reason: "Statement credit", RefundEntryID: &refund.ID}
	audits := len(s.AuditEvents)
	_, err = s.CreateObligationAdjustment(st.ID, credit, st.Version)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	if len(s.ObligationAdjustments) != 0 || !reflect.DeepEqual(s.Statements[st.ID], st) || len(s.AuditEvents) != audits {
		t.Fatal("cash refund incorrectly reduced card obligation")
	}
	credit.RefundEntryID = nil
	if _, err := s.CreateObligationAdjustment(st.ID, credit, st.Version); err != nil {
		t.Fatal(err)
	}
	balance, err := s.StatementBalance(s.Statements[st.ID])
	if err != nil || balance.RemainingMinor != st.AmountDueMinor-100 || *balance.SpendingMinor != *st.NetSpendingMinor {
		t.Fatalf("explicit obligation credit changed spending: %+v, %v", balance, err)
	}
}
