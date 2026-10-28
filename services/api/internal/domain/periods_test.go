package domain

import "testing"

func TestReconciliationPeriodDoesNotPostCash(t *testing.T) {
	s := summaryTestState(t)
	p, e := s.CreatePeriod(PeriodInput{StartDate: "2026-10-01", EndDate: "2026-10-31", ReportingMonth: "2026-10", AdditionsMinor: 10000, DeductionsMinor: 15000, Evidence: "synthetic worksheet"})
	if e != nil {
		t.Fatal(e)
	}
	if p.RemainderMinor != -5000 || p.ProposedTopUpMinor != 5000 || len(s.Entries) != 0 {
		t.Fatal(p)
	}
	summary, e := s.Summarize("2026-10")
	if e != nil {
		t.Fatal(e)
	}
	if summary.FundingRemainderMinor != 0 || summary.KnownSpendingMinor != 0 {
		t.Fatal(summary)
	}
}
