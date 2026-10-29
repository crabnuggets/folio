package domain

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

func monthlyName(name string) string    { return strings.Join(strings.Fields(name), " ") }
func monthlyNameKey(name string) string { return strings.ToLower(monthlyName(name)) }

func monthlyText(name, note string, amount, usual *Money, repeat bool) error {
	if name == "" || len(name) > 120 {
		return Invalid("Item name must contain 1 to 120 bytes")
	}
	if len(note) > 2000 {
		return Invalid("Note must contain at most 2000 bytes")
	}
	if amount != nil && *amount < 0 || usual != nil && *usual < 0 {
		return Invalid("Monthly amounts must be nonnegative")
	}
	if !repeat && usual != nil {
		return Invalid("A usual amount requires Repeat monthly")
	}
	return nil
}

func (s *State) monthlySection(id string) (MonthlySectionDefinition, error) {
	for _, section := range s.MonthlySheet.Sections {
		if section.ID == id {
			return section, nil
		}
	}
	return MonthlySectionDefinition{}, NotFound()
}

func (item MonthlyItemIdentity) preference(month string) MonthlyPreference {
	var result MonthlyPreference
	for _, p := range item.Preferences {
		if p.Month <= month && p.Month > result.Month {
			result = p
		}
	}
	return result
}

func sameMonthlyMoney(a, b *Money) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// setMonthlyPreference retains effective periods so past months and future edits
// keep their meaning when a preference is changed in a different month.
func (s *State) setMonthlyPreference(item *MonthlyItemIdentity, month, section string, repeat bool, usual *Money) {
	previous := item.preference(month)
	if previous.SectionID == section && previous.RepeatMonthly == repeat && sameMonthlyMoney(previous.UsualAmountMinor, usual) {
		return
	}
	p := MonthlyPreference{month, section, repeat, usual}
	found := false
	for i := range item.Preferences {
		if item.Preferences[i].Month == month {
			item.Preferences[i] = p
			found = true
			break
		}
	}
	if !found {
		item.Preferences = append(item.Preferences, p)
	}
	sort.Slice(item.Preferences, func(i, j int) bool { return item.Preferences[i].Month < item.Preferences[j].Month })
	// Only untouched generated rows can follow changed future preferences.
	for m, rows := range s.MonthlySheet.Rows {
		row, ok := rows[item.ID]
		if m <= month || !ok || row.Removed || !row.Generated || row.Touched || row.AmountMinor != nil || row.Note != "" {
			continue
		}
		future := item.preference(m)
		if !future.RepeatMonthly {
			delete(rows, item.ID)
		} else {
			row.SectionID = future.SectionID
			rows[item.ID] = row
		}
	}
}

func (s *State) monthlyBefore(month string, expected int64) (MonthlySheetData, error) {
	if !ValidMonth(month) {
		return MonthlySheetData{}, Invalid("Invalid monthly sheet month")
	}
	if err := CheckVersion(s.MonthlySheet.Version, expected); err != nil {
		return MonthlySheetData{}, err
	}
	// A detached before-image supports Undo and immutable audit history.
	b, err := json.Marshal(s.MonthlySheet.MonthlySheetData)
	if err != nil {
		return MonthlySheetData{}, err
	}
	var before MonthlySheetData
	err = json.Unmarshal(b, &before)
	return before, err
}

func (s *State) monthlyFinish(month string, before MonthlySheetData, undo bool) (MonthlySheet, error) {
	old, err := json.Marshal(before)
	if err != nil {
		return MonthlySheet{}, err
	}
	current, err := json.Marshal(s.MonthlySheet.MonthlySheetData)
	if err != nil {
		return MonthlySheet{}, err
	}
	changed := !bytes.Equal(old, current)
	if changed {
		// Check every affected month before the owner transaction can commit.
		for m := range s.MonthlySheet.Rows {
			if _, err := s.GetMonthlySheet(m); err != nil {
				return MonthlySheet{}, err
			}
		}
		s.MonthlySheet.Version++
		s.MonthlySheet.Undo = nil
		if undo {
			s.MonthlySheet.Undo = &MonthlyStoredUndo{NewID(), s.MonthlySheet.Version, before}
		}
		s.AuditChange("monthly_sheet.changed", month, "", before, s.MonthlySheet.MonthlySheetData)
	}
	result, err := s.GetMonthlySheet(month)
	if changed && undo && s.MonthlySheet.Undo != nil {
		token := s.MonthlySheet.Undo.Token
		result.UndoToken = &token
	}
	return result, err
}

