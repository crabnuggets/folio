package domain

// ValidateSummaries is a transaction commit gate. A command must not make any
// persisted reporting month impossible to render within the public money range.
func (s *State) ValidateSummaries() error {
	months := map[string]bool{}
	for month := range s.Months {
		months[month] = true
	}
	for month := range s.Plans {
		months[month] = true
	}
	for _, entry := range s.Entries {
		months[entry.ReportingMonth] = true
	}
	for _, statement := range s.Statements {
		months[statement.ReportingMonth] = true
		months[statement.DueDate[:7]] = true
	}
	for _, match := range s.ReceiptMatches {
		if match.RecoveryMonth != nil {
			months[*match.RecoveryMonth] = true
		}
	}
	for _, month := range sortedKeys(months) {
		if _, err := s.Summarize(month); err != nil {
			return err
		}
	}
	return nil
}
