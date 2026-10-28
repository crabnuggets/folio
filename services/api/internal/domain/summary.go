package domain

import (
	"math/big"
	"sort"
	"time"
)

// summarySum uses arbitrary precision internally so aggregation cannot wrap int64.
type summarySum struct{ n big.Int }

// add accumulates minor units without limiting the intermediate total to int64.
func (a *summarySum) add(v Money) { a.n.Add(&a.n, big.NewInt(int64(v))) }

// sub subtracts minor units without limiting the intermediate total to int64.
func (a *summarySum) sub(v Money) { a.n.Sub(&a.n, big.NewInt(int64(v))) }

// value returns the minor-unit total or a validation error if it exceeds int64.
func (a *summarySum) value() (Money, error) {
	if !a.n.IsInt64() {
		return 0, Invalid("Financial total exceeds the supported amount range")
	}
	return Money(a.n.Int64()), nil
}

// summaryMoney sums minor units, rejecting only a final total outside int64.
func summaryMoney(values ...Money) (Money, error) {
	var a summarySum
	for _, v := range values {
		a.add(v)
	}
	return a.value()
}

// summarySubtract returns a minus b in minor units, or a validation error on overflow.
func summarySubtract(a, b Money) (Money, error) {
	var sum summarySum
	sum.add(a)
	sum.sub(b)
	return sum.value()
}

// summaryPointer makes a known minor-unit amount available as an optional value.
func summaryPointer(v Money) *Money { return &v }

