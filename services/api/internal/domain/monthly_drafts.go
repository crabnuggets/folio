package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type MonthlyDraftInput struct {
	Month        string `json:"month"`
	SectionID    string `json:"section_id"`
	Name         string `json:"name"`
	AmountMinor  *Money `json:"amount_minor"`
	Note         string `json:"note"`
	Evidence     string `json:"evidence"`
	SourceID     string `json:"source_id"`
	ExternalID   string `json:"external_id"`
	SheetVersion int64  `json:"sheet_version"`
	// Explanations break a money-out proposal into named parts, such as the
	// charges behind a card statement total. Accepting copies them to the row.
	Explanations []MonthlyExplanation `json:"explanations"`
	// RepeatMonthly makes the accepted item a monthly recurring item from Month,
	// with UsualAmountMinor as its suggestion. Only then may AmountMinor be blank.
	// A recurring item change uses both for the item's settings after the change.
	RepeatMonthly    bool   `json:"repeat_monthly"`
	UsualAmountMinor *Money `json:"usual_amount_minor"`
}
type MonthlyDraftEdit struct {
	SectionID    string               `json:"section_id"`
	Name         string               `json:"name"`
	AmountMinor  *Money               `json:"amount_minor"`
	Note         string               `json:"note"`
	Explanations []MonthlyExplanation `json:"explanations,omitzero"`
}
type MonthlyDraft struct {
	Meta
	MonthlyDraftInput
	// Amendment is set when the proposal changes an item that already has an
	// amount. Nil means the proposal fills a blank item or adds one.
	Amendment *MonthlyAmendment `json:"amendment"`
	// RecurrenceChange is set when the proposal changes an item's recurrence
	// settings from Month onward. SectionID, RepeatMonthly and UsualAmountMinor
	// then hold the settings after the change, and AmountMinor stays blank.
	RecurrenceChange *MonthlyRecurrenceChange `json:"recurrence_change"`
	State            string                   `json:"state"`
	AcceptedItemID   *string                  `json:"accepted_item_id"`
	ProposalHash     string                   `json:"proposal_hash"`
}

// MonthlyRecurrenceChange keeps the settings a recurring item change was proposed
// against, so acceptance is refused once the owner has changed them.
type MonthlyRecurrenceChange struct {
	ItemID               string `json:"item_id"`
	BaseSectionID        string `json:"base_section_id"`
	BaseRepeatMonthly    bool   `json:"base_repeat_monthly"`
	BaseUsualAmountMinor *Money `json:"base_usual_amount_minor"`
}

// MonthlyRecurrenceInput asks for an item's complete recurrence settings from Month on.
type MonthlyRecurrenceInput struct {
	Month            string `json:"month"`
	ItemID           string `json:"item_id"`
	SectionID        string `json:"section_id"`
	RepeatMonthly    bool   `json:"repeat_monthly"`
	UsualAmountMinor *Money `json:"usual_amount_minor"`
	Note             string `json:"note"`
	Evidence         string `json:"evidence"`
	SheetVersion     int64  `json:"sheet_version"`
}

// MonthlyAmendment keeps the entered values a proposal was made against, so
// acceptance is refused once the owner has changed the item.
type MonthlyAmendment struct {
	ItemID           string               `json:"item_id"`
	BaseAmountMinor  Money                `json:"base_amount_minor"`
	BaseExplanations []MonthlyExplanation `json:"base_explanations"`
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

func (s *State) validateMonthlyProposal(month string) error {
	if !ValidMonth(month) {
		return Invalid("Invalid monthly sheet month")
	}
	if s.Settings.Currency != "SGD" {
		return Fail(409, "CURRENCY_LOCKED", "Monthly proposals require SGD settings")
	}
	return nil
}

func (s *State) validateMonthlyDraft(d MonthlyDraftInput) error {
	if err := s.validateMonthlyProposal(d.Month); err != nil {
		return err
	}
	section, err := s.monthlySection(d.SectionID)
	if err != nil {
		return err
	}
	if err := monthlyText(d.Name, d.Note, d.AmountMinor, d.UsualAmountMinor, d.RepeatMonthly); err != nil {
		return err
	}
	if d.AmountMinor == nil && !d.RepeatMonthly {
		return Invalid("Enter an amount, or set repeat_monthly to add a blank monthly recurring item")
	}
	if len(d.Explanations) > 0 && section.Direction != "out" {
		return Invalid("Only money out proposals can have explanations")
	}
	_, err = monthlyUnexplained(d.AmountMinor, d.Explanations)
	return err
}

func sameExplanations(a, b []MonthlyExplanation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if monthlyNameKey(a[i].Name) != monthlyNameKey(b[i].Name) || a[i].AmountMinor != b[i].AmountMinor {
			return false
		}
	}
	return true
}

