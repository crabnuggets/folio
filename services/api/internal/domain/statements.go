package domain

import (
	"math/big"
	"strings"
)

// statementMoney sums minor units and returns a validation error if the final
// result exceeds the signed 64-bit range; intermediate sums may exceed it.
func statementMoney(values ...Money) (Money, error) {
	total := new(big.Int)
	for _, v := range values {
		total.Add(total, big.NewInt(int64(v)))
	}
	if !total.IsInt64() {
		return 0, Invalid("Financial amount exceeds signed 64-bit range")
	}
	return Money(total.Int64()), nil
}

// StatementSpending returns net cycle spending in minor units, or nil when unknown.
// Spending is either an explicit net amount with evidence or charges plus fees
// and interest minus refunds. Mixed or incomplete inputs, negative components,
// missing net evidence, and an out-of-range total return validation errors.
func (s *State) StatementSpending(st Statement) (*Money, error) {
	fields := []*Money{st.NewChargesMinor, st.FeesMinor, st.InterestMinor, st.RefundsMinor}
	n := 0
	for _, v := range fields {
		if v != nil {
			n++
		}
	}
	if st.NetSpendingMinor != nil {
		if n != 0 {
			return nil, Invalid("Use spending components or net spending, not both")
		}
		if strings.TrimSpace(st.SpendingEvidence) == "" {
			return nil, Invalid("Net spending needs evidence")
		}
		v := *st.NetSpendingMinor
		return &v, nil
	}
	if n == 0 {
		return nil, nil
	}
	if n != 4 {
		return nil, Invalid("Supply all four spending components or leave spending unknown")
	}
	for _, v := range fields {
		if *v < 0 {
			return nil, Invalid("Statement spending components cannot be negative")
		}
	}
	v, e := statementMoney(*fields[0], *fields[1], *fields[2], -*fields[3])
	if e != nil {
		return nil, e
	}
	return &v, nil
}

// StatementBalance returns cycle spending and the remaining payable or excess
// credit in minor units, accounting for carry history after the cycle cutoff.
// Inactive payables have zero remaining and credit amounts. Spending validation
// errors and balances or credits outside the signed 64-bit range are returned.
func (s *State) StatementBalance(st Statement) (StatementBalance, error) {
	spending, e := s.StatementSpending(st)
	if e != nil {
		return StatementBalance{}, e
	}
	balance := s.statementPayable(st)
	out := StatementBalance{StatementID: st.ID, AmountDueMinor: st.AmountDueMinor, SpendingMinor: spending, PayableActive: st.PayableActive && st.Active}
	if !out.PayableActive {
		return out, nil
	}
	if balance.Sign() < 0 {
		balance.Neg(balance)
		if !balance.IsInt64() {
			return out, Invalid("Statement credit exceeds signed 64-bit range")
		}
		out.CreditMinor = Money(balance.Int64())
	} else {
		if !balance.IsInt64() {
			return out, Invalid("Statement balance exceeds signed 64-bit range")
		}
		out.RemainingMinor = Money(balance.Int64())
	}
	return out, nil
}

// statementPayable follows carry history but subtracts only payments and credits
// after this statement's cutoff. Earlier movements are already in printed due.
func (s *State) statementPayable(st Statement) *big.Int {
	ids := map[string]bool{st.ID: true}
	priorID := st.CarriesStatementID
	for priorID != nil && !ids[*priorID] {
		ids[*priorID] = true
		prior, ok := s.Statements[*priorID]
		if !ok {
			break
		}
		priorID = prior.CarriesStatementID
	}
	balance := big.NewInt(int64(st.AmountDueMinor))
	for _, v := range s.Settlements {
		if ids[v.StatementID] && s.Entries[v.EntryID].EffectiveDate > st.CycleEnd {
			balance.Sub(balance, big.NewInt(int64(v.AmountMinor)))
		}
	}
	for _, v := range s.ObligationAdjustments {
		if ids[v.StatementID] && v.EffectiveDate > st.CycleEnd {
			balance.Sub(balance, big.NewInt(int64(v.AmountMinor)))
		}
	}
	return balance
}

