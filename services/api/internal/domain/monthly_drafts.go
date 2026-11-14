package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type MonthlyDraftInput struct {
	Month        string `json:"month"`
	SectionID    string `json:"section_id"`
	Name         string `json:"name"`
	AmountMinor  Money  `json:"amount_minor"`
	Note         string `json:"note"`
	Evidence     string `json:"evidence"`
	SourceID     string `json:"source_id"`
	ExternalID   string `json:"external_id"`
	SheetVersion int64  `json:"sheet_version"`
	// Explanations break a money-out proposal into named parts, such as the
	// charges behind a card statement total. Accepting copies them to the row.
	Explanations []MonthlyExplanation `json:"explanations"`
}
type MonthlyDraftEdit struct {
	SectionID    string               `json:"section_id"`
	Name         string               `json:"name"`
	AmountMinor  Money                `json:"amount_minor"`
	Note         string               `json:"note"`
	Explanations []MonthlyExplanation `json:"explanations,omitzero"`
}
type MonthlyDraft struct {
	Meta
	MonthlyDraftInput
	State          string  `json:"state"`
	AcceptedItemID *string `json:"accepted_item_id"`
	ProposalHash   string  `json:"proposal_hash"`
}
type MonthlyDraftAccept struct {
	SheetVersion int64 `json:"sheet_version"`
}

func normalizeExplanations(in []MonthlyExplanation) []MonthlyExplanation {
	out := make([]MonthlyExplanation, len(in))
	for i, e := range in {
		out[i] = MonthlyExplanation{Name: monthlyName(e.Name), AmountMinor: e.AmountMinor}
	}
	return out
}

func (s *State) validateMonthlyDraft(month, sectionID, name, note string, amount Money, explanations []MonthlyExplanation) error {
	if !ValidMonth(month) {
		return Invalid("Invalid monthly sheet month")
	}
	if s.Settings.Currency != "SGD" {
		return Fail(409, "CURRENCY_LOCKED", "Monthly proposals require SGD settings")
	}
	section, err := s.monthlySection(sectionID)
	if err != nil {
		return err
	}
	if err := monthlyText(name, note, &amount, nil, false); err != nil {
		return err
	}
	if len(explanations) > 0 && section.Direction != "out" {
		return Invalid("Only money out proposals can have explanations")
	}
	_, err = monthlyUnexplained(&amount, explanations)
	return err
}

// CreateMonthlyDraft stages a proposal without materializing rows or changing totals.
// Source identity is durable even after editing, rejecting, accepting, or Undo.
func (s *State) CreateMonthlyDraft(in MonthlyDraftInput) (MonthlyDraft, error) {
	in.Name = monthlyName(in.Name)
	in.SourceID, in.ExternalID = strings.TrimSpace(in.SourceID), strings.TrimSpace(in.ExternalID)
	in.Explanations = normalizeExplanations(in.Explanations)
	if in.SourceID == "" || len(in.SourceID) > 120 || in.ExternalID == "" || len(in.ExternalID) > 500 || len(in.Evidence) > 4000 || len(in.Explanations) > 100 {
		return MonthlyDraft{}, Invalid("Provide bounded source identity, evidence, and at most 100 explanations")
	}
	if err := s.validateMonthlyDraft(in.Month, in.SectionID, in.Name, in.Note, in.AmountMinor, in.Explanations); err != nil {
		return MonthlyDraft{}, err
	}
	identity := in
	identity.SheetVersion = 0
	data, _ := json.Marshal(identity)
	hash := sha256.Sum256(data)
	digest := hex.EncodeToString(hash[:])
	for _, d := range s.MonthlyDrafts {
		if d.SourceID == in.SourceID && d.ExternalID == in.ExternalID {
			if d.ProposalHash != digest {
				return MonthlyDraft{}, Fail(409, "SOURCE_CONFLICT", "This source identity already describes a different proposal")
			}
			return d, nil
		}
	}
	if err := CheckVersion(s.MonthlySheet.Version, in.SheetVersion); err != nil {
		return MonthlyDraft{}, err
	}
	d := MonthlyDraft{Meta: NewMeta(), MonthlyDraftInput: in, State: "pending", ProposalHash: digest}
	s.MonthlyDrafts[d.ID] = d
	s.AuditChange("monthly_draft.created", d.ID, "", nil, d)
	return d, nil
}

