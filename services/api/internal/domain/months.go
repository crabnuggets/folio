package domain

import (
	"sort"
	"strings"
	"time"
)

var planKinds = map[string]bool{"income": true, "spending": true, "allocation": true, "funding_inflow": true, "funding_outflow": true}

// GetMonth returns a stored review or an unstored open review at version 1.
// Invalid YYYY-MM months return a validation error.
func (s *State) GetMonth(month string) (Month, error) {
	if !ValidMonth(month) {
		return Month{}, Invalid("Invalid reporting month")
	}
	if m, ok := s.Months[month]; ok {
		if m.ReviewedCardIDs == nil {
			m.ReviewedCardIDs = []string{}
		}
		return m, nil
	}
	return Month{Month: month, Version: 1, State: "open", ReviewedCardIDs: []string{}, Checklist: Checklist{NoStatementReasons: map[string]string{}}}, nil
}

// GetPlan returns a stored plan or an unstored empty plan at version 1.
// Invalid YYYY-MM months return a validation error.
func (s *State) GetPlan(month string) (Plan, error) {
	if !ValidMonth(month) {
		return Plan{}, Invalid("Invalid reporting month")
	}
	if p, ok := s.Plans[month]; ok {
		return p, nil
	}
	return Plan{Version: 1, Lines: []PlanLine{}}, nil
}

// PutPlan replaces and audits an open month's plan at the expected plan version.
// It advances the plan and month versions and rejects invalid lines or recurrence
// links, missing references, closed months, stale versions, and removal or
// reclassification of matched lines (PLAN_LINE_IN_USE).
func (s *State) PutPlan(month string, in PlanInput, expected int64) (Plan, error) {
	old, err := s.GetPlan(month)
	if err != nil {
		return Plan{}, err
	}
	if err = s.EnsureOpen(month); err != nil {
		return Plan{}, err
	}
	if err = CheckVersion(old.Version, expected); err != nil {
		return Plan{}, err
	}
	if in.Lines == nil {
		in.Lines = []PlanLine{}
	}
	seen := map[string]PlanLine{}
	recurrences := map[string]bool{}
	for _, line := range in.Lines {
		if !ValidID(line.ID) || !planKinds[line.Kind] {
			return Plan{}, Invalid("Plan lines require UUID identities and a supported kind")
		}
		if _, ok := seen[line.ID]; ok {
			return Plan{}, Invalid("Plan line identities must be unique")
		}
		if line.ExpectedMinor < 0 && line.EstimatedRemainingMinor == nil && !line.Complete {
			return Plan{}, Invalid("Signed plan lines require an explicit remaining estimate")
		}
		if line.Complete && line.EstimatedRemainingMinor != nil && *line.EstimatedRemainingMinor != 0 {
			return Plan{}, Invalid("A complete plan line cannot have a remaining estimate")
		}
		if line.AccountID != nil {
			a, ok := s.Accounts[*line.AccountID]
			if !ok {
				return Plan{}, NotFound()
			}
			if a.Currency != s.Settings.Currency {
				return Plan{}, Invalid("Plan account currency differs from settings")
			}
		}
		if line.RecurrenceID != nil {
			r, ok := s.Recurrences[*line.RecurrenceID]
			if !ok {
				return Plan{}, NotFound()
			}
			if recurrences[r.ID] {
				return Plan{}, Invalid("A recurrence can contribute only one plan line per month")
			}
			recurrences[r.ID] = true
			if month < r.StartMonth || (r.EndMonth != nil && month > *r.EndMonth) || r.Kind != line.Kind {
				return Plan{}, Invalid("Recurrence is not effective for this plan line")
			}
		}
		// Identities cannot be reused by another reporting month: entry matching is unambiguous.
		for otherMonth, p := range s.Plans {
			if otherMonth != month {
				for _, other := range p.Lines {
					if other.ID == line.ID {
						return Plan{}, Invalid("Plan line identity belongs to another month")
					}
				}
			}
		}
		seen[line.ID] = line
	}
	for _, prior := range old.Lines {
		linked := false
		for _, e := range s.Entries {
			if e.Active && e.ReportingMonth == month && ((e.PlanLineID != nil && *e.PlanLineID == prior.ID) || (e.FundingPlanLineID != nil && *e.FundingPlanLineID == prior.ID)) {
				linked = true
			}
		}
		for _, st := range s.Statements {
			if st.Active && st.ReportingMonth == month && st.PlanLineID != nil && *st.PlanLineID == prior.ID {
				linked = true
			}
		}
		if linked {
			next, ok := seen[prior.ID]
			if !ok || next.Kind != prior.Kind || !sameOptionalString(next.AccountID, prior.AccountID) {
				return Plan{}, Fail(409, "PLAN_LINE_IN_USE", "Matched plan lines cannot be removed or reclassified")
			}
		}
	}
	p := Plan{Version: old.Version + 1, Lines: append([]PlanLine{}, in.Lines...)}
	s.Plans[month] = p
	s.BumpMonth(month)
	s.AuditChange("plan.updated", month, "", old, p)
	return p, nil
}