// proposalFingerprint covers what a proposal would change, not how the assistant
// worded its note or evidence, so a re-read of the same document matches.
func proposalFingerprint(in MonthlyDraftInput, itemID string) string {
	parts := []string{in.Month, in.SectionID, monthlyNameKey(in.Name), itemID, minorText(in.AmountMinor)}
	for _, e := range in.Explanations {
		parts = append(parts, monthlyNameKey(e.Name), strconv.FormatInt(int64(e.AmountMinor), 10))
	}
	// Proposals without recurrence keep the fingerprint they were stored with.
	var value any = parts
	if in.RepeatMonthly {
		value = []any{parts, "repeat_monthly", minorText(in.UsualAmountMinor)}
	}
	return fingerprint(value)
}

func recurrenceFingerprint(in MonthlyRecurrenceInput) string {
	return fingerprint([]any{"recurrence", in.Month, in.ItemID, in.SectionID, in.RepeatMonthly, minorText(in.UsualAmountMinor)})
}

func fingerprint(value any) string {
	data, _ := json.Marshal(value)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func minorText(amount *Money) string {
	if amount == nil {
		return ""
	}
	return strconv.FormatInt(int64(*amount), 10)
}

// minorValue describes an optional amount in error messages for the assistant.
func minorValue(amount *Money) any {
	if amount == nil {
		return "blank"
	}
	return int64(*amount)
}

// CreateMonthlyDraft stages a proposal to fill a blank item or add one, without
// materializing rows or changing totals. existing reports that the source was
// already proposed; the stored proposal is returned unchanged.
func (s *State) CreateMonthlyDraft(in MonthlyDraftInput) (d MonthlyDraft, existing bool, err error) {
	return s.stageMonthlyDraft(in, "")
}

// CreateMonthlyAmendment stages a proposal to change the amount or explanations of
// an item that already has an amount in the month. The explanations replace the
// item's breakdown. The item's name, section, note, and recurrence are kept.
func (s *State) CreateMonthlyAmendment(in MonthlyDraftInput, itemID string) (d MonthlyDraft, existing bool, err error) {
	return s.stageMonthlyDraft(in, itemID)
}

// Source identity is durable even after editing, rejecting, accepting, or Undo.
func (s *State) stageMonthlyDraft(in MonthlyDraftInput, itemID string) (MonthlyDraft, bool, error) {
	in.Name = monthlyName(in.Name)
	in.SourceID, in.ExternalID = strings.TrimSpace(in.SourceID), strings.TrimSpace(in.ExternalID)
	in.Explanations = normalizeExplanations(in.Explanations)
	if in.SourceID == "" || len(in.SourceID) > 120 || in.ExternalID == "" || len(in.ExternalID) > 500 || len(in.Evidence) > 4000 || len(in.Explanations) > 100 {
		return MonthlyDraft{}, false, Invalid("Provide bounded source identity, evidence, and at most 100 explanations")
	}
	if itemID != "" {
		// The item, not the assistant's wording, determines an amendment's name and section.
		in.SectionID, in.Name = "", ""
	}
	digest := proposalFingerprint(in, itemID)
	for _, d := range s.MonthlyDrafts {
		if d.SourceID == in.SourceID && d.ExternalID == in.ExternalID {
			if d.ProposalHash != digest {
				return MonthlyDraft{}, false, Fail(409, "SOURCE_CONFLICT", fmt.Sprintf("This source identity already belongs to a different %s proposal (id %s): %q for %s, amount_minor %v, with %d explanations. Tell the owner instead of proposing again. Propose under a new identity only if the owner asks for a separate proposal.", d.State, d.ID, d.Name, d.Month, minorValue(d.AmountMinor), len(d.Explanations)))
			}
			return d, true, nil
		}
	}
	var amendment *MonthlyAmendment
	if itemID != "" {
		if in.AmountMinor == nil || in.RepeatMonthly || in.UsualAmountMinor != nil {
			return MonthlyDraft{}, false, Invalid("An amendment needs the item's full amount and cannot change its recurrence")
		}
		if !ValidMonth(in.Month) {
			return MonthlyDraft{}, false, Invalid("Invalid monthly sheet month")
		}
		row, ok := s.MonthlySheet.Rows[in.Month][itemID]
		if !ok || row.Removed {
			return MonthlyDraft{}, false, NotFound()
		}
		if row.AmountMinor == nil {
			return MonthlyDraft{}, false, Invalid("Only an item with an amount can be amended. Propose a monthly amount for the blank item instead")
		}
		if *row.AmountMinor == *in.AmountMinor && sameExplanations(row.Explanations, in.Explanations) {
			return MonthlyDraft{}, false, Invalid("The amendment matches the entered amount and explanations")
		}
		in.SectionID, in.Name = row.SectionID, s.MonthlySheet.Items[itemID].Name
		amendment = &MonthlyAmendment{ItemID: itemID, BaseAmountMinor: *row.AmountMinor, BaseExplanations: append([]MonthlyExplanation{}, row.Explanations...)}
	}
	if err := s.validateMonthlyDraft(in); err != nil {
		return MonthlyDraft{}, false, err
	}
	if in.RepeatMonthly {
		section, _ := s.monthlySection(in.SectionID)
		if item, exists := s.findMonthlyItem(in.Name, section.Direction, ""); exists {
			return MonthlyDraft{}, false, Invalid(fmt.Sprintf("%q is already an item (id %s). Leave repeat_monthly unset to propose its amount, or use propose_recurring_item_change to change its recurrence", item.Name, item.ID))
		}
	}
	if err := CheckVersion(s.MonthlySheet.Version, in.SheetVersion); err != nil {
		return MonthlyDraft{}, false, err
	}
	d := MonthlyDraft{Meta: NewMeta(), MonthlyDraftInput: in, Amendment: amendment, State: "pending", ProposalHash: digest}
	s.MonthlyDrafts[d.ID] = d
	s.AuditChange("monthly_draft.created", d.ID, "", nil, d)
	return d, false, nil
}

// CreateMonthlyRecurrenceChange stages a proposal to change an item's recurrence
// settings from in.Month onward. It has no source identity: each item can have one
// pending change, and proposing the same change again returns it as existing.
func (s *State) CreateMonthlyRecurrenceChange(in MonthlyRecurrenceInput) (MonthlyDraft, bool, error) {
	if err := s.validateMonthlyProposal(in.Month); err != nil {
		return MonthlyDraft{}, false, err
	}
	if len(in.Evidence) > 4000 {
		return MonthlyDraft{}, false, Invalid("Evidence must contain at most 4000 bytes")
	}
	item, ok := s.MonthlySheet.Items[in.ItemID]
	if !ok {
		return MonthlyDraft{}, false, NotFound()
	}
	section, err := s.monthlySection(in.SectionID)
	if err != nil {
		return MonthlyDraft{}, false, err
	}
	if section.Direction != item.Direction {
		return MonthlyDraft{}, false, Invalid("An item can only move to a section on the same side of the sheet")
	}
	if err := monthlyText(item.Name, in.Note, nil, in.UsualAmountMinor, in.RepeatMonthly); err != nil {
		return MonthlyDraft{}, false, err
	}
	current := item.preference(in.Month)
	if current.Month == "" {
		return MonthlyDraft{}, false, Invalid(fmt.Sprintf("%q has no settings for %s yet. Choose a month on or after it first appeared", item.Name, in.Month))
	}
	digest := recurrenceFingerprint(in)
	for _, id := range sortedKeys(s.MonthlyDrafts) {
		d := s.MonthlyDrafts[id]
		if d.State != "pending" || d.RecurrenceChange == nil || d.RecurrenceChange.ItemID != in.ItemID {
			continue
		}
		if d.ProposalHash == digest {
			return d, true, nil
		}
		return MonthlyDraft{}, false, Fail(409, "PROPOSAL_PENDING", fmt.Sprintf("A different recurring item change for %q is pending (id %s): from %s, repeat_monthly %t, usual_amount_minor %v. Tell the owner to accept or reject it before proposing another.", d.Name, d.ID, d.Month, d.RepeatMonthly, minorValue(d.UsualAmountMinor)))
	}
	if current.SectionID == in.SectionID && current.RepeatMonthly == in.RepeatMonthly && sameMonthlyMoney(current.UsualAmountMinor, in.UsualAmountMinor) {
		return MonthlyDraft{}, false, Invalid("The proposal matches the item's current recurrence settings")
	}
	if err := CheckVersion(s.MonthlySheet.Version, in.SheetVersion); err != nil {
		return MonthlyDraft{}, false, err
	}
	d := MonthlyDraft{
		Meta:              NewMeta(),
		MonthlyDraftInput: MonthlyDraftInput{Month: in.Month, SectionID: in.SectionID, Name: item.Name, Note: in.Note, Evidence: in.Evidence, SheetVersion: in.SheetVersion, Explanations: []MonthlyExplanation{}, RepeatMonthly: in.RepeatMonthly, UsualAmountMinor: in.UsualAmountMinor},
		RecurrenceChange:  &MonthlyRecurrenceChange{ItemID: item.ID, BaseSectionID: current.SectionID, BaseRepeatMonthly: current.RepeatMonthly, BaseUsualAmountMinor: current.UsualAmountMinor},
		State:             "pending",
		ProposalHash:      digest,
	}
	s.MonthlyDrafts[d.ID] = d
	s.AuditChange("monthly_draft.created", d.ID, "", nil, d)
	return d, false, nil
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
	if d.RecurrenceChange != nil {
		return d, Invalid("A recurring item change cannot be edited. Reject it, or accept it and change the item on the sheet")
	}
	in.Name = monthlyName(in.Name)
	if d.Amendment != nil && (in.SectionID != d.SectionID || in.Name != d.Name) {
		return d, Invalid("An amendment's item name and section cannot change")
	}
	explanations := d.Explanations
	if in.Explanations != nil {
		explanations = normalizeExplanations(in.Explanations)
	}
	if len(explanations) > 100 {
		return d, Invalid("A proposal can have at most 100 explanations")
	}
	if d.Amendment != nil && in.AmountMinor == nil {
		return d, Invalid("An amendment needs the item's full amount")
	}
	edited := d.MonthlyDraftInput
	edited.SectionID, edited.Name, edited.AmountMinor, edited.Note, edited.Explanations = in.SectionID, in.Name, in.AmountMinor, in.Note, explanations
	if err = s.validateMonthlyDraft(edited); err != nil {
		return d, err
	}
	before := d
	d.MonthlyDraftInput = edited
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
	if d.RecurrenceChange != nil {
		return s.acceptMonthlyRecurrenceChange(d, sheetVersion)
	}
	if err := s.validateMonthlyDraft(d.MonthlyDraftInput); err != nil {
		return MonthlySheet{}, err
	}
	if err := CheckVersion(s.MonthlySheet.Version, sheetVersion); err != nil {
		return MonthlySheet{}, err
	}
	section, _ := s.monthlySection(d.SectionID)
	item, exists := s.findMonthlyItem(d.Name, section.Direction, "")
	var sheet MonthlySheet
	var err error
	if a := d.Amendment; a != nil {
		row, active := s.MonthlySheet.Rows[d.Month][a.ItemID]
		if !active || row.Removed || row.AmountMinor == nil || *row.AmountMinor != a.BaseAmountMinor || !sameExplanations(row.Explanations, a.BaseExplanations) {
			return MonthlySheet{}, Fail(409, "ITEM_CHANGED", "This item's amount or explanations changed after the amendment was proposed. Reject it, or ask for a new proposal against the current values")
		}
		item = s.MonthlySheet.Items[a.ItemID]
		p := item.preference(d.Month)
		sheet, err = s.UpdateMonthlyItem(d.Month, item.ID, MonthlyItemUpdate{SectionID: row.SectionID, Name: item.Name, AmountMinor: d.AmountMinor, Note: row.Note, RepeatMonthly: p.RepeatMonthly, UsualAmountMinor: p.UsualAmountMinor, Explanations: append([]MonthlyExplanation{}, d.Explanations...)}, sheetVersion)
	} else if exists && d.RepeatMonthly {
		// Recurrence on an existing item is a recurring item change, so its settings are never overwritten here.
		return MonthlySheet{}, Fail(409, "ITEM_EXISTS", fmt.Sprintf("An item named %q now exists. Reject this proposal, and ask for an amount or a recurring item change for that item instead", item.Name))
	} else if row, active := s.MonthlySheet.Rows[d.Month][item.ID]; exists && active && !row.Removed {
		if row.AmountMinor != nil {
			return MonthlySheet{}, Fail(409, "ITEM_ALREADY_ENTERED", "This item already has an amount. Review it on the monthly sheet before accepting another proposal")
		}
		p := item.preference(d.Month)
		sheet, err = s.UpdateMonthlyItem(d.Month, item.ID, MonthlyItemUpdate{SectionID: d.SectionID, Name: item.Name, AmountMinor: d.AmountMinor, Note: d.Note, RepeatMonthly: p.RepeatMonthly, UsualAmountMinor: p.UsualAmountMinor, Explanations: d.Explanations}, sheetVersion)
	} else {
		var repeat *bool
		if d.RepeatMonthly {
			repeat = &d.RepeatMonthly
		}
		sheet, err = s.AddMonthlyItem(d.Month, MonthlyItemInput{SectionID: d.SectionID, Name: d.Name, AmountMinor: d.AmountMinor, Note: d.Note, RepeatMonthly: repeat, UsualAmountMinor: d.UsualAmountMinor, Explanations: d.Explanations}, sheetVersion)
	}
	if err != nil {
		return MonthlySheet{}, err
	}
	if d.Amendment == nil {
		item, _ = s.findMonthlyItem(d.Name, section.Direction, "")
	}
	before := d
	d.State = "accepted"
	d.AcceptedItemID = &item.ID
	d.Touch()
	s.MonthlyDrafts[id] = d
	s.AuditChange("monthly_draft.accepted", id, "", before, d)
	return sheet, nil
}

func (s *State) acceptMonthlyRecurrenceChange(d MonthlyDraft, sheetVersion int64) (MonthlySheet, error) {
	if err := s.validateMonthlyProposal(d.Month); err != nil {
		return MonthlySheet{}, err
	}
	if err := CheckVersion(s.MonthlySheet.Version, sheetVersion); err != nil {
		return MonthlySheet{}, err
	}
	rc := d.RecurrenceChange
	item, ok := s.MonthlySheet.Items[rc.ItemID]
	if !ok {
		return MonthlySheet{}, NotFound()
	}
	if p := item.preference(d.Month); p.SectionID != rc.BaseSectionID || p.RepeatMonthly != rc.BaseRepeatMonthly || !sameMonthlyMoney(p.UsualAmountMinor, rc.BaseUsualAmountMinor) {
		return MonthlySheet{}, Fail(409, "ITEM_CHANGED", "This item's recurrence settings changed after the proposal was made. Reject it, or ask for a new proposal against the current settings")
	}
	sheet, err := s.SetMonthlyRecurrence(d.Month, item.ID, d.SectionID, d.RepeatMonthly, d.UsualAmountMinor, sheetVersion)
	if err != nil {
		return MonthlySheet{}, err
	}
	before := d
	d.State = "accepted"
	d.AcceptedItemID = &item.ID
	d.Touch()
	s.MonthlyDrafts[d.ID] = d
	s.AuditChange("monthly_draft.accepted", d.ID, "", before, d)
	return sheet, nil
}
