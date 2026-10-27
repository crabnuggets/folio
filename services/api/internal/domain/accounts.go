package domain

import (
	"slices"
	"strings"
	"time"
)

// CurrencyExponent lists the currencies supported by this initial single-currency
// service. Adding one requires an explicit exponent, never an assumed scale.
func CurrencyExponent(code string) (int, bool) {
	exponents := map[string]int{"SGD": 2, "USD": 2, "EUR": 2, "GBP": 2, "AUD": 2, "CAD": 2, "CHF": 2, "HKD": 2, "MYR": 2, "JPY": 0, "KRW": 0, "KWD": 3, "BHD": 3, "OMR": 3}
	exponent, ok := exponents[code]
	return exponent, ok
}

func (s *State) ValidateCurrency(code string) error {
	if _, ok := CurrencyExponent(code); !ok || code != s.Settings.Currency {
		return Invalid("Currency must equal the configured supported currency")
	}
	return nil
}

func (s *State) UpdateSettings(input SettingsInput, expected int64) (Settings, error) {
	if err := CheckVersion(s.Settings.Version, expected); err != nil {
		return Settings{}, err
	}
	exponent, ok := CurrencyExponent(input.Currency)
	if !ok {
		return Settings{}, Invalid("Unsupported currency")
	}
	if input.Timezone == "" || input.Timezone == "Local" {
		return Settings{}, Invalid("Timezone must be an IANA timezone")
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return Settings{}, Invalid("Timezone must be an IANA timezone")
	}
	if input.Currency != s.Settings.Currency && (len(s.Accounts) > 0 || len(s.Entries) > 0 || len(s.Statements) > 0 || len(s.Claims) > 0 || len(s.Snapshots) > 0 || len(s.Plans) > 0 || len(s.Recurrences) > 0 || len(s.Periods) > 0 || len(s.LegacySummaries) > 0) {
		return Settings{}, Fail(409, "CURRENCY_LOCKED", "Currency cannot change after financial setup; an explicit migration is required")
	}
	before := s.Settings
	s.Settings = Settings{Version: s.Settings.Version + 1, Currency: input.Currency, CurrencyExponent: exponent, Timezone: input.Timezone}
	s.AuditChange("settings.updated", "settings", "", before, s.Settings)
	return s.Settings, nil
}

func normalizeAccount(input AccountInput) (AccountInput, error) {
	input.Alias = strings.TrimSpace(input.Alias)
	if input.Alias == "" || len(input.Alias) > 120 {
		return AccountInput{}, Invalid("Account alias must contain 1 to 120 bytes")
	}
	switch input.Kind {
	case "bank", "cash", "credit_card", "investment", "savings":
	default:
		return AccountInput{}, Invalid("Unsupported account kind")
	}
	if input.Kind == "credit_card" && input.IncludeInLiquidCash {
		return AccountInput{}, Invalid("Credit card obligations are not liquid cash")
	}
	input.FundingBoundary = slices.Clone(input.FundingBoundary)
	if input.FundingBoundary == nil {
		input.FundingBoundary = []Boundary{}
	}
	slices.SortFunc(input.FundingBoundary, func(a, b Boundary) int { return strings.Compare(a.EffectiveMonth, b.EffectiveMonth) })
	for i, b := range input.FundingBoundary {
		if !ValidMonth(b.EffectiveMonth) {
			return AccountInput{}, Invalid("Invalid funding-boundary effective month")
		}
		if i > 0 && input.FundingBoundary[i-1].EffectiveMonth == b.EffectiveMonth {
			return AccountInput{}, Invalid("Funding-boundary effective months must be unique")
		}
		if input.Kind == "credit_card" && b.Included {
			return AccountInput{}, Invalid("A credit card cannot be a funding account")
		}
	}
	return input, nil
}

func (s *State) CreateAccount(input AccountInput) (Account, error) {
	if err := s.ValidateCurrency(input.Currency); err != nil {
		return Account{}, err
	}
	input, err := normalizeAccount(input)
	if err != nil {
		return Account{}, err
	}
	account := Account{Meta: NewMeta(), AccountInput: input}
	s.Accounts[account.ID] = account
	s.Audit("account.created", account.ID, "")
	return account, nil
}

func boundaryIncluded(boundaries []Boundary, month string) bool {
	included := false
	// Work with unsorted historical data as well as normalized commands.
	latest := ""
	for _, b := range boundaries {
		if b.EffectiveMonth <= month && b.EffectiveMonth >= latest {
			included = b.Included
			latest = b.EffectiveMonth
		}
	}
	return included
}

func (s *State) FundingIncluded(accountID, month string) bool {
	account, ok := s.Accounts[accountID]
	return ok && boundaryIncluded(account.FundingBoundary, month)
}

