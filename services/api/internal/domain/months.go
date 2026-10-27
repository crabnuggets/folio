package domain

import (
	"sort"
	"strings"
	"time"
)

var planKinds = map[string]bool{"income": true, "spending": true, "allocation": true, "funding_inflow": true, "funding_outflow": true}

func (s *State) GetMonth(month string) (Month, error) {
	if !ValidMonth(month) {
		return Month{}, Invalid("Invalid reporting month")
	}
	if m, ok := s.Months[month]; ok {
		return m, nil
	}
	return Month{Month: month, Version: 1, State: "open", Checklist: Checklist{NoStatementReasons: map[string]string{}}}, nil
}
func (s *State) GetPlan(month string) (Plan, error) {
	if !ValidMonth(month) {
		return Plan{}, Invalid("Invalid reporting month")
	}
	if p, ok := s.Plans[month]; ok {
		return p, nil
	}
	return Plan{Version: 1, Lines: []PlanLine{}}, nil
}
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
func sameOptionalString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

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
	m.Checklist = c
	m.State = "closed"
	m.ClosedAt = &now
	m.Version++
	m.Revision++
	s.Months[month] = m
	s.AuditChange("month.closed", month, in.Reason, before, m)
	return m, nil
}
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

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