// GetMonthlySheet reads only this model's persisted cells. Legacy financial
// postings never contribute to its totals, even for a closed legacy month.
func (s *State) GetMonthlySheet(month string) (MonthlySheet, error) {
	if !ValidMonth(month) {
		return MonthlySheet{}, Invalid("Invalid monthly sheet month")
	}
	result := MonthlySheet{Month: month, Version: s.MonthlySheet.Version, Currency: s.Settings.Currency, CurrencyExponent: s.Settings.CurrencyExponent, Sections: []MonthlySection{}}
	result.CurrencyLocked = s.currencyLocked()
	result.HasLegacyData = len(s.Accounts)+len(s.Entries)+len(s.Statements)+len(s.Claims)+len(s.Snapshots)+len(s.Plans)+len(s.Recurrences)+len(s.Periods)+len(s.Drafts)+len(s.LegacySummaries) > 0
	var incoming, outgoing summarySum
	for _, section := range s.MonthlySheet.Sections {
		view := MonthlySection{MonthlySectionDefinition: section, Items: []MonthlyItem{}}
		var total summarySum
		for id, row := range s.MonthlySheet.Rows[month] {
			if row.Removed || row.SectionID != section.ID {
				continue
			}
			item := s.MonthlySheet.Items[id]
			p := item.preference(month)
			view.Items = append(view.Items, MonthlyItem{id, item.Name, row.SectionID, row.AmountMinor, row.Note, p.RepeatMonthly, p.UsualAmountMinor})
			if row.AmountMinor == nil {
				view.MissingCount++
			} else {
				total.add(*row.AmountMinor)
			}
		}
		sort.Slice(view.Items, func(i, j int) bool {
			a, b := monthlyNameKey(view.Items[i].Name), monthlyNameKey(view.Items[j].Name)
			if a == b {
				return view.Items[i].ID < view.Items[j].ID
			}
			return a < b
		})
		var err error
		view.TotalMinor, err = total.value()
		if err != nil {
			return MonthlySheet{}, err
		}
		if section.Direction == "in" {
			incoming.add(view.TotalMinor)
		} else {
			outgoing.add(view.TotalMinor)
		}
		result.MissingCount += view.MissingCount
		result.Sections = append(result.Sections, view)
	}
	var err error
	if result.MoneyInMinor, err = incoming.value(); err != nil {
		return MonthlySheet{}, err
	}
	if result.MoneyOutMinor, err = outgoing.value(); err != nil {
		return MonthlySheet{}, err
	}
	result.RemainderMinor, err = summarySubtract(result.MoneyInMinor, result.MoneyOutMinor)
	return result, err
}

// OpenMonthlySheet materializes only names, never amounts, and honors skips.
func (s *State) OpenMonthlySheet(month string) (MonthlySheet, error) {
	before, err := s.monthlyBefore(month, s.MonthlySheet.Version)
	if err != nil {
		return MonthlySheet{}, err
	}
	if s.MonthlySheet.Rows[month] == nil {
		s.MonthlySheet.Rows[month] = map[string]MonthlyStoredRow{}
	}
	for id, item := range s.MonthlySheet.Items {
		p := item.preference(month)
		if _, exists := s.MonthlySheet.Rows[month][id]; exists || !p.RepeatMonthly {
			continue
		}
		s.MonthlySheet.Rows[month][id] = MonthlyStoredRow{ItemID: id, SectionID: p.SectionID, Generated: true}
	}
	return s.monthlyFinish(month, before, false)
}

func (s *State) findMonthlyItem(name, direction, except string) (MonthlyItemIdentity, bool) {
	key := monthlyNameKey(name)
	for _, id := range sortedKeys(s.MonthlySheet.Items) {
		item := s.MonthlySheet.Items[id]
		if item.ID == except || item.Direction != direction {
			continue
		}
		if monthlyNameKey(item.Name) == key {
			return item, true
		}
		for _, alias := range item.Aliases {
			if monthlyNameKey(alias) == key {
				return item, true
			}
		}
	}
	return MonthlyItemIdentity{}, false
}

