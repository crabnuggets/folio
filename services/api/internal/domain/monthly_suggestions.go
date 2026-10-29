package domain

import (
	"sort"
	"strings"
)

// SuggestMonthlyItems looks up identities on the selected side. Historical
// amounts never become suggestions without an explicit usual preference.
func (s *State) SuggestMonthlyItems(month, sectionID, query string) (MonthlyItemSuggestions, error) {
	result := MonthlyItemSuggestions{Items: []MonthlyItemSuggestion{}}
	if !ValidMonth(month) {
		return result, Invalid("Invalid monthly sheet month")
	}
	section, err := s.monthlySection(sectionID)
	if err != nil {
		return result, err
	}
	query = monthlyNameKey(query)
	type candidate struct {
		view           MonthlyItemSuggestion
		currentSection bool
		frequency      int
		recent         string
	}
	candidates := []candidate{}
	for _, item := range s.MonthlySheet.Items {
		if item.Direction != section.Direction {
			continue
		}
		matches := strings.Contains(monthlyNameKey(item.Name), query)
		for _, alias := range item.Aliases {
			matches = matches || strings.Contains(monthlyNameKey(alias), query)
		}
		if !matches {
			continue
		}
		p := item.preference(month)
		c := candidate{view: MonthlyItemSuggestion{ID: item.ID, Name: item.Name, Direction: item.Direction, RepeatMonthly: p.RepeatMonthly, UsualAmountMinor: p.UsualAmountMinor}}
		latestSection := p.SectionID
		for m, rows := range s.MonthlySheet.Rows {
			row, ok := rows[item.ID]
			if !ok || row.Removed {
				continue
			}
			c.frequency++
			if m > c.recent {
				c.recent, latestSection = m, row.SectionID
			}
			if row.SectionID == section.ID {
				c.currentSection = true
			}
			if m == month {
				id := item.ID
				c.view.CurrentMonthItemID = &id
			}
		}
		if c.currentSection {
			latestSection = section.ID
		}
		if latestSection == "" && len(item.Preferences) > 0 {
			latestSection = item.Preferences[len(item.Preferences)-1].SectionID
		}
		historicalSection, err := s.monthlySection(latestSection)
		if err != nil {
			return result, err
		}
		c.view.SectionID, c.view.SectionName = historicalSection.ID, historicalSection.Name
		candidates = append(candidates, c)
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.currentSection != b.currentSection {
			return a.currentSection
		}
		if a.frequency != b.frequency {
			return a.frequency > b.frequency
		}
		if a.recent != b.recent {
			return a.recent > b.recent
		}
		if monthlyNameKey(a.view.Name) != monthlyNameKey(b.view.Name) {
			return monthlyNameKey(a.view.Name) < monthlyNameKey(b.view.Name)
		}
		return a.view.ID < b.view.ID
	})
	seen := map[string]bool{}
	for _, c := range candidates {
		key := monthlyNameKey(c.view.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		result.Items = append(result.Items, c.view)
		if len(result.Items) == 10 {
			break
		}
	}
	return result, nil
}