// Summarize computes a month's spending, funding, coverage, and forecasts without
// changing state. Amounts use minor units; incomplete spending coverage retains
// labeled subtotals and leaves final surplus nil. Projections with unknown inputs remain nil.
// Savings rate is a ratio rounded to six decimal places, present only with complete
// spending coverage and positive income. Snapshots are the latest observations per
// liquid account strictly before the next month in the configured timezone.
// Invalid months, currencies, timezones, allocations, spending inputs, signed plan
// estimates without explicit remaining amounts, and out-of-range totals return errors.
func (s *State) Summarize(month string) (Summary, error) {
	m, err := s.GetMonth(month)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{Month: month, Currency: s.Settings.Currency, Revision: m.Revision, MissingItems: []string{}, Statements: []StatementBalance{}, ForecastLines: []ForecastLine{}, Snapshots: []Snapshot{}, SpendingComplete: true, FundingComplete: true}
	missing := map[string]bool{}
	forecastSpendingKnown, forecastFundingInputsKnown := true, true
	need := func(code string, spending, funding bool) {
		missing[code] = true
		if spending {
			out.SpendingComplete = false
		}
		if funding {
			out.FundingComplete = false
		}
	}
	if !m.Checklist.IncomeReviewed {
		need("income_not_reviewed", true, true)
	}
	if !m.Checklist.CardsReviewed {
		need("cards_not_reviewed", true, true)
	}
	if !m.Checklist.ExpensesReviewed {
		need("expenses_not_reviewed", true, true)
	}
	if !m.Checklist.AllocationsReviewed {
		need("allocations_not_reviewed", true, true)
	}
	var income, gross, allocations, recoveries, inflows, outflows, unmatchedClaims, unmatchedReceipts summarySum
	actual := map[string]*summarySum{}
	addActual := func(id *string, amount Money) {
		if id == nil {
			return
		}
		if actual[*id] == nil {
			actual[*id] = &summarySum{}
		}
		actual[*id].add(amount)
	}
	for _, id := range sortedKeys(s.Entries) {
		e := s.Entries[id]
		if !e.Active || e.ReportingMonth != month {
			continue
		}
		if err := s.ValidateCurrency(e.Currency); err != nil {
			return Summary{}, err
		}
		recognized := Money(0)
		switch e.Kind {
		case "income":
			income.add(e.AmountMinor)
			recognized = e.AmountMinor
		case "direct_expense":
			gross.add(e.AmountMinor)
			recognized = e.AmountMinor
		case "refund":
			gross.sub(e.AmountMinor)
			recognized = -e.AmountMinor
		case "allocation":
			allocations.add(e.AmountMinor)
			recognized = e.AmountMinor
		case "allocation_withdrawal":
			allocations.sub(e.AmountMinor)
			recognized = -e.AmountMinor
		}
		addActual(e.PlanLineID, recognized)
		sourceInside, destinationInside := false, false
		if e.SourceAccountID != nil {
			sourceInside = s.FundingIncluded(*e.SourceAccountID, month)
		}
		if e.DestinationAccountID != nil {
			destinationInside = s.FundingIncluded(*e.DestinationAccountID, month)
		}
		if sourceInside && !destinationInside {
			outflows.add(e.AmountMinor)
			addActual(e.FundingPlanLineID, e.AmountMinor)
		}
		if destinationInside && !sourceInside {
			inflows.add(e.AmountMinor)
			addActual(e.FundingPlanLineID, e.AmountMinor)
		}
	}
	cardsWithStatement := map[string]bool{}
	for _, id := range sortedKeys(s.Statements) {
		st := s.Statements[id]
		if !st.Active {
			continue
		}
		if st.ReportingMonth == month {
			cardsWithStatement[st.AccountID] = true
			value, e := s.StatementSpending(st)
			if e != nil {
				return Summary{}, e
			}
			if value == nil {
				need("statement_spending_unknown:"+id, true, false)
				forecastSpendingKnown = false
			} else {
				gross.add(*value)
				addActual(st.PlanLineID, *value)
			}
		}
		if st.ReportingMonth == month || st.DueDate[:7] == month {
			balance, e := s.StatementBalance(st)
			if e != nil {
				return Summary{}, e
			}
			out.Statements = append(out.Statements, balance)
		}
	}
	// Closed reviews retain the card inventory reviewed by the owner. Later
	// account setup and archival must not silently change historical coverage.
	requiredCards := m.ReviewedCardIDs
	if m.State != "closed" {
		requiredCards = []string{}
		for _, id := range sortedKeys(s.Accounts) {
			a := s.Accounts[id]
			if a.Kind == "credit_card" && a.Active {
				requiredCards = append(requiredCards, id)
			}
		}
	}
	for _, id := range requiredCards {
		if !cardsWithStatement[id] && m.Checklist.NoStatementReasons[id] == "" {
			need("statement_missing:"+id, true, true)
			forecastSpendingKnown, forecastFundingInputsKnown = false, false
		}
	}
	// Receipt allocations and personal-cost recovery are distinct. A cash receipt can
	// be matched to a claim while its recognized-cost classification is still unknown.
	claimAllocated := map[string]*summarySum{}
	receiptRecovered := map[string]*summarySum{}
	for _, id := range sortedKeys(s.ReceiptMatches) {
		match := s.ReceiptMatches[id]
		if claimAllocated[match.ClaimID] == nil {
			claimAllocated[match.ClaimID] = &summarySum{}
		}
		receipt, exists := s.Entries[match.ReceiptEntryID]
		if exists && receipt.Active && receipt.ReportingMonth <= month {
			claimAllocated[match.ClaimID].add(match.AmountMinor)
		}
		if match.RecoveryMonth != nil {
			if receiptRecovered[match.ReceiptEntryID] == nil {
				receiptRecovered[match.ReceiptEntryID] = &summarySum{}
			}
			receiptRecovered[match.ReceiptEntryID].add(match.AmountMinor)
			if *match.RecoveryMonth == month {
				recoveries.add(match.AmountMinor)
			}
		}
	}
	for _, id := range sortedKeys(s.Claims) {
		claim := s.Claims[id]
		if claim.State == "waived" || s.ClaimMonth(claim) != month {
			continue
		}
		var outstanding summarySum
		outstanding.add(claim.AmountMinor)
		if matched := claimAllocated[id]; matched != nil {
			outstanding.n.Sub(&outstanding.n, &matched.n)
		}
		if outstanding.n.Sign() < 0 {
			return Summary{}, Invalid("Claim allocations exceed the claim amount")
		}
		value, e := outstanding.value()
		if e != nil {
			return Summary{}, e
		}
		unmatchedClaims.add(value)
	}
	for _, id := range sortedKeys(s.Entries) {
		e := s.Entries[id]
		if !e.Active || e.Kind != "reimbursement_receipt" || e.ReportingMonth != month {
			continue
		}
		var pending summarySum
		pending.add(e.AmountMinor)
		if matched := receiptRecovered[id]; matched != nil {
			pending.n.Sub(&pending.n, &matched.n)
		}
		if pending.n.Sign() < 0 {
			return Summary{}, Invalid("Recovery allocations exceed the receipt amount")
		}
		value, e2 := pending.value()
		if e2 != nil {
			return Summary{}, e2
		}
		unmatchedReceipts.add(value)
		if value > 0 {
			need("receipt_cost_unmatched:"+id, true, false)
			forecastSpendingKnown = false
		}
	}
	if out.IncomeMinor, err = income.value(); err != nil {
		return Summary{}, err
	}
	if out.GrossSpendingMinor, err = gross.value(); err != nil {
		return Summary{}, err
	}
	if out.AllocationMinor, err = allocations.value(); err != nil {
		return Summary{}, err
	}
	if out.ConfirmedRecoveriesMinor, err = recoveries.value(); err != nil {
		return Summary{}, err
	}
	if out.KnownSpendingMinor, err = summarySubtract(out.GrossSpendingMinor, out.ConfirmedRecoveriesMinor); err != nil {
		return Summary{}, err
	}
	out.NetPersonalCostMinor = out.KnownSpendingMinor
	if out.FundingInflowsMinor, err = inflows.value(); err != nil {
		return Summary{}, err
	}
	if out.FundingOutflowsMinor, err = outflows.value(); err != nil {
		return Summary{}, err
	}
	if out.FundingRemainderMinor, err = summarySubtract(out.FundingInflowsMinor, out.FundingOutflowsMinor); err != nil {
		return Summary{}, err
	}
	if out.UnmatchedClaimsMinor, err = unmatchedClaims.value(); err != nil {
		return Summary{}, err
	}
	if out.UnmatchedReceiptsMinor, err = unmatchedReceipts.value(); err != nil {
		return Summary{}, err
	}
	var surplus summarySum
	surplus.add(out.IncomeMinor)
	surplus.sub(out.KnownSpendingMinor)
	surplus.sub(out.AllocationMinor)
	if out.ProvisionalSurplusMinor, err = surplus.value(); err != nil {
		return Summary{}, err
	}
	consumption, e := summarySubtract(out.IncomeMinor, out.KnownSpendingMinor)
	if e != nil {
		return Summary{}, e
	}
	if err = s.applyForecast(month, actual, &out, forecastSpendingKnown, forecastFundingInputsKnown); err != nil {
		return Summary{}, err
	}
	if out.SpendingComplete {
		out.MonthlySurplusMinor = summaryPointer(out.ProvisionalSurplusMinor)
		out.ConsumptionSavingsMinor = summaryPointer(consumption)
		if out.IncomeMinor > 0 {
			// The API exposes a ratio rounded to six decimal places, never a percent.
			rate := new(big.Rat).SetFrac(big.NewInt(int64(consumption)), big.NewInt(int64(out.IncomeMinor))).FloatString(6)
			out.SavingsRate = &rate
		}
	}
	// Snapshots remain dated observations, never a reconstructed available balance.
	location, e := time.LoadLocation(s.Settings.Timezone)
	if e != nil {
		return Summary{}, Invalid("Invalid configured timezone")
	}
	start, e := time.ParseInLocation("2006-01", month, location)
	if e != nil {
		return Summary{}, Invalid("Invalid reporting month")
	}
	end := start.AddDate(0, 1, 0)
	latest := map[string]Snapshot{}
	for _, id := range sortedKeys(s.Snapshots) {
		snap := s.Snapshots[id]
		account, ok := s.Accounts[snap.AccountID]
		if !ok || !account.IncludeInLiquidCash || !snap.ObservedAt.Before(end) {
			continue
		}
		prior, ok := latest[snap.AccountID]
		if !ok || snap.ObservedAt.After(prior.ObservedAt) || snap.ObservedAt.Equal(prior.ObservedAt) && snap.ID > prior.ID {
			latest[snap.AccountID] = snap
		}
	}
	for _, id := range sortedKeys(latest) {
		out.Snapshots = append(out.Snapshots, latest[id])
	}
	for code := range missing {
		out.MissingItems = append(out.MissingItems, code)
	}
	sort.Strings(out.MissingItems)
	out.IsProvisional = !out.SpendingComplete || !out.FundingComplete
	return out, nil
}