func (s *State) AddMonthlyItem(month string, in MonthlyItemInput, expected int64) (MonthlySheet, error) {
	before, err := s.monthlyBefore(month, expected)
	if err != nil {
		return MonthlySheet{}, err
	}
	section, err := s.monthlySection(in.SectionID)
	if err != nil {
		return MonthlySheet{}, err
	}
	name := monthlyName(in.Name)
	item, exists := s.findMonthlyItem(name, section.Direction, "")
	if in.ItemID != nil {
		var ok bool
		item, ok = s.MonthlySheet.Items[*in.ItemID]
		if !ok || item.Direction != section.Direction {
			return MonthlySheet{}, NotFound()
		}
		exists = true
	}
	if !exists {
		item = MonthlyItemIdentity{ID: NewID(), Name: name, Direction: section.Direction, Aliases: []string{}, Preferences: []MonthlyPreference{}}
	}
	p := item.preference(month)
	repeat, usual := p.RepeatMonthly, p.UsualAmountMinor
	if in.RepeatMonthly != nil {
		repeat, usual = *in.RepeatMonthly, in.UsualAmountMinor
	} else if in.UsualAmountMinor != nil {
		usual = in.UsualAmountMinor
	}
	if err := monthlyText(name, in.Note, in.AmountMinor, usual, repeat); err != nil {
		return MonthlySheet{}, err
	}
	if row, ok := s.MonthlySheet.Rows[month][item.ID]; ok && !row.Removed {
		// An active duplicate keeps its entered fields. Reusing a skipped identity
		// below applies the submitted fields; explicit Restore recovers old fields.
		return s.monthlyFinish(month, before, true)
	}
	if s.MonthlySheet.Rows[month] == nil {
		s.MonthlySheet.Rows[month] = map[string]MonthlyStoredRow{}
	}
	s.setMonthlyPreference(&item, month, section.ID, repeat, usual)
	s.MonthlySheet.Items[item.ID] = item
	s.MonthlySheet.Rows[month][item.ID] = MonthlyStoredRow{ItemID: item.ID, SectionID: section.ID, AmountMinor: in.AmountMinor, Note: in.Note, Touched: true}
	if in.AmountMinor != nil || usual != nil {
		s.MonthlySheet.CurrencyLocked = true
	}
	return s.monthlyFinish(month, before, true)
}

func (s *State) UpdateMonthlyItem(month, id string, in MonthlyItemUpdate, expected int64) (MonthlySheet, error) {
	before, err := s.monthlyBefore(month, expected)
	if err != nil {
		return MonthlySheet{}, err
	}
	row, ok := s.MonthlySheet.Rows[month][id]
	if !ok || row.Removed {
		return MonthlySheet{}, NotFound()
	}
	item := s.MonthlySheet.Items[id]
	section, err := s.monthlySection(in.SectionID)
	if err != nil {
		return MonthlySheet{}, err
	}
	if section.Direction != item.Direction {
		return MonthlySheet{}, Invalid("An item's direction cannot change")
	}
	name := monthlyName(in.Name)
	if err := monthlyText(name, in.Note, in.AmountMinor, in.UsualAmountMinor, in.RepeatMonthly); err != nil {
		return MonthlySheet{}, err
	}
	if _, collision := s.findMonthlyItem(name, item.Direction, id); collision {
		return MonthlySheet{}, Fail(409, "ITEM_NAME_EXISTS", "An item with this name already exists on this side of the sheet")
	}
	if name != item.Name {
		item.Aliases = append(item.Aliases, item.Name)
		item.Name = name
	}
	s.setMonthlyPreference(&item, month, section.ID, in.RepeatMonthly, in.UsualAmountMinor)
	s.MonthlySheet.Items[id] = item
	row.SectionID, row.AmountMinor, row.Note, row.Touched = section.ID, in.AmountMinor, in.Note, true
	s.MonthlySheet.Rows[month][id] = row
	if in.AmountMinor != nil || in.UsualAmountMinor != nil {
		s.MonthlySheet.CurrencyLocked = true
	}
	return s.monthlyFinish(month, before, true)
}