// sameOptionalString treats two absent values as equal and otherwise compares strings.
func sameOptionalString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// CloseMonth records review assertions, the active card inventory, and a UTC close
// time, advancing the month version and revision and auditing the change. Unknown
// figures remain unknown. It rejects invalid months or assertions, missing cards,
// stale versions, and already closed months; expected is the month version.
func (s *State) CloseMonth(month string, in MonthTransition, expected int64) (Month, error) {
	m, err := s.GetMonth(month)
	if err != nil {
		return Month{}, err
	}
	if err = CheckVersion(m.Version, expected); err != nil {
		return Month{}, err
	}
	if m.State == "closed" {
		return Month{}, Fail(409, "MONTH_CLOSED", "Month is already closed")
	}
	c := in.Checklist
	if !c.IncomeReviewed || !c.CardsReviewed || !c.ExpensesReviewed || !c.AllocationsReviewed {
		return Month{}, Invalid("All monthly review checklist assertions are required")
	}
	if c.NoStatementReasons == nil {
		c.NoStatementReasons = map[string]string{}
	}
	for id, reason := range c.NoStatementReasons {
		a, ok := s.Accounts[id]
		if !ok {
			return Month{}, NotFound()
		}
		if a.Kind != "credit_card" || strings.TrimSpace(reason) == "" {
			return Month{}, Invalid("No-statement assertions require a card account and reason")
		}
		for _, st := range s.Statements {
			if st.Active && st.AccountID == id && st.ReportingMonth == month {
				return Month{}, Invalid("A no-statement assertion conflicts with a recorded statement")
			}
		}
	}
	// Closing is an explicit review assertion. Unknown figures are retained as unknown,
	// and still yield provisional summaries after closing; closure does not invent data.
	before := m
	now := time.Now().UTC()
	m.ReviewedCardIDs = []string{}
	for _, id := range sortedKeys(s.Accounts) {
		account := s.Accounts[id]
		if account.Kind == "credit_card" && account.Active {
			m.ReviewedCardIDs = append(m.ReviewedCardIDs, id)
		}
	}
	m.Checklist = c
	m.State = "closed"
	m.ClosedAt = &now
	m.Version++
	m.Revision++
	s.Months[month] = m
	s.AuditChange("month.closed", month, in.Reason, before, m)
	return m, nil
}

// ReopenMonth clears a closed month's close time, advances its version and revision,
// and audits the supplied reason. It rejects invalid months, stale month versions,
// already open months, and blank reasons; existing review assertions are retained.
func (s *State) ReopenMonth(month string, in MonthTransition, expected int64) (Month, error) {
	m, err := s.GetMonth(month)
	if err != nil {
		return Month{}, err
	}
	if err = CheckVersion(m.Version, expected); err != nil {
		return Month{}, err
	}
	if m.State != "closed" {
		return Month{}, Fail(409, "MONTH_OPEN", "Month is already open")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return Month{}, Invalid("Reopening requires a reason")
	}
	before := m
	m.State = "open"
	m.ClosedAt = nil
	m.Version++
	m.Revision++
	s.Months[month] = m
	s.AuditChange("month.reopened", month, in.Reason, before, m)
	return m, nil
}

// sortedKeys returns map keys in lexicographic order for deterministic traversal.
func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
