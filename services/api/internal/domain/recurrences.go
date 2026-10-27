package domain

import "time"

// validateRecurrence checks the kind, nonnegative minor-unit estimate, and inclusive
// month interval, propagating posting-account errors for an optional account.
func (s *State) validateRecurrence(in RecurrenceInput) error {
	if !planKinds[in.Kind] || !ValidMonth(in.StartMonth) {
		return Invalid("Recurrence requires a supported kind and start month")
	}
	if in.ExpectedMinor < 0 {
		return Invalid("Recurring expected amounts cannot be negative")
	}
	if in.EndMonth != nil && (!ValidMonth(*in.EndMonth) || *in.EndMonth < in.StartMonth) {
		return Invalid("Recurrence end month precedes its start")
	}
	if in.AccountID != nil {
		a, err := s.AccountForPosting(*in.AccountID)
		if err != nil {
			return err
		}
		if a.Currency != s.Settings.Currency {
			return Invalid("Recurrence account currency differs from settings")
		}
	}
	return nil
}
// CreateRecurrence stores and audits an expectation template without creating plan
// lines or postings. It propagates validation and account errors and returns
// MONTH_CLOSED if the inclusive interval overlaps a closed review.
func (s *State) CreateRecurrence(in RecurrenceInput) (Recurrence, error) {
	if err := s.validateRecurrence(in); err != nil {
		return Recurrence{}, err
	}
	for month, m := range s.Months {
		if m.State == "closed" && month >= in.StartMonth && (in.EndMonth == nil || month <= *in.EndMonth) {
			return Recurrence{}, Fail(409, "MONTH_CLOSED", "A recurrence cannot change a closed month's expectations")
		}
	}
	r := Recurrence{Meta: NewMeta(), RecurrenceInput: in}
	s.Recurrences[r.ID] = r
	s.Audit("recurrence.created", r.ID, "")
	return r, nil
}
// ReviseRecurrence returns a new template and ends its predecessor the month before
// the new start, advancing the predecessor's version and auditing the revision.
// It rejects missing or stale predecessors, invalid inputs or start months,
// superseded predecessors, and changes affecting closed months or existing future
// plan lines. expected is the predecessor's version.
func (s *State) ReviseRecurrence(id string, in RecurrenceInput, expected int64) (Recurrence, error) {
	old, ok := s.Recurrences[id]
	if !ok {
		return Recurrence{}, NotFound()
	}
	if err := CheckVersion(old.Version, expected); err != nil {
		return Recurrence{}, err
	}
	if err := s.validateRecurrence(in); err != nil {
		return Recurrence{}, err
	}
	if in.StartMonth <= old.StartMonth {
		return Recurrence{}, Invalid("A recurrence revision must start after its predecessor")
	}
	for _, r := range s.Recurrences {
		if r.SupersedesID != nil && *r.SupersedesID == id {
			return Recurrence{}, Fail(409, "RECURRENCE_SUPERSEDED", "Revise the latest recurrence version")
		}
	}
	if old.EndMonth != nil && in.StartMonth > *old.EndMonth {
		return Recurrence{}, Invalid("Revision must start within the predecessor's effective interval")
	}
	for month, m := range s.Months {
		if m.State == "closed" && month >= in.StartMonth && (old.EndMonth == nil || month <= *old.EndMonth) {
			return Recurrence{}, Fail(409, "MONTH_CLOSED", "Recurrence revisions cannot restate closed months")
		}
	}
	for month, p := range s.Plans {
		if month >= in.StartMonth {
			for _, line := range p.Lines {
				if line.RecurrenceID != nil && *line.RecurrenceID == id {
					return Recurrence{}, Fail(409, "RECURRENCE_IN_USE", "Revise affected future plan lines explicitly before changing the recurrence")
				}
			}
		}
	}
	start, _ := time.Parse("2006-01", in.StartMonth)
	end := start.AddDate(0, -1, 0).Format("2006-01")
	old.EndMonth = &end
	old.Touch()
	r := Recurrence{Meta: NewMeta(), RecurrenceInput: in, SupersedesID: &id}
	s.Recurrences[id] = old
	s.Recurrences[r.ID] = r
	s.Audit("recurrence.revised", r.ID, "")
	return r, nil
}