// checkStatementCredit rejects a payable whose magnitude would exceed the signed
// 64-bit range after subtracting extra minor units of payment or credit.
func (s *State) checkStatementCredit(st Statement, extra Money) error {
	balance := s.statementPayable(st)
	balance.Sub(balance, big.NewInt(int64(extra)))
	balance.Abs(balance)
	if !balance.IsInt64() {
		return Invalid("Statement balance or credit exceeds signed 64-bit range")
	}
	return nil
}

// validateStatement defaults the reporting month to the due month and checks the
// card, dates, amounts, spending evidence, plan match, and carry relationship.
// old identifies the statement being revised, or nil for creation. Invalid inputs,
// missing references, closed affected months, and duplicate active cycles return errors.
func (s *State) validateStatement(in StatementInput, old *Statement) (StatementInput, error) {
	if in.Currency != s.Settings.Currency {
		return in, Invalid("Statement currency must match settings")
	}
	card, ok := s.Accounts[in.AccountID]
	if !ok {
		return in, NotFound()
	}
	if !card.Active || card.Kind != "credit_card" || card.Currency != in.Currency {
		return in, Invalid("Statement account must be an active credit card of the same currency")
	}
	if !ValidDate(in.CycleStart) || !ValidDate(in.CycleEnd) || !ValidDate(in.IssueDate) || !ValidDate(in.DueDate) || in.CycleStart > in.CycleEnd || in.CycleEnd > in.IssueDate || in.IssueDate > in.DueDate {
		return in, Invalid("Invalid statement cycle, issue, or due dates")
	}
	if in.ReportingMonth == "" {
		in.ReportingMonth = in.DueDate[:7]
	}
	if !ValidMonth(in.ReportingMonth) {
		return in, Invalid("Invalid reporting month")
	}
	if in.ReportingMonth != in.DueDate[:7] && strings.TrimSpace(in.ReportingMonthReason) == "" {
		return in, Invalid("A reporting month override requires a reason")
	}
	if e := s.ensureStatementMonthsOpen(in); e != nil {
		return in, e
	}
	if in.AmountDueMinor < 0 {
		return in, Invalid("Statement amount due cannot be negative")
	}
	if in.MinimumDueMinor != nil && (*in.MinimumDueMinor < 0 || *in.MinimumDueMinor > in.AmountDueMinor) {
		return in, Invalid("Minimum due must be between zero and amount due")
	}
	if _, e := s.StatementSpending(Statement{StatementInput: in}); e != nil {
		return in, e
	}
	for id, st := range s.Statements {
		if st.Active && st.AccountID == in.AccountID && (old == nil || id != old.ID) && st.CycleStart == in.CycleStart && st.CycleEnd == in.CycleEnd {
			return in, Fail(409, "STATEMENT_EXISTS", "Revise the existing statement for this cycle")
		}
	}
	if in.PlanLineID != nil {
		if !ValidID(*in.PlanLineID) {
			return in, Invalid("Invalid plan line ID")
		}
		found := false
		for _, line := range s.Plans[in.ReportingMonth].Lines {
			if line.ID == *in.PlanLineID {
				if line.Kind != "spending" {
					return in, Invalid("Statements must match a spending plan line")
				}
				if line.AccountID != nil && *line.AccountID != in.AccountID {
					return in, Invalid("Statement card differs from its plan line account")
				}
				found = true
			}
		}
		if !found {
			return in, NotFound()
		}
	}
	if in.CarriesStatementID != nil {
		prior, ok := s.Statements[*in.CarriesStatementID]
		if !ok {
			return in, NotFound()
		}
		sameCarry := old != nil && old.CarriesStatementID != nil && *old.CarriesStatementID == prior.ID
		if !prior.Active || (!prior.PayableActive && !sameCarry) || prior.AccountID != in.AccountID || prior.CycleEnd >= in.CycleStart || prior.DueDate > in.DueDate {
			return in, Invalid("Carry must reference an earlier active payable on the same card")
		}
		if !sameCarry {
			if e := s.ensureStatementMonthsOpen(prior.StatementInput); e != nil {
				return in, e
			}
		}
	}
	return in, nil
}

