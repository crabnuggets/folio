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
	// Amendment is set when the proposal changes an item that already has an
	// amount. Nil means the proposal fills a blank item or adds one.
	Amendment      *MonthlyAmendment `json:"amendment"`
	State          string            `json:"state"`
	AcceptedItemID *string           `json:"accepted_item_id"`
	ProposalHash   string            `json:"proposal_hash"`
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
	parts := []string{in.Month, in.SectionID, monthlyNameKey(in.Name), itemID, strconv.FormatInt(int64(in.AmountMinor), 10)}
	for _, e := range in.Explanations {
		parts = append(parts, monthlyNameKey(e.Name), strconv.FormatInt(int64(e.AmountMinor), 10))
	}
	data, _ := json.Marshal(parts)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
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
				return MonthlyDraft{}, false, Fail(409, "SOURCE_CONFLICT", fmt.Sprintf("This source identity already belongs to a different %s proposal (id %s): %q for %s, amount_minor %d, with %d explanations. Tell the owner instead of proposing again. Propose under a new identity only if the owner asks for a separate proposal.", d.State, d.ID, d.Name, d.Month, d.AmountMinor, len(d.Explanations)))
			}
			return d, true, nil
		}
	}
	var amendment *MonthlyAmendment
	if itemID != "" {
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
		if *row.AmountMinor == in.AmountMinor && sameExplanations(row.Explanations, in.Explanations) {
			return MonthlyDraft{}, false, Invalid("The amendment matches the entered amount and explanations")
		}
		in.SectionID, in.Name = row.SectionID, s.MonthlySheet.Items[itemID].Name
		amendment = &MonthlyAmendment{ItemID: itemID, BaseAmountMinor: *row.AmountMinor, BaseExplanations: append([]MonthlyExplanation{}, row.Explanations...)}
	}
	if err := s.validateMonthlyDraft(in.Month, in.SectionID, in.Name, in.Note, in.AmountMinor, in.Explanations); err != nil {
		return MonthlyDraft{}, false, err
	}
	if err := CheckVersion(s.MonthlySheet.Version, in.SheetVersion); err != nil {
		return MonthlyDraft{}, false, err
	}
	d := MonthlyDraft{Meta: NewMeta(), MonthlyDraftInput: in, Amendment: amendment, State: "pending", ProposalHash: digest}
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
	if a := d.Amendment; a != nil {
		row, active := s.MonthlySheet.Rows[d.Month][a.ItemID]
		if !active || row.Removed || row.AmountMinor == nil || *row.AmountMinor != a.BaseAmountMinor || !sameExplanations(row.Explanations, a.BaseExplanations) {
			return MonthlySheet{}, Fail(409, "ITEM_CHANGED", "This item's amount or explanations changed after the amendment was proposed. Reject it, or ask for a new proposal against the current values")
		}
		item = s.MonthlySheet.Items[a.ItemID]
		p := item.preference(d.Month)
		sheet, err = s.UpdateMonthlyItem(d.Month, item.ID, MonthlyItemUpdate{SectionID: row.SectionID, Name: item.Name, AmountMinor: &d.AmountMinor, Note: row.Note, RepeatMonthly: p.RepeatMonthly, UsualAmountMinor: p.UsualAmountMinor, Explanations: append([]MonthlyExplanation{}, d.Explanations...)}, sheetVersion)
	} else if row, active := s.MonthlySheet.Rows[d.Month][item.ID]; exists && active && !row.Removed {
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
