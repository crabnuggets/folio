package domain

import "strings"

func (s *State) CreatePeriod(in PeriodInput) (Period, error) {
	if !ValidDate(in.StartDate) || !ValidDate(in.EndDate) || in.EndDate < in.StartDate || !ValidMonth(in.ReportingMonth) {
		return Period{}, Invalid("Reconciliation period requires valid dates and reporting month")
	}
	if in.AdditionsMinor < 0 || in.DeductionsMinor < 0 || strings.TrimSpace(in.Evidence) == "" {
		return Period{}, Invalid("Period requires nonnegative observations and source evidence")
	}
	if err := s.EnsureOpen(in.ReportingMonth); err != nil {
		return Period{}, err
	}
	// The difference of two nonnegative int64 values remains representable.
	remainder := in.AdditionsMinor - in.DeductionsMinor
	topup := Money(0)
	if remainder < 0 {
		topup = -remainder
	}
	p := Period{Meta: NewMeta(), PeriodInput: in, RemainderMinor: remainder, ProposedTopUpMinor: topup}
	s.Periods[p.ID] = p
	s.Audit("reconciliation_period.created", p.ID, "")
	return p, nil
}