// applyForecast extends out with plan actuals keyed by line ID and remaining
// minor-unit estimates, updating coverage for pending lines. The known-input flags
// control whether projections are populated; unpaid cards also require explicit
// funding plans. Invalid months, ambiguous signed estimates, statement-balance
// errors, and out-of-range totals return errors; out may be partially updated.
func (s *State) applyForecast(month string, actual map[string]*summarySum, out *Summary, spendingInputsKnown, fundingInputsKnown bool) error {
	plan, err := s.GetPlan(month)
	if err != nil {
		return err
	}
	var forecastSurplus, forecastFunding summarySum
	forecastSurplus.add(out.ProvisionalSurplusMinor)
	forecastFunding.add(out.FundingRemainderMinor)
	fundingCardPlan := map[string]bool{}
	lines := append([]PlanLine{}, plan.Lines...)
	sort.Slice(lines, func(i, j int) bool { return lines[i].ID < lines[j].ID })
	for _, line := range lines {
		value := Money(0)
		if a := actual[line.ID]; a != nil {
			if value, err = a.value(); err != nil {
				return err
			}
		}
		remaining := Money(0)
		if !line.Complete {
			if line.EstimatedRemainingMinor != nil {
				remaining = *line.EstimatedRemainingMinor
			} else {
				if line.ExpectedMinor < 0 || value < 0 {
					return Invalid("Signed plan actuals require an explicit remaining estimate")
				}
				if line.ExpectedMinor > value {
					remaining = line.ExpectedMinor - value
				}
			}
		}
		if !line.Complete && remaining != 0 {
			out.MissingItems = append(out.MissingItems, "planned_item_pending:"+line.ID)
			switch line.Kind {
			case "income", "spending", "allocation":
				out.SpendingComplete = false
			case "funding_inflow", "funding_outflow":
				out.FundingComplete = false
			}
		}
		total, e := summaryMoney(value, remaining)
		if e != nil {
			return e
		}
		out.ForecastLines = append(out.ForecastLines, ForecastLine{ID: line.ID, Kind: line.Kind, ActualMinor: value, RemainingMinor: remaining, ForecastMinor: total})
		switch line.Kind {
		case "income":
			forecastSurplus.add(remaining)
		case "spending", "allocation":
			forecastSurplus.sub(remaining)
		case "funding_inflow":
			forecastFunding.add(remaining)
		case "funding_outflow":
			forecastFunding.sub(remaining)
			if line.AccountID != nil {
				a := s.Accounts[*line.AccountID]
				if a.Kind == "credit_card" {
					fundingCardPlan[a.ID] = true
				}
			}
		}
	}
	// An unpaid statement does not identify the cash account that will settle it.
	// Require an explicit card-specific boundary cash plan rather than assuming
	// the payment will cross the funding boundary.
	forecastFundingKnown := true
	for _, id := range sortedKeys(s.Statements) {
		st := s.Statements[id]
		if !st.Active || !st.PayableActive || st.DueDate[:7] != month || fundingCardPlan[st.AccountID] {
			continue
		}
		balance, e := s.StatementBalance(st)
		if e != nil {
			return e
		}
		if balance.RemainingMinor > 0 {
			forecastFundingKnown = false
			out.MissingItems = append(out.MissingItems, "obligation_funding_plan_missing:"+id)
		}
	}
	projectedSurplus, e := forecastSurplus.value()
	if e != nil {
		return e
	}
	projectedFunding, e := forecastFunding.value()
	if e != nil {
		return e
	}
	if spendingInputsKnown {
		out.ProjectedSurplusMinor = summaryPointer(projectedSurplus)
	}
	if fundingInputsKnown && forecastFundingKnown {
		out.ProjectedFundingRemainderMinor = summaryPointer(projectedFunding)
	}
	return nil
}

