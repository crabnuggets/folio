package domain

import (
	"encoding/json"
	"strings"
)

// CreateDraft stages untrusted candidates without posting or requiring an open month.
func (s *State) CreateDraft(in DraftInput) (Draft, error) {
	if strings.TrimSpace(in.SourceID) == "" || len(in.SourceID) > 200 || strings.TrimSpace(in.ExternalID) == "" || len(in.ExternalID) > 200 || strings.TrimSpace(in.Evidence) == "" || len(in.Evidence) > 10000 {
		return Draft{}, Invalid("Draft source, external identity and evidence are required")
	}
	if (in.Entry == nil) == (in.Statement == nil) {
		return Draft{}, Invalid("Draft must contain exactly one entry or statement candidate")
	}
	for _, d := range s.Drafts {
		if d.SourceID == in.SourceID && d.ExternalID == in.ExternalID {
			a, _ := json.Marshal(d.DraftInput)
			b, _ := json.Marshal(in)
			if string(a) != string(b) {
				return Draft{}, Fail(409, "SOURCE_CONFLICT", "Source identity already has a different candidate")
			}
			return d, nil
		}
	}
	d := Draft{Meta: NewMeta(), DraftInput: in, State: "pending"}
	s.Drafts[d.ID] = d
	s.Audit("draft.created", d.ID, "")
	return d, nil
}
func (s *State) AcceptDraft(id string, expected int64) (Draft, error) {
	d, ok := s.Drafts[id]
	if !ok {
		return Draft{}, NotFound()
	}
	if e := CheckVersion(d.Version, expected); e != nil {
		return Draft{}, e
	}
	if d.State != "pending" {
		return Draft{}, Fail(409, "DRAFT_FINALIZED", "Draft has already been accepted or rejected")
	}
	var recordID string
	if d.Entry != nil {
		v, e := s.CreateEntry(*d.Entry)
		if e != nil {
			return Draft{}, e
		}
		recordID = v.ID
	} else if d.Statement != nil {
		v, e := s.CreateStatement(*d.Statement)
		if e != nil {
			return Draft{}, e
		}
		recordID = v.ID
	} else {
		return Draft{}, Invalid("Draft contains no candidate")
	}
	d.State = "accepted"
	d.AcceptedRecordID = &recordID
	d.Touch()
	s.Drafts[id] = d
	s.Audit("draft.accepted", id, "")
	return d, nil
}
func (s *State) RejectDraft(id, reason string, expected int64) (Draft, error) {
	d, ok := s.Drafts[id]
	if !ok {
		return Draft{}, NotFound()
	}
	if e := CheckVersion(d.Version, expected); e != nil {
		return Draft{}, e
	}
	if d.State != "pending" {
		return Draft{}, Fail(409, "DRAFT_FINALIZED", "Draft has already been accepted or rejected")
	}
	if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
		return Draft{}, Invalid("A rejection reason is required")
	}
	d.State = "rejected"
	d.RejectionReason = reason
	d.Touch()
	s.Drafts[id] = d
	s.Audit("draft.rejected", id, reason)
	return d, nil
}