func (s *State) pendingMonthlyDraft(id string, expected int64) (MonthlyDraft, error) {
	d, ok := s.MonthlyDrafts[id]
	if !ok {
		return d, NotFound()
	}
	if err := CheckVersion(d.Version, expected); err != nil {
		return d, err
	}
	if d.State != "pending" {
		return d, Fail(409, "DRAFT_RESOLVED", "This proposal has already been reviewed")
	}
	return d, nil
}
func (s *State) EditMonthlyDraft(id string, in MonthlyDraftEdit, expected int64) (MonthlyDraft, error) {
	d, err := s.pendingMonthlyDraft(id, expected)
	if err != nil {
		return d, err
	}
	in.Name = monthlyName(in.Name)
	explanations := d.Explanations
	if in.Explanations != nil {
		explanations = normalizeExplanations(in.Explanations)
	}
	if len(explanations) > 100 {
		return d, Invalid("A proposal can have at most 100 explanations")
	}
	if err = s.validateMonthlyDraft(d.Month, in.SectionID, in.Name, in.Note, in.AmountMinor, explanations); err != nil {
		return d, err
	}
	before := d
	d.SectionID, d.Name, d.AmountMinor, d.Note, d.Explanations = in.SectionID, in.Name, in.AmountMinor, in.Note, explanations
	d.Touch()
	s.MonthlyDrafts[id] = d
	s.AuditChange("monthly_draft.edited", id, "", before, d)
	return d, nil
}
func (s *State) RejectMonthlyDraft(id string, expected int64) (MonthlyDraft, error) {
	d, err := s.pendingMonthlyDraft(id, expected)
	if err != nil {
		return d, err
	}
	before := d
	d.State = "rejected"
	d.Touch()
	s.MonthlyDrafts[id] = d
	s.AuditChange("monthly_draft.rejected", id, "", before, d)
	return d, nil
}

// AcceptMonthlyDraft runs inside the same owner transaction as its retry response.
// Existing blank cells are filled while preserving recurrence; entered cells require
// an explicit edit through the sheet. Accepted sources stay consumed after Undo.
func (s *State) AcceptMonthlyDraft(id string, expected, sheetVersion int64) (MonthlySheet, error) {
	d, ok := s.MonthlyDrafts[id]
	if !ok {
		return MonthlySheet{}, NotFound()
	}
	if d.State == "accepted" {
		return s.GetMonthlySheet(d.Month)
	}
	if _, err := s.pendingMonthlyDraft(id, expected); err != nil {
		return MonthlySheet{}, err
	}
	if err := s.validateMonthlyDraft(d.Month, d.SectionID, d.Name, d.Note, d.AmountMinor, d.Explanations); err != nil {
		return MonthlySheet{}, err
	}
	if err := CheckVersion(s.MonthlySheet.Version, sheetVersion); err != nil {
		return MonthlySheet{}, err
	}
	section, _ := s.monthlySection(d.SectionID)
	item, exists := s.findMonthlyItem(d.Name, section.Direction, "")
	var sheet MonthlySheet
	var err error
	if row, active := s.MonthlySheet.Rows[d.Month][item.ID]; exists && active && !row.Removed {
		if row.AmountMinor != nil {
			return MonthlySheet{}, Fail(409, "ITEM_ALREADY_ENTERED", "This item already has an amount. Review it on the monthly sheet before accepting another proposal")
		}
		p := item.preference(d.Month)
		sheet, err = s.UpdateMonthlyItem(d.Month, item.ID, MonthlyItemUpdate{SectionID: d.SectionID, Name: item.Name, AmountMinor: &d.AmountMinor, Note: d.Note, RepeatMonthly: p.RepeatMonthly, UsualAmountMinor: p.UsualAmountMinor, Explanations: d.Explanations}, sheetVersion)
	} else {
		sheet, err = s.AddMonthlyItem(d.Month, MonthlyItemInput{SectionID: d.SectionID, Name: d.Name, AmountMinor: &d.AmountMinor, Note: d.Note, Explanations: d.Explanations}, sheetVersion)
	}
	if err != nil {
		return MonthlySheet{}, err
	}
	item, _ = s.findMonthlyItem(d.Name, section.Direction, "")
	before := d
	d.State = "accepted"
	d.AcceptedItemID = &item.ID
	d.Touch()
	s.MonthlyDrafts[id] = d
	s.AuditChange("monthly_draft.accepted", id, "", before, d)
	return sheet, nil
}