// Insights returns applicable coverage, shortfall, and pending-claim notices with
// minor-unit values, or an empty slice when none apply. Summary errors propagate.
func (s *State) Insights(month string) ([]Insight, error) {
	summary, err := s.Summarize(month)
	if err != nil {
		return nil, err
	}
	insights := []Insight{}
	if summary.IsProvisional {
		insights = append(insights, Insight{Code: "INCOMPLETE_COVERAGE", Message: "Review the missing items before treating this month as complete.", Values: map[string]Money{"known_spending_minor": summary.KnownSpendingMinor}})
	}
	if summary.FundingRemainderMinor < 0 {
		insights = append(insights, Insight{Code: "FUNDING_SHORTFALL", Message: "Recorded funding outflows exceed recorded inflows; this excludes opening cash.", Values: map[string]Money{"funding_remainder_minor": summary.FundingRemainderMinor}})
	}
	if summary.MonthlySurplusMinor != nil && *summary.MonthlySurplusMinor < 0 {
		insights = append(insights, Insight{Code: "SPENDING_SHORTFALL", Message: "Recorded income is below spending and completed allocations.", Values: map[string]Money{"monthly_surplus_minor": *summary.MonthlySurplusMinor}})
	}
	if summary.UnmatchedClaimsMinor > 0 {
		insights = append(insights, Insight{Code: "PENDING_CLAIMS", Message: "Unreceived claims are excluded from actual income and cash funding.", Values: map[string]Money{"unmatched_claims_minor": summary.UnmatchedClaimsMinor}})
	}
	return insights, nil
}
