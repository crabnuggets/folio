package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func aeAccount(t *testing.T, s *State, kind string, funding bool) Account {
	t.Helper()
	a, err := s.CreateAccount(AccountInput{Alias: kind, Kind: kind, Currency: s.Settings.Currency, Active: true, FundingBoundary: []Boundary{{EffectiveMonth: "2026-01", Included: funding}}})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func aePtr[T any](v T) *T { return &v }
func aeErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s", code)
	}
	e, ok := err.(*Error)
	if !ok || e.Code != code {
		t.Fatalf("got %v, want %s", err, code)
	}
}
func TestAccountSettingsCurrencyAndTimezone(t *testing.T) {
	for _, timezone := range []string{"Asia/Singapore", "UTC"} {
		s := NewState()
		got, err := s.UpdateSettings(SettingsInput{Currency: "SGD", Timezone: timezone}, 1)
		if err != nil || got.Currency != "SGD" || got.CurrencyExponent != 2 || got.Timezone != timezone || got.Version != 2 {
			t.Fatalf("%s: got %+v, %v", timezone, got, err)
		}
		_, err = s.UpdateSettings(SettingsInput{Currency: "SGD", Timezone: "UTC"}, 1)
		aeErrorCode(t, err, "STALE_VERSION")
	}
	s := NewState()
	for _, input := range []SettingsInput{{"USD", "UTC"}, {"JPY", "UTC"}, {"KWD", "UTC"}, {"ZZZ", "UTC"}, {"SGD", "Local"}, {"SGD", "Mars/City"}, {"SGD", ""}} {
		_, err := s.UpdateSettings(input, 1)
		aeErrorCode(t, err, "VALIDATION_FAILED")
		if s.Settings != NewState().Settings || len(s.AuditEvents) != 0 {
			t.Fatal("invalid settings command changed settings or audit history")
		}
	}
	aeAccount(t, s, "bank", true)
	_, err := s.UpdateSettings(SettingsInput{Currency: "JPY", Timezone: "UTC"}, 1)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	if s.Settings.Currency != "SGD" || s.Settings.Version != 1 {
		t.Fatal("failed settings command mutated state")
	}
}