func (s *State) UpdateAccount(id string, input AccountInput, expected int64) (Account, error) {
	old, ok := s.Accounts[id]
	if !ok {
		return Account{}, NotFound()
	}
	if err := CheckVersion(old.Version, expected); err != nil {
		return Account{}, err
	}
	if err := s.ValidateCurrency(input.Currency); err != nil {
		return Account{}, err
	}
	input, err := normalizeAccount(input)
	if err != nil {
		return Account{}, err
	}
	// Account kind is part of the meaning of referenced historical postings.
	if old.Kind != input.Kind && s.accountReferenced(id) {
		return Account{}, Fail(409, "ACCOUNT_REFERENCED", "Referenced account kind cannot change")
	}
	for _, entry := range s.Entries {
		if boundaryIncluded(old.FundingBoundary, entry.ReportingMonth) != boundaryIncluded(input.FundingBoundary, entry.ReportingMonth) {
			return Account{}, Fail(409, "BOUNDARY_RESTATEMENT_REQUIRED", "Funding-boundary changes cannot restate recorded history")
		}
	}
	for _, statement := range s.Statements {
		if boundaryIncluded(old.FundingBoundary, statement.ReportingMonth) != boundaryIncluded(input.FundingBoundary, statement.ReportingMonth) {
			return Account{}, Fail(409, "BOUNDARY_RESTATEMENT_REQUIRED", "Funding-boundary changes cannot restate recorded statement history")
		}
	}
	for month, m := range s.Months {
		if m.State == "closed" && boundaryIncluded(old.FundingBoundary, month) != boundaryIncluded(input.FundingBoundary, month) {
			return Account{}, Fail(409, "BOUNDARY_RESTATEMENT_REQUIRED", "Funding-boundary changes cannot restate closed months")
		}
	}

	before := old
	old.AccountInput = input
	old.Touch()
	s.Accounts[id] = old
	s.AuditChange("account.updated", id, "", before, old)
	return old, nil
}

func (s *State) accountReferenced(id string) bool {
	for _, e := range s.Entries {
		if (e.SourceAccountID != nil && *e.SourceAccountID == id) || (e.DestinationAccountID != nil && *e.DestinationAccountID == id) {
			return true
		}
	}
	for _, st := range s.Statements {
		if st.AccountID == id {
			return true
		}
	}
	for _, snap := range s.Snapshots {
		if snap.AccountID == id {
			return true
		}
	}
	for _, plan := range s.Plans {
		for _, line := range plan.Lines {
			if line.AccountID != nil && *line.AccountID == id {
				return true
			}
		}
	}
	for _, r := range s.Recurrences {
		if r.AccountID != nil && *r.AccountID == id {
			return true
		}
	}
	return false
}

func (s *State) AccountForPosting(id string) (Account, error) {
	if !ValidID(id) {
		return Account{}, Invalid("Account ID must be a UUID")
	}
	account, ok := s.Accounts[id]
	if !ok {
		return Account{}, NotFound()
	}
	if !account.Active {
		return Account{}, Fail(409, "ACCOUNT_ARCHIVED", "Account is archived")
	}
	if err := s.ValidateCurrency(account.Currency); err != nil {
		return Account{}, err
	}
	return account, nil
}

func (s *State) NormalizeReportingMonth(date, month, reason string) (string, error) {
	if !ValidDate(date) {
		return "", Invalid("Invalid effective date")
	}
	if month == "" {
		month = date[:7]
	}
	if !ValidMonth(month) {
		return "", Invalid("Invalid reporting month")
	}
	if month != date[:7] && strings.TrimSpace(reason) == "" {
		return "", Invalid("Reporting-month overrides require a reason")
	}
	if err := s.EnsureOpen(month); err != nil {
		return "", err
	}
	return month, nil
}

func (s *State) CreateSnapshot(input SnapshotInput) (Snapshot, error) {
	if err := s.ValidateCurrency(input.Currency); err != nil {
		return Snapshot{}, err
	}
	if _, err := s.AccountForPosting(input.AccountID); err != nil {
		return Snapshot{}, err
	}
	if input.ObservedAt.IsZero() || input.ObservedAt.Year() < 1900 || input.ObservedAt.Year() > 9999 {
		return Snapshot{}, Invalid("A valid observation timestamp is required")
	}
	if input.ObservedAt.After(time.Now().Add(5 * time.Minute)) {
		return Snapshot{}, Invalid("Observation timestamp cannot be in the future")
	}
	location, err := time.LoadLocation(s.Settings.Timezone)
	if err != nil {
		return Snapshot{}, Invalid("Configured timezone is invalid")
	}
	month := input.ObservedAt.In(location).Format("2006-01")
	if err := s.EnsureOpen(month); err != nil {
		return Snapshot{}, err
	}
	input.ObservedAt = input.ObservedAt.UTC()
	snapshot := Snapshot{Meta: NewMeta(), SnapshotInput: input}
	s.Snapshots[snapshot.ID] = snapshot
	s.BumpMonth(month)
	s.Audit("snapshot.created", snapshot.ID, "")
	return snapshot, nil
}
