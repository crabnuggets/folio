package domain

import "strings"

func (s *State) validateEntry(input EntryInput, excludeID string) (EntryInput, error) {
	if input.AmountMinor <= 0 {
		return EntryInput{}, Invalid("Entry amount must be positive")
	}
	if err := s.ValidateCurrency(input.Currency); err != nil {
		return EntryInput{}, err
	}
	month, err := s.NormalizeReportingMonth(input.EffectiveDate, input.ReportingMonth, input.ReportingMonthReason)
	if err != nil {
		return EntryInput{}, err
	}
	input.ReportingMonth = month
	var source, destination Account
	if input.SourceAccountID != nil {
		source, err = s.AccountForPosting(*input.SourceAccountID)
		if err != nil {
			return EntryInput{}, err
		}
	}
	if input.DestinationAccountID != nil {
		destination, err = s.AccountForPosting(*input.DestinationAccountID)
		if err != nil {
			return EntryInput{}, err
		}
	}
	if input.SourceAccountID != nil && input.DestinationAccountID != nil && *input.SourceAccountID == *input.DestinationAccountID {
		return EntryInput{}, Invalid("Source and destination must differ")
	}
	switch input.Kind {
	case "income", "refund", "reimbursement_receipt":
		if input.SourceAccountID != nil || input.DestinationAccountID == nil || destination.Kind == "credit_card" {
			return EntryInput{}, Invalid("External receipts require only a non-card destination account")
		}
	case "direct_expense":
		if input.SourceAccountID == nil || input.DestinationAccountID != nil || source.Kind == "credit_card" {
			return EntryInput{}, Invalid("Direct expenses require only a non-card source account")
		}
	case "card_settlement":
		if input.SourceAccountID == nil || input.DestinationAccountID == nil || source.Kind == "credit_card" || destination.Kind != "credit_card" {
			return EntryInput{}, Invalid("Card settlement requires a non-card source and credit-card destination")
		}
	case "transfer", "envelope_funding", "envelope_return", "allocation", "allocation_withdrawal":
		if input.SourceAccountID == nil || input.DestinationAccountID == nil || source.Kind == "credit_card" || destination.Kind == "credit_card" {
			return EntryInput{}, Invalid("Owned transfers require two non-card accounts")
		}
		if input.Kind == "allocation" && destination.Kind != "savings" && destination.Kind != "investment" {
			return EntryInput{}, Invalid("Allocations require a savings or investment destination")
		}
		if input.Kind == "allocation_withdrawal" && source.Kind != "savings" && source.Kind != "investment" {
			return EntryInput{}, Invalid("Allocation withdrawals require a savings or investment source")
		}
		if input.Kind == "envelope_funding" && (!s.FundingIncluded(source.ID, month) || s.FundingIncluded(destination.ID, month)) {
			return EntryInput{}, Invalid("Envelope funding must leave the funding boundary")
		}
		if input.Kind == "envelope_return" && (s.FundingIncluded(source.ID, month) || !s.FundingIncluded(destination.ID, month)) {
			return EntryInput{}, Invalid("Envelope return must enter the funding boundary")
		}
	default:
		return EntryInput{}, Invalid("Unsupported entry kind")
	}
	if input.RelatedEntryID != nil {
		if !ValidID(*input.RelatedEntryID) {
			return EntryInput{}, Invalid("Related entry ID must be a UUID")
		}
		if input.Kind != "refund" && input.Kind != "allocation_withdrawal" {
			return EntryInput{}, Invalid("Only refunds and allocation withdrawals reference original entries")
		}
	}
	if input.Kind == "refund" || input.Kind == "allocation_withdrawal" {
		if input.RelatedEntryID == nil {
			return EntryInput{}, Invalid("Refunds and withdrawals require an original tracked entry")
		}
		original, ok := s.Entries[*input.RelatedEntryID]
		if !ok || !original.Active || original.ID == excludeID {
			return EntryInput{}, NotFound()
		}
		expectedKind := "direct_expense"
		if input.Kind == "allocation_withdrawal" {
			expectedKind = "allocation"
		}
		if original.Kind != expectedKind {
			return EntryInput{}, Invalid("Original entry has an incompatible purpose")
		}
		if input.EffectiveDate < original.EffectiveDate {
			return EntryInput{}, Invalid("Refund or withdrawal cannot precede its original entry")
		}
		if input.Kind == "allocation_withdrawal" && (*input.SourceAccountID != *original.DestinationAccountID || *input.DestinationAccountID != *original.SourceAccountID) {
			return EntryInput{}, Invalid("Allocation withdrawal must reverse the original account direction")
		}
		remaining := original.AmountMinor
		for id, e := range s.Entries {
			if id != excludeID && e.Active && e.RelatedEntryID != nil && *e.RelatedEntryID == original.ID {
				remaining -= e.AmountMinor
				if remaining < 0 {
					return EntryInput{}, Invalid("Original amount has already been exceeded")
				}
			}
		}
		if input.Kind == "refund" {
			for _, match := range s.ReceiptMatches {
				claim := s.Claims[match.ClaimID]
				if match.RecoveryMonth != nil && claim.ExpenseEntryID != nil && *claim.ExpenseEntryID == original.ID {
					remaining -= match.AmountMinor
					if remaining < 0 {
						return EntryInput{}, Invalid("Original expense has already been recovered")
					}
				}
			}
		}
		if input.AmountMinor > remaining {
			return EntryInput{}, Fail(409, "ORIGINAL_AMOUNT_EXCEEDED", "Refund or withdrawal exceeds the unrecovered original amount")
		}
	}
	if input.PlanLineID != nil {
		kind := ""
		switch input.Kind {
		case "income":
			kind = "income"
		case "direct_expense", "refund":
			kind = "spending"
		case "allocation", "allocation_withdrawal":
			kind = "allocation"
		default:
			return EntryInput{}, Invalid("This entry kind has no recognized-spending plan contribution")
		}
		if err := s.validateEntryPlanLine(month, input.PlanLineID, kind, input); err != nil {
			return EntryInput{}, err
		}
	}
	if input.FundingPlanLineID != nil {
		sourceFunding := input.SourceAccountID != nil && s.FundingIncluded(*input.SourceAccountID, month)
		destinationFunding := input.DestinationAccountID != nil && s.FundingIncluded(*input.DestinationAccountID, month)
		if sourceFunding == destinationFunding {
			return EntryInput{}, Invalid("A cash plan match requires a funding-boundary movement")
		}
		kind := "funding_outflow"
		if destinationFunding {
			kind = "funding_inflow"
		}
		if err := s.validateEntryPlanLine(month, input.FundingPlanLineID, kind, input); err != nil {
			return EntryInput{}, err
		}
	}
	return input, nil
}

