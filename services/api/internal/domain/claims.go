package domain

import (
	"math/big"
	"strings"
)

// validateClaim defaults an omitted state to pending and checks the amount,
// currency, description, receipt date, and optional cost link. Invalid inputs and
// missing linked expenses or statements return errors.
func (s *State) validateClaim(in ClaimInput) (ClaimInput, error) {
	if in.Currency != s.Settings.Currency || in.AmountMinor <= 0 {
		return in, Invalid("Claim requires a positive amount in the configured currency")
	}
	if strings.TrimSpace(in.Description) == "" {
		return in, Invalid("Claim description is required")
	}
	if in.ExpectedReceiptDate != nil && !ValidDate(*in.ExpectedReceiptDate) {
		return in, Invalid("Invalid expected receipt date")
	}
	if in.ExpenseEntryID != nil && in.StatementID != nil {
		return in, Invalid("A claim must reference at most one recognized cost")
	}
	if in.ExpenseEntryID != nil {
		entry, ok := s.Entries[*in.ExpenseEntryID]
		if !ok {
			return in, NotFound()
		}
		if !entry.Active || entry.Kind != "direct_expense" || entry.Currency != in.Currency {
			return in, Invalid("Claim expense must be an active direct expense")
		}
	}
	if in.StatementID != nil {
		st, ok := s.Statements[*in.StatementID]
		if !ok {
			return in, NotFound()
		}
		if !st.Active || st.Currency != in.Currency {
			return in, Invalid("Claim statement must be active and of the same currency")
		}
	}
	if in.State == "" {
		in.State = "pending"
	}
	switch in.State {
	case "pending", "partially_received", "received", "disputed", "waived":
	default:
		return in, Invalid("Invalid claim state")
	}
	return in, nil
}

// CreateClaim stores and audits an expected reimbursement without posting cash,
// advancing its anchor month when dated or linked. It propagates validation and
// closed-anchor-month errors and rejects receipt states without confirmed matches.
func (s *State) CreateClaim(in ClaimInput) (Claim, error) {
	in, e := s.validateClaim(in)
	if e != nil {
		return Claim{}, e
	}
	if in.State == "received" || in.State == "partially_received" {
		return Claim{}, Invalid("Receipt state requires confirmed receipt matches")
	}
	claim := Claim{Meta: NewMeta(), ClaimInput: in}
	if month := s.ClaimMonth(claim); month != "" {
		if e := s.EnsureOpen(month); e != nil {
			return Claim{}, e
		}
	}
	s.Claims[claim.ID] = claim
	s.AuditChange("claim.created", claim.ID, "", nil, claim)
	s.bumpClaimMonths(claim)
	return claim, nil
}

// claimMatchedAmount totals receipt allocations in minor units, including matches
// whose recognized-cost recovery has not yet been classified.
func (s *State) claimMatchedAmount(id string) *big.Int {
	total := new(big.Int)
	for _, match := range s.ReceiptMatches {
		if match.ClaimID == id {
			total.Add(total, big.NewInt(int64(match.AmountMinor)))
		}
	}
	return total
}

// ClaimMonth anchors a claim to its expected receipt month, otherwise its
// recognized cost month. Undated, unlinked claims remain in the claim backlog.
func (s *State) ClaimMonth(claim Claim) string {
	if claim.ExpectedReceiptDate != nil {
		return (*claim.ExpectedReceiptDate)[:7]
	}
	if claim.ExpenseEntryID != nil {
		return s.Entries[*claim.ExpenseEntryID].ReportingMonth
	}
	if claim.StatementID != nil {
		return s.Statements[*claim.StatementID].ReportingMonth
	}
	return ""
}

// bumpClaimMonths advances the claim's anchor month when it has one.
func (s *State) bumpClaimMonths(claim Claim) {
	if month := s.ClaimMonth(claim); month != "" {
		s.BumpMonth(month)
	}
}