func TestAccountOwnershipArchivalAndBoundaryHistory(t *testing.T) {
	s := NewState()
	bank := aeAccount(t, s, "bank", true)
	cash := aeAccount(t, s, "cash", false)
	_, err := s.AccountForPosting(NewID())
	aeErrorCode(t, err, "NOT_FOUND")
	entry, err := s.CreateEntry(EntryInput{Kind: "income", AmountMinor: 500000, Currency: "SGD", EffectiveDate: "2026-10-25", DestinationAccountID: &bank.ID})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ReportingMonth != "2026-10" {
		t.Fatal("month not defaulted from local date")
	}
	update := bank.AccountInput
	update.FundingBoundary = []Boundary{{EffectiveMonth: "2026-01", Included: false}}
	_, err = s.UpdateAccount(bank.ID, update, 1)
	aeErrorCode(t, err, "BOUNDARY_RESTATEMENT_REQUIRED")
	update.FundingBoundary = []Boundary{{EffectiveMonth: "2027-01", Included: false}, {EffectiveMonth: "2026-01", Included: true}}
	updated, err := s.UpdateAccount(bank.ID, update, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !s.FundingIncluded(bank.ID, "2026-10") || s.FundingIncluded(bank.ID, "2027-01") {
		t.Fatal("effective boundary did not preserve history")
	}
	if updated.FundingBoundary[0].EffectiveMonth != "2026-01" {
		t.Fatal("boundary not sorted")
	}
	update = updated.AccountInput
	update.Kind = "credit_card"
	update.FundingBoundary = []Boundary{}
	_, err = s.UpdateAccount(bank.ID, update, updated.Version)
	aeErrorCode(t, err, "ACCOUNT_REFERENCED")
	update = updated.AccountInput
	update.Active = false
	archived, err := s.UpdateAccount(bank.ID, update, updated.Version)
	if err != nil || archived.Active {
		t.Fatal("archive failed", err)
	}
	_, err = s.CreateEntry(EntryInput{Kind: "direct_expense", AmountMinor: 1, Currency: "SGD", EffectiveDate: "2026-11-01", SourceAccountID: &bank.ID})
	aeErrorCode(t, err, "ACCOUNT_ARCHIVED")
	_, err = s.CreateAccount(AccountInput{Alias: "Foreign", Kind: "bank", Currency: "USD", Active: true})
	aeErrorCode(t, err, "VALIDATION_FAILED")
	cashInput := cash.AccountInput
	cashInput.FundingBoundary = []Boundary{{"2026-01", true}, {"2026-01", false}}
	_, err = s.UpdateAccount(cash.ID, cashInput, 1)
	aeErrorCode(t, err, "VALIDATION_FAILED")
}

func TestSnapshotObservationsRetainTimestampsAndMonthProtection(t *testing.T) {
	s := NewState()
	bank := aeAccount(t, s, "bank", true)
	at := time.Date(2026, 9, 30, 16, 1, 0, 0, time.UTC)
	snapshot, err := s.CreateSnapshot(SnapshotInput{AccountID: bank.ID, AmountMinor: -20, Currency: "SGD", ObservedAt: at, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.ObservedAt.Equal(at) || s.Months["2026-10"].Revision != 1 {
		t.Fatal("snapshot did not use Singapore month")
	}
	s.Months["2026-10"] = Month{Month: "2026-10", Version: 1, State: "closed"}
	_, err = s.CreateSnapshot(SnapshotInput{AccountID: bank.ID, AmountMinor: 20, Currency: "SGD", ObservedAt: at})
	aeErrorCode(t, err, "MONTH_CLOSED")
	if len(s.Snapshots) != 1 {
		t.Fatal("closed-month observation changed state")
	}
}

func TestSettingsAuditPreservesBeforeAndAfter(t *testing.T) {
	s := NewState()
	original := s.Settings
	updated, err := s.UpdateSettings(SettingsInput{Currency: "SGD", Timezone: "UTC"}, original.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.AuditEvents) != 1 {
		t.Fatal("settings update did not produce one audit event")
	}
	event := s.AuditEvents[0]
	if event.Action != "settings.updated" || event.EntityID != "settings" {
		t.Fatalf("unexpected audit metadata: %+v", event)
	}
	var before, after Settings
	if err := json.Unmarshal(event.Before, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(event.After, &after); err != nil {
		t.Fatal(err)
	}
	if before != original || after != updated {
		t.Fatalf("settings audit lost values: before %+v after %+v", before, after)
	}
	if _, err := s.UpdateSettings(SettingsInput{Currency: "SGD", Timezone: "Asia/Singapore"}, updated.Version); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(s.AuditEvents[0].After, &after); err != nil {
		t.Fatal(err)
	}
	if after != updated {
		t.Fatal("later settings update changed earlier audit history")
	}
	_, err = s.UpdateSettings(SettingsInput{Currency: "USD", Timezone: "UTC"}, original.Version)
	aeErrorCode(t, err, "STALE_VERSION")
	_, err = s.UpdateSettings(SettingsInput{Currency: "USD", Timezone: "UTC"}, s.Settings.Version)
	aeErrorCode(t, err, "VALIDATION_FAILED")
	if len(s.AuditEvents) != 2 {
		t.Fatal("failed settings command produced audit history")
	}
}

func TestAccountAuditPreservesBeforeAndAfter(t *testing.T) {
	s := NewState()
	original := aeAccount(t, s, "bank", true)
	input := original.AccountInput
	input.Alias = "Primary funding"
	input.Active = false
	input.FundingBoundary = append(input.FundingBoundary, Boundary{EffectiveMonth: "2027-01", Included: false})
	updated, err := s.UpdateAccount(original.ID, input, original.Version)
	if err != nil {
		t.Fatal(err)
	}
	event := s.AuditEvents[len(s.AuditEvents)-1]
	if event.Action != "account.updated" || event.EntityID != original.ID {
		t.Fatalf("unexpected audit metadata: %+v", event)
	}
	var before, after Account
	if err := json.Unmarshal(event.Before, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(event.After, &after); err != nil {
		t.Fatal(err)
	}
	if before.Version != 1 || before.Alias != "bank" || !before.Active || len(before.FundingBoundary) != 1 || !before.FundingBoundary[0].Included {
		t.Fatalf("original account was not retained: %+v", before)
	}
	if after.Version != 2 || after.Alias != "Primary funding" || after.Active || len(after.FundingBoundary) != 2 || after.FundingBoundary[1].Included {
		t.Fatalf("updated account was not retained: %+v", after)
	}
	input = updated.AccountInput
	input.Alias = "Renamed again"
	if _, err := s.UpdateAccount(updated.ID, input, updated.Version); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(s.AuditEvents[1].After, &after); err != nil {
		t.Fatal(err)
	}
	if after.Alias != "Primary funding" || after.Version != 2 {
		t.Fatal("later account update changed earlier audit history")
	}
}