func (s *State) RemoveMonthlyItem(month, id string, removed bool, expected int64) (MonthlySheet, error) {
	before, err := s.monthlyBefore(month, expected)
	if err != nil {
		return MonthlySheet{}, err
	}
	row, ok := s.MonthlySheet.Rows[month][id]
	if !ok {
		return MonthlySheet{}, NotFound()
	}
	row.Removed, row.Touched = removed, true
	s.MonthlySheet.Rows[month][id] = row
	return s.monthlyFinish(month, before, true)
}

func (s *State) UseMonthlyUsualAmounts(month string, in MonthlyUsualInput, expected int64) (MonthlySheet, error) {
	before, err := s.monthlyBefore(month, expected)
	if err != nil {
		return MonthlySheet{}, err
	}
	if in.SectionID != nil {
		if _, err := s.monthlySection(*in.SectionID); err != nil {
			return MonthlySheet{}, err
		}
	}
	for id, row := range s.MonthlySheet.Rows[month] {
		if row.Removed || row.AmountMinor != nil || in.SectionID != nil && row.SectionID != *in.SectionID {
			continue
		}
		p := s.MonthlySheet.Items[id].preference(month)
		if p.RepeatMonthly && p.UsualAmountMinor != nil {
			row.AmountMinor, row.Touched = p.UsualAmountMinor, true
			s.MonthlySheet.Rows[month][id] = row
			s.MonthlySheet.CurrencyLocked = true
		}
	}
	return s.monthlyFinish(month, before, true)
}

func (s *State) UndoMonthlySheet(month string, in MonthlyUndoInput, expected int64) (MonthlySheet, error) {
	before, err := s.monthlyBefore(month, expected)
	if err != nil {
		return MonthlySheet{}, err
	}
	u := s.MonthlySheet.Undo
	if u == nil || u.Token != in.UndoToken || u.Version != expected {
		return MonthlySheet{}, Fail(412, "STALE_VERSION", "This change can no longer be undone safely")
	}
	s.MonthlySheet.MonthlySheetData = u.Before
	return s.monthlyFinish(month, before, false)
}

func (s *State) CreateMonthlySection(month string, in MonthlySectionInput, expected int64) (MonthlySheet, error) {
	before, err := s.monthlyBefore(month, expected)
	if err != nil {
		return MonthlySheet{}, err
	}
	name := monthlyName(in.Name)
	if name == "" || len(name) > 120 || in.Direction != "in" && in.Direction != "out" {
		return MonthlySheet{}, Invalid("A section needs a name and incoming or outgoing direction")
	}
	for _, section := range s.MonthlySheet.Sections {
		if section.Direction == in.Direction && monthlyNameKey(section.Name) == monthlyNameKey(name) {
			return MonthlySheet{}, Fail(409, "SECTION_NAME_EXISTS", "A section with this name already exists on this side")
		}
	}
	s.MonthlySheet.Sections = append(s.MonthlySheet.Sections, MonthlySectionDefinition{NewID(), name, in.Direction, "Amount"})
	return s.monthlyFinish(month, before, true)
}

func (s *State) RenameMonthlySection(month, id string, in MonthlySectionRename, expected int64) (MonthlySheet, error) {
	before, err := s.monthlyBefore(month, expected)
	if err != nil {
		return MonthlySheet{}, err
	}
	section, err := s.monthlySection(id)
	if err != nil {
		return MonthlySheet{}, err
	}
	name := monthlyName(in.Name)
	if name == "" || len(name) > 120 {
		return MonthlySheet{}, Invalid("Section name must contain 1 to 120 bytes")
	}
	for _, other := range s.MonthlySheet.Sections {
		if other.ID != id && other.Direction == section.Direction && monthlyNameKey(other.Name) == monthlyNameKey(name) {
			return MonthlySheet{}, Fail(409, "SECTION_NAME_EXISTS", "A section with this name already exists on this side")
		}
	}
	for i := range s.MonthlySheet.Sections {
		if s.MonthlySheet.Sections[i].ID == id {
			s.MonthlySheet.Sections[i].Name = name
		}
	}
	return s.monthlyFinish(month, before, true)
}
