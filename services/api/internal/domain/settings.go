package domain

import (
	"time"
)

func (s *State) currencyLocked() bool {
	return s.MonthlySheet.CurrencyLocked || s.Settings.Currency != "SGD"
}

// UpdateSettings replaces and audits settings when expected matches their version.
// It rejects unsupported currencies, invalid timezones, and currency changes after
// financial setup; version mismatches return STALE_VERSION.
func (s *State) UpdateSettings(input SettingsInput, expected int64) (Settings, error) {
	if err := CheckVersion(s.Settings.Version, expected); err != nil {
		return Settings{}, err
	}
	if input.Currency != "SGD" {
		return Settings{}, Invalid("Currency is fixed to SGD")
	}
	if input.Timezone == "" || input.Timezone == "Local" {
		return Settings{}, Invalid("Timezone must be an IANA timezone")
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return Settings{}, Invalid("Timezone must be an IANA timezone")
	}
	if input.Currency != s.Settings.Currency && s.currencyLocked() {
		return Settings{}, Fail(409, "CURRENCY_LOCKED", "Currency cannot change after financial setup; an explicit migration is required")
	}
	before := s.Settings
	s.Settings = Settings{Version: s.Settings.Version + 1, Currency: input.Currency, CurrencyExponent: 2, Timezone: input.Timezone}
	s.AuditChange("settings.updated", "settings", "", before, s.Settings)
	return s.Settings, nil
}