// UpdateClaim replaces and audits a claim at the expected version and advances
// affected anchor months. Receipt states are reconciled with confirmed allocations.
// It rejects missing or stale claims, invalid inputs, closed anchor months, amounts
// below allocations, and cost-link changes after a classified recovery.
func (s *State) UpdateClaim(id string, in ClaimInput, expected int64) (Claim, error) {
	old, ok := s.Claims[id]
	if !ok {
		return Claim{}, NotFound()
	}
	if e := CheckVersion(old.Version, expected); e != nil {
		return Claim{}, e
	}
	in, e := s.validateClaim(in)
	if e != nil {
		return Claim{}, e
	}
	oldMonth := s.ClaimMonth(old)
	newMonth := s.ClaimMonth(Claim{ClaimInput: in})
	for _, month := range []string{oldMonth, newMonth} {
		if month != "" {
			if e := s.EnsureOpen(month); e != nil {
				return Claim{}, e
			}
		}
	}
	matched := s.claimMatchedAmount(id)
	if matched.Cmp(big.NewInt(int64(in.AmountMinor))) > 0 {
		return Claim{}, Fail(409, "CLAIM_LIMIT", "Claim amount is below received allocations")
	}
	if matched.Sign() > 0 && (!sameStatementRef(old.ExpenseEntryID, in.ExpenseEntryID) || !sameStatementRef(old.StatementID, in.StatementID)) {
		for _, match := range s.ReceiptMatches {
			if match.ClaimID == id && match.RecoveryMonth != nil {
				return Claim{}, Fail(409, "LINKED_RECORD", "A recovered claim cannot change its cost link")
			}
		}
	}
	switch in.State {
	case "pending", "partially_received", "received":
		if matched.Sign() == 0 {
			if in.State != "pending" {
				return Claim{}, Invalid("Claim receipt state must match confirmed receipts")
			}
		} else if matched.Cmp(big.NewInt(int64(in.AmountMinor))) == 0 {
			in.State = "received"
		} else {
			in.State = "partially_received"
		}
	}
	before := old
	old.ClaimInput = in
	old.Touch()
	s.Claims[id] = old
	s.AuditChange("claim.updated", id, "", before, old)
	s.bumpClaimMonths(old)
	if oldMonth != "" && oldMonth != newMonth {
		s.BumpMonth(oldMonth)
	}
	return old, nil
}

