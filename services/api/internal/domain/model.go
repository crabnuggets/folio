// Package domain contains Folio's financial rules, independent of HTTP and storage.
package domain

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

type Money int64

// MarshalJSON encodes minor units as a decimal integer string to preserve precision.
func (m Money) MarshalJSON() ([]byte, error) { return json.Marshal(strconv.FormatInt(int64(m), 10)) }

// UnmarshalJSON accepts a decimal integer string within the signed 64-bit range,
// allowing a leading minus for nonzero values, but no plus or leading zeros.
// Invalid input returns an error without changing m.
func (m *Money) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("amount must be an integer string")
	}
	if !regexp.MustCompile(`^(0|-?[1-9][0-9]*)$`).MatchString(v) {
		return fmt.Errorf("amount must be a canonical integer string")
	}
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil {
		return fmt.Errorf("amount exceeds signed 64-bit range")
	}
	*m = Money(n)
	return nil
}

type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields"`
}

// Error returns the public error message.
func (e *Error) Error() string { return e.Message }

// Fail constructs a domain error with the supplied HTTP status and public details.
func Fail(status int, code, message string) error {
	return &Error{status, code, message, map[string]string{}}
}

// Invalid constructs a 422 VALIDATION_FAILED error with the public message.
func Invalid(message string) error { return Fail(422, "VALIDATION_FAILED", message) }

// NotFound constructs a 404 error without identifying an inaccessible record.
func NotFound() error { return Fail(404, "NOT_FOUND", "Record not found") }

// CheckVersion returns a 412 STALE_VERSION error unless actual equals expected.
func CheckVersion(actual, expected int64) error {
	if actual != expected {
		return Fail(412, "STALE_VERSION", "Fetch the current record before changing it")
	}
	return nil
}

// NewID returns a random version 4 UUID and panics if secure randomness is unavailable.
func NewID() string { return fmt.Sprintf("%s", uuidBytes()) }

// uuidBytes returns a formatted version 4 UUID, panicking if random generation fails.
func uuidBytes() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ValidID accepts a UUID-shaped hexadecimal string with either letter case.
func ValidID(id string) bool {
	return regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`).MatchString(id)
}

// ValidMonth accepts a YYYY-MM month with a year from 1900 through 9999.
func ValidMonth(month string) bool {
	t, e := time.Parse("2006-01", month)
	return e == nil && t.Year() >= 1900 && t.Year() <= 9999 && t.Format("2006-01") == month
}

type Meta struct {
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewMeta creates version 1 metadata with a random ID and equal UTC timestamps.
func NewMeta() Meta { now := time.Now().UTC(); return Meta{NewID(), 1, now, now} }

// Touch advances the version and updates the modification timestamp in UTC.
func (m *Meta) Touch() { m.Version++; m.UpdatedAt = time.Now().UTC() }

type Settings struct {
	Version          int64  `json:"version"`
	Currency         string `json:"currency"`
	CurrencyExponent int    `json:"currency_exponent"`
	Timezone         string `json:"timezone"`
}
type SettingsInput struct {
	Currency string `json:"currency"`
	Timezone string `json:"timezone"`
}
type Audit struct {
	Before   json.RawMessage `json:"before"`
	After    json.RawMessage `json:"after"`
	ID       string          `json:"id"`
	Action   string          `json:"action"`
	EntityID string          `json:"entity_id"`
	Reason   string          `json:"reason"`
	At       time.Time       `json:"at"`
}
type State struct {
	MonthlySheet  MonthlySheetState       `json:"monthly_sheet"`
	MonthlyDrafts map[string]MonthlyDraft `json:"monthly_drafts"`
	Settings      Settings                `json:"settings"`
	AuditEvents   []Audit                 `json:"audit_events"`
	retained      map[string]json.RawMessage
}

// Unknown aggregate fields survive writes without loading retired financial types.
// They are never used for monthly calculations or exposed through the API.
func (s *State) UnmarshalJSON(data []byte) error {
	type current State
	v := current(*NewState())
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"monthly_sheet", "monthly_drafts", "settings", "audit_events"} {
		delete(fields, key)
	}
	*s = State(v)
	if len(fields) > 0 {
		s.retained = fields
	}
	return nil
}
func (s State) MarshalJSON() ([]byte, error) {
	type current State
	data, err := json.Marshal(current(s))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range s.retained {
		fields[key] = value
	}
	return json.Marshal(fields)
}
func NewState() *State {
	return &State{MonthlySheet: newMonthlySheetState(), MonthlyDrafts: map[string]MonthlyDraft{}, Settings: Settings{1, "SGD", 2, "Asia/Singapore"}, AuditEvents: []Audit{}}
}

// Audit appends an event with a new identity and the current UTC timestamp.
func (s *State) Audit(action, id, reason string) {
	s.AuditEvents = append(s.AuditEvents, Audit{ID: NewID(), Action: action, EntityID: id, Reason: reason, At: time.Now().UTC()})
}

// AuditChange appends an event with JSON snapshots of the old and new values.
// It panics if either value cannot be encoded.
func (s *State) AuditChange(action, id, reason string, before, after any) {
	b, e := json.Marshal(before)
	if e != nil {
		panic(e)
	}
	a, e := json.Marshal(after)
	if e != nil {
		panic(e)
	}
	s.AuditEvents = append(s.AuditEvents, Audit{ID: NewID(), Action: action, EntityID: id, Reason: reason, At: time.Now().UTC(), Before: b, After: a})
}