// CreateStatement stores and audits an active statement and retires the prior
// payable when carrying it forward, preserving prior spending. It advances affected
// reporting and due months and propagates statement-validation and credit-range errors.
func (s *State) CreateStatement(in StatementInput) (Statement, error) {
	in, e := s.validateStatement(in, nil)
	if e != nil {
		return Statement{}, e
	}
	st := Statement{Meta: NewMeta(), StatementInput: in, Active: true, PayableActive: true}
	if e := s.checkStatementCredit(st, 0); e != nil {
		return Statement{}, e
	}
	if in.CarriesStatementID != nil {
		prior := s.Statements[*in.CarriesStatementID]
		before := prior
		prior.PayableActive = false
		prior.Touch()
		s.Statements[prior.ID] = prior
		s.AuditChange("statement.payable_carried", prior.ID, "Payable represented by "+st.ID, before, prior)
		s.bumpStatementMonths(prior.StatementInput)
	}
	s.Statements[st.ID] = st
	s.AuditChange("statement.created", st.ID, in.ReportingMonthReason, nil, st)
	s.bumpStatementMonths(st.StatementInput)
	return st, nil
}
// sameStatementRef compares optional record IDs, treating two nil references as equal.
func sameStatementRef(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ReviseStatement returns a replacement and retires the original, moving its
// settlements, credits, and claims to the new ID and auditing the changes.
// It advances affected months and rejects missing, stale, inactive, or carried
// source statements, blank reasons, invalid replacements, closed months, changed
// card/cycle/carry links, invalidated associations, and spending below recoveries.
// expected is the original statement's version; credit-range errors also propagate.
func (s *State) ReviseStatement(id string, in StatementRevision, expected int64) (Statement, error) {
	old, ok := s.Statements[id]
	if !ok {
		return Statement{}, NotFound()
	}
	if e := CheckVersion(old.Version, expected); e != nil {
		return Statement{}, e
	}
	if !old.Active {
		return Statement{}, Fail(409, "INACTIVE_RECORD", "Only an active statement can be revised")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return Statement{}, Invalid("Statement revision requires a reason")
	}
	if e := s.ensureStatementMonthsOpen(old.StatementInput); e != nil {
		return Statement{}, e
	}
	if !old.PayableActive {
		return Statement{}, Fail(409, "LINKED_RECORD", "Revise the carrying statement before revising its source")
	}
	replacement, e := s.validateStatement(in.Replacement, &old)
	if e != nil {
		return Statement{}, e
	}
	if replacement.AccountID != old.AccountID || replacement.CycleStart != old.CycleStart || replacement.CycleEnd != old.CycleEnd || !sameStatementRef(replacement.CarriesStatementID, old.CarriesStatementID) {
		return Statement{}, Invalid("A revision cannot change card, cycle, or carry relationship")
	}
	for _, v := range s.Settlements {
		if v.StatementID == id {
			entry := s.Entries[v.EntryID]
			if !entry.Active || entry.EffectiveDate <= replacement.CycleEnd || replacement.ReportingMonth != old.ReportingMonth {
				return Statement{}, Fail(409, "LINKED_RECORD", "Revision invalidates an existing settlement")
			}
		}
	}
	for _, v := range s.ObligationAdjustments {
		if v.StatementID == id && (v.EffectiveDate <= replacement.CycleEnd || replacement.ReportingMonth != old.ReportingMonth) {
			return Statement{}, Fail(409, "LINKED_RECORD", "Revision invalidates an existing obligation adjustment")
		}
	}
	spending, e := s.StatementSpending(Statement{StatementInput: replacement})
	if e != nil {
		return Statement{}, e
	}
	recovered := new(big.Int)
	for _, claim := range s.Claims {
		if claim.StatementID != nil && *claim.StatementID == id {
			if replacement.ReportingMonth != old.ReportingMonth {
				return Statement{}, Fail(409, "LINKED_RECORD", "A claimed statement cannot move reporting month")
			}
			for _, match := range s.ReceiptMatches {
				if match.ClaimID == claim.ID && match.RecoveryMonth != nil {
					recovered.Add(recovered, big.NewInt(int64(match.AmountMinor)))
				}
			}
		}
	}
	if recovered.Sign() > 0 && (spending == nil || big.NewInt(int64(*spending)).Cmp(recovered) < 0) {
		return Statement{}, Fail(409, "RECOVERY_LIMIT", "Revised spending is below confirmed recoveries")
	}
	candidate := old
	candidate.StatementInput = replacement
	if e := s.checkStatementCredit(candidate, 0); e != nil {
		return Statement{}, e
	}
	st := Statement{Meta: NewMeta(), StatementInput: replacement, Active: true, PayableActive: true, SupersedesID: &old.ID, RevisionReason: in.Reason}
	before := old
	old.Active = false
	old.PayableActive = false
	old.Touch()
	s.Statements[id] = old
	s.Statements[st.ID] = st
	for key, v := range s.Settlements {
		if v.StatementID == id {
			linkBefore := v
			v.StatementID = st.ID
			v.Touch()
			s.Settlements[key] = v
			s.AuditChange("settlement.statement_revised", v.ID, in.Reason, linkBefore, v)
		}
	}
	for key, v := range s.ObligationAdjustments {
		if v.StatementID == id {
			linkBefore := v
			v.StatementID = st.ID
			v.Touch()
			s.ObligationAdjustments[key] = v
			s.AuditChange("obligation_adjustment.statement_revised", v.ID, in.Reason, linkBefore, v)
		}
	}
	for key, v := range s.Claims {
		if v.StatementID != nil && *v.StatementID == id {
			linkBefore := v
			newID := st.ID
			v.StatementID = &newID
			v.Touch()
			s.Claims[key] = v
			s.AuditChange("claim.statement_revised", v.ID, in.Reason, linkBefore, v)
		}
	}
	s.AuditChange("statement.revised", st.ID, in.Reason, before, st)
	s.bumpStatementMonths(st.StatementInput, old.ReportingMonth, old.DueDate[:7])
	return st, nil
}

// payableForMutation returns an active payable at the expected statement version.
// Missing records, stale versions, inactive payables, and closed reporting or due
// months return errors.
func (s *State) payableForMutation(id string, expected int64) (Statement, error) {
	st, ok := s.Statements[id]
	if !ok {
		return st, NotFound()
	}
	if e := CheckVersion(st.Version, expected); e != nil {
		return st, e
	}
	if !st.Active || !st.PayableActive {
		return st, Fail(409, "INACTIVE_RECORD", "Use the active payable statement")
	}
	if e := s.ensureStatementMonthsOpen(st.StatementInput); e != nil {
		return st, e
	}
	return st, nil
}

// CreateSettlement allocates an existing card payment to a payable without posting
// another movement or expense. The positive minor-unit amount may exceed the due
// amount, creating credit, but total allocations cannot exceed the payment.
// It advances the statement version and affected months and audits the link.
// Payable lookup/version/month errors, invalid or duplicate payment links, closed
// payment months, allocation caps, and credit-range errors are returned.
func (s *State) CreateSettlement(statementID string, in SettlementInput, expected int64) (Settlement, error) {
	st, e := s.payableForMutation(statementID, expected)
	if e != nil {
		return Settlement{}, e
	}
	entry, ok := s.Entries[in.EntryID]
	if !ok {
		return Settlement{}, NotFound()
	}
	if !entry.Active || entry.Kind != "card_settlement" || entry.Currency != st.Currency || entry.DestinationAccountID == nil || *entry.DestinationAccountID != st.AccountID {
		return Settlement{}, Invalid("Settlement must reference an active payment to the same card")
	}
	if in.AmountMinor <= 0 {
		return Settlement{}, Invalid("Settlement amount must be positive")
	}
	if !ValidDate(entry.EffectiveDate) || entry.EffectiveDate <= st.CycleEnd {
		return Settlement{}, Invalid("Payment already included in statement cutoff cannot be applied again")
	}
	if e := s.EnsureOpen(entry.ReportingMonth); e != nil {
		return Settlement{}, e
	}
	used := big.NewInt(int64(in.AmountMinor))
	for _, v := range s.Settlements {
		if v.EntryID == in.EntryID {
			if v.StatementID == statementID {
				return Settlement{}, Fail(409, "SETTLEMENT_EXISTS", "Payment is already linked to this statement")
			}
			used.Add(used, big.NewInt(int64(v.AmountMinor)))
		}
	}
	if used.Cmp(big.NewInt(int64(entry.AmountMinor))) > 0 {
		return Settlement{}, Fail(409, "SETTLEMENT_LIMIT", "Applied settlements exceed the payment amount")
	}
	if e := s.checkStatementCredit(st, in.AmountMinor); e != nil {
		return Settlement{}, e
	}
	settlement := Settlement{Meta: NewMeta(), SettlementInput: in, StatementID: statementID}
	s.Settlements[settlement.ID] = settlement
	st.Touch()
	s.Statements[st.ID] = st
	s.AuditChange("statement.settled", settlement.ID, "", nil, settlement)
	s.bumpStatementMonths(st.StatementInput, entry.ReportingMonth)
	return settlement, nil
}

// CreateObligationAdjustment records a positive minor-unit credit after the cycle
// cutoff without changing recognized spending. It advances the statement version
// and affected months and audits the credit. expected is the statement version.
// Payable errors, invalid inputs or optional refund links, closed credit months,
// credits exceeding a linked refund, and credit-range errors are returned.
func (s *State) CreateObligationAdjustment(statementID string, in ObligationAdjustmentInput, expected int64) (ObligationAdjustment, error) {
	st, e := s.payableForMutation(statementID, expected)
	if e != nil {
		return ObligationAdjustment{}, e
	}
	if in.AmountMinor <= 0 || !ValidDate(in.EffectiveDate) || in.EffectiveDate <= st.CycleEnd || strings.TrimSpace(in.Reason) == "" {
		return ObligationAdjustment{}, Invalid("Obligation credit needs a positive amount, reason, and date after cutoff")
	}
	if e := s.EnsureOpen(in.EffectiveDate[:7]); e != nil {
		return ObligationAdjustment{}, e
	}
	if in.RefundEntryID != nil {
		entry, ok := s.Entries[*in.RefundEntryID]
		if !ok {
			return ObligationAdjustment{}, NotFound()
		}
		if !entry.Active || entry.Kind != "refund" || entry.Currency != st.Currency || entry.EffectiveDate != in.EffectiveDate {
			return ObligationAdjustment{}, Invalid("Obligation credit must reference a matching confirmed refund")
		}
		used := big.NewInt(int64(in.AmountMinor))
		for _, v := range s.ObligationAdjustments {
			if v.RefundEntryID != nil && *v.RefundEntryID == entry.ID {
				used.Add(used, big.NewInt(int64(v.AmountMinor)))
			}
		}
		if used.Cmp(big.NewInt(int64(entry.AmountMinor))) > 0 {
			return ObligationAdjustment{}, Fail(409, "REFUND_LIMIT", "Obligation credits exceed the refund amount")
		}
	}
	if e := s.checkStatementCredit(st, in.AmountMinor); e != nil {
		return ObligationAdjustment{}, e
	}
	adj := ObligationAdjustment{Meta: NewMeta(), ObligationAdjustmentInput: in, StatementID: statementID}
	s.ObligationAdjustments[adj.ID] = adj
	st.Touch()
	s.Statements[st.ID] = st
	s.AuditChange("statement.obligation_adjusted", adj.ID, in.Reason, nil, adj)
	s.bumpStatementMonths(st.StatementInput, in.EffectiveDate[:7])
	return adj, nil
}

// Spending attribution and obligation due month independently affect reports.
func (s *State) ensureStatementMonthsOpen(in StatementInput) error {
	if err := s.EnsureOpen(in.ReportingMonth); err != nil {
		return err
	}
	return s.EnsureOpen(in.DueDate[:7])
}
// bumpStatementMonths advances each distinct reporting, due, and extra month once.
// Callers must supply validated dates and months.
func (s *State) bumpStatementMonths(in StatementInput, extra ...string) {
	months := map[string]bool{in.ReportingMonth: true, in.DueDate[:7]: true}
	for _, month := range extra {
		months[month] = true
	}
	for _, month := range sortedKeys(months) {
		s.BumpMonth(month)
	}
}