// MatchReceipt allocates a confirmed receipt and updates and audits the claim's
// receipt state. expected is the claim version. A known cost defaults recovery to
// the receipt's reporting month; an unknown cost permits allocation without recovery.
// An existing unclassified allocation can gain recovery at the same amount and ID.
// Missing records, stale versions, disputed or waived claims, invalid or duplicate
// links, closed affected months, and claim, receipt, or cost caps return errors;
// statement-spending errors propagate. No new cash entry is posted.
func (s *State) MatchReceipt(claimID string, in ReceiptMatchInput, expected int64) (ReceiptMatch, error) {
	claim, ok := s.Claims[claimID]
	if !ok {
		return ReceiptMatch{}, NotFound()
	}
	if e := CheckVersion(claim.Version, expected); e != nil {
		return ReceiptMatch{}, e
	}
	if claim.State == "waived" || claim.State == "disputed" {
		return ReceiptMatch{}, Fail(409, "CLAIM_STATE", "Resolve the claim state before matching receipts")
	}
	receipt, ok := s.Entries[in.ReceiptEntryID]
	if !ok {
		return ReceiptMatch{}, NotFound()
	}
	if !receipt.Active || receipt.Kind != "reimbursement_receipt" || receipt.Currency != claim.Currency {
		return ReceiptMatch{}, Invalid("Match requires an active reimbursement receipt of the same currency")
	}
	if in.AmountMinor <= 0 {
		return ReceiptMatch{}, Invalid("Receipt match amount must be positive")
	}
	if e := s.EnsureOpen(receipt.ReportingMonth); e != nil {
		return ReceiptMatch{}, e
	}
	var existing *ReceiptMatch
	for _, match := range s.ReceiptMatches {
		if match.ReceiptEntryID == receipt.ID && match.ClaimID == claimID {
			if match.RecoveryMonth != nil || in.RecoveryMonth == nil || in.AmountMinor != match.AmountMinor {
				return ReceiptMatch{}, Fail(409, "RECEIPT_MATCH_EXISTS", "Receipt is already allocated to this claim")
			}
			previous := match
			existing = &previous
		}
	}
	promoting := existing != nil
	anchorMonth := s.ClaimMonth(claim)
	affectsAnchor := !promoting && anchorMonth != "" && receipt.ReportingMonth <= anchorMonth
	if affectsAnchor {
		if e := s.EnsureOpen(anchorMonth); e != nil {
			return ReceiptMatch{}, e
		}
	}
	claimUsed := s.claimMatchedAmount(claimID)
	receiptUsed := new(big.Int)
	for _, match := range s.ReceiptMatches {
		if match.ReceiptEntryID == receipt.ID {
			receiptUsed.Add(receiptUsed, big.NewInt(int64(match.AmountMinor)))
		}
	}
	if !promoting {
		claimUsed.Add(claimUsed, big.NewInt(int64(in.AmountMinor)))
		receiptUsed.Add(receiptUsed, big.NewInt(int64(in.AmountMinor)))
	}
	if claimUsed.Cmp(big.NewInt(int64(claim.AmountMinor))) > 0 {
		return ReceiptMatch{}, Fail(409, "CLAIM_LIMIT", "Receipt exceeds the claim outstanding amount")
	}
	if receiptUsed.Cmp(big.NewInt(int64(receipt.AmountMinor))) > 0 {
		return ReceiptMatch{}, Fail(409, "RECEIPT_LIMIT", "Allocations exceed the received amount")
	}
	var cost *Money
	if claim.ExpenseEntryID != nil {
		entry, exists := s.Entries[*claim.ExpenseEntryID]
		if !exists || !entry.Active || entry.Kind != "direct_expense" {
			return ReceiptMatch{}, Invalid("Claim cost is no longer an active expense")
		}
		available := big.NewInt(int64(entry.AmountMinor))
		for _, v := range s.Entries {
			if v.Active && v.Kind == "refund" && v.RelatedEntryID != nil && *v.RelatedEntryID == entry.ID {
				available.Sub(available, big.NewInt(int64(v.AmountMinor)))
			}
		}
		if !available.IsInt64() {
			return ReceiptMatch{}, Invalid("Expense amount exceeds signed 64-bit range")
		}
		n := Money(available.Int64())
		cost = &n
	}
	if claim.StatementID != nil {
		st, exists := s.Statements[*claim.StatementID]
		if !exists || !st.Active {
			return ReceiptMatch{}, Invalid("Claim statement is no longer active")
		}
		var e error
		cost, e = s.StatementSpending(st)
		if e != nil {
			return ReceiptMatch{}, e
		}
	}
	if cost == nil {
		if in.RecoveryMonth != nil {
			return ReceiptMatch{}, Invalid("Recovery requires a known recognized cost")
		}
	} else {
		if in.RecoveryMonth == nil {
			month := receipt.ReportingMonth
			in.RecoveryMonth = &month
		}
		if e := s.EnsureOpen(*in.RecoveryMonth); e != nil {
			return ReceiptMatch{}, e
		}
		used := big.NewInt(int64(in.AmountMinor))
		for _, match := range s.ReceiptMatches {
			if match.RecoveryMonth == nil {
				continue
			}
			other := s.Claims[match.ClaimID]
			sameCost := (claim.ExpenseEntryID != nil && sameStatementRef(claim.ExpenseEntryID, other.ExpenseEntryID)) || (claim.StatementID != nil && sameStatementRef(claim.StatementID, other.StatementID))
			if sameCost {
				used.Add(used, big.NewInt(int64(match.AmountMinor)))
			}
		}
		if used.Cmp(big.NewInt(int64(*cost))) > 0 {
			return ReceiptMatch{}, Fail(409, "RECOVERY_LIMIT", "Recovery exceeds the unrecovered recognized cost")
		}
	}
	beforeClaim := claim
	match := ReceiptMatch{Meta: NewMeta(), ReceiptMatchInput: in, ClaimID: claimID}
	if promoting {
		match = *existing
		match.ReceiptMatchInput = in
		match.Touch()
	}
	s.ReceiptMatches[match.ID] = match
	if claimUsed.Cmp(big.NewInt(int64(claim.AmountMinor))) == 0 {
		claim.State = "received"
	} else {
		claim.State = "partially_received"
	}
	claim.Touch()
	s.Claims[claimID] = claim
	if promoting {
		s.AuditChange("claim.receipt_recovery_classified", match.ID, "Classified previously unmatched recovery", existing, match)
	} else {
		s.AuditChange("claim.receipt_matched", match.ID, "", nil, match)
	}
	s.AuditChange("claim.receipt_state_changed", claim.ID, "Confirmed receipt match", beforeClaim, claim)
	s.BumpMonth(receipt.ReportingMonth)
	if in.RecoveryMonth != nil && *in.RecoveryMonth != receipt.ReportingMonth {
		s.BumpMonth(*in.RecoveryMonth)
	}
	if affectsAnchor && anchorMonth != receipt.ReportingMonth && (in.RecoveryMonth == nil || anchorMonth != *in.RecoveryMonth) {
		s.BumpMonth(anchorMonth)
	}
	return match, nil
}