func (s *State) CreateEntry(input EntryInput) (Entry, error) {
	input, err := s.validateEntry(input, "")
	if err != nil {
		return Entry{}, err
	}
	entry := Entry{Meta: NewMeta(), EntryInput: input, Active: true}
	s.Entries[entry.ID] = entry
	s.BumpMonth(entry.ReportingMonth)
	s.Audit("entry.created", entry.ID, input.ReportingMonthReason)
	return entry, nil
}

func (s *State) CorrectEntry(id string, input EntryCorrection, expected int64) (Entry, error) {
	original, ok := s.Entries[id]
	if !ok {
		return Entry{}, NotFound()
	}
	if err := CheckVersion(original.Version, expected); err != nil {
		return Entry{}, err
	}
	if !original.Active {
		return Entry{}, Fail(409, "ENTRY_INACTIVE", "Only an active entry can be corrected")
	}
	if strings.TrimSpace(input.Reason) == "" {
		return Entry{}, Invalid("Correction requires a reason")
	}
	if err := s.EnsureOpen(original.ReportingMonth); err != nil {
		return Entry{}, err
	}
	if s.entryHasLinks(id) {
		return Entry{}, Fail(409, "ENTRY_LINKED", "Reconcile linked records before correcting this entry")
	}
	var replacement Entry
	if input.Replacement != nil {
		validated, err := s.validateEntry(*input.Replacement, id)
		if err != nil {
			return Entry{}, err
		}
		replacement = Entry{Meta: NewMeta(), EntryInput: validated, Active: true, SupersedesID: &id, CorrectionReason: input.Reason}
	}
	original.Active = false
	original.CorrectionReason = input.Reason
	original.Touch()
	s.Entries[id] = original
	s.BumpMonth(original.ReportingMonth)
	if input.Replacement != nil {
		s.Entries[replacement.ID] = replacement
		if replacement.ReportingMonth != original.ReportingMonth {
			s.BumpMonth(replacement.ReportingMonth)
		}
		s.Audit("entry.corrected", replacement.ID, input.Reason)
		return replacement, nil
	}
	s.Audit("entry.voided", id, input.Reason)
	return original, nil
}

func (s *State) entryHasLinks(id string) bool {
	for _, e := range s.Entries {
		if e.Active && e.RelatedEntryID != nil && *e.RelatedEntryID == id {
			return true
		}
	}
	for _, settlement := range s.Settlements {
		if settlement.EntryID == id {
			return true
		}
	}
	for _, match := range s.ReceiptMatches {
		if match.ReceiptEntryID == id {
			return true
		}
	}
	for _, claim := range s.Claims {
		if claim.ExpenseEntryID != nil && *claim.ExpenseEntryID == id {
			return true
		}
	}
	for _, adjustment := range s.ObligationAdjustments {
		if adjustment.RefundEntryID != nil && *adjustment.RefundEntryID == id {
			return true
		}
	}
	return false
}

func (s *State) validateEntryPlanLine(month string, id *string, kind string, input EntryInput) error {
	if !ValidID(*id) {
		return Invalid("Plan line ID must be a UUID")
	}
	for _, line := range s.Plans[month].Lines {
		if line.ID == *id {
			if line.Kind != kind {
				return Invalid("Plan line kind is incompatible with this entry")
			}
			if line.AccountID != nil {
				accountID := *line.AccountID
				var matches bool
				switch kind {
				case "income":
					matches = input.DestinationAccountID != nil && *input.DestinationAccountID == accountID
				case "spending":
					matches = input.SourceAccountID != nil && *input.SourceAccountID == accountID
					if input.Kind == "refund" && input.RelatedEntryID != nil {
						original := s.Entries[*input.RelatedEntryID]
						matches = original.SourceAccountID != nil && *original.SourceAccountID == accountID
					}
				case "allocation":
					matchedAccount := input.DestinationAccountID
					if input.Kind == "allocation_withdrawal" {
						matchedAccount = input.SourceAccountID
					}
					matches = matchedAccount != nil && *matchedAccount == accountID
				case "funding_inflow", "funding_outflow":
					matches = (input.SourceAccountID != nil && *input.SourceAccountID == accountID) || (input.DestinationAccountID != nil && *input.DestinationAccountID == accountID)
					if s.Accounts[accountID].Kind == "credit_card" {
						matches = input.Kind == "card_settlement" && input.DestinationAccountID != nil && *input.DestinationAccountID == accountID
					}
				}
				if !matches {
					return Invalid("Plan line account is incompatible with this entry")
				}
			}
			return nil
		}
	}
	return NotFound()
}
