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

// ValidDate accepts a real YYYY-MM-DD calendar date from 1900 through 9999.
func ValidDate(date string) bool {
	t, e := time.Parse("2006-01-02", date)
	return e == nil && t.Year() >= 1900 && t.Format("2006-01-02") == date
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
type Boundary struct {
	EffectiveMonth string `json:"effective_month"`
	Included       bool   `json:"included"`
}
type AccountInput struct {
	Alias               string     `json:"alias"`
	Kind                string     `json:"kind"`
	Currency            string     `json:"currency"`
	IncludeInLiquidCash bool       `json:"include_in_liquid_cash"`
	Active              bool       `json:"active"`
	FundingBoundary     []Boundary `json:"funding_boundary"`
}
type Account struct {
	Meta
	AccountInput
}
type EntryInput struct {
	FundingPlanLineID    *string `json:"funding_plan_line_id"`
	Kind                 string  `json:"kind"`
	AmountMinor          Money   `json:"amount_minor"`
	Currency             string  `json:"currency"`
	EffectiveDate        string  `json:"effective_date"`
	ReportingMonth       string  `json:"reporting_month"`
	ReportingMonthReason string  `json:"reporting_month_reason"`
	SourceAccountID      *string `json:"source_account_id"`
	DestinationAccountID *string `json:"destination_account_id"`
	Category             string  `json:"category"`
	RelatedEntryID       *string `json:"related_entry_id"`
	PlanLineID           *string `json:"plan_line_id"`
}
type Entry struct {
	Meta
	EntryInput
	Active           bool    `json:"active"`
	SupersedesID     *string `json:"supersedes_id"`
	CorrectionReason string  `json:"correction_reason"`
}
type EntryCorrection struct {
	Reason      string      `json:"reason"`
	Replacement *EntryInput `json:"replacement"`
}
type StatementInput struct {
	AccountID            string  `json:"account_id"`
	Currency             string  `json:"currency"`
	CycleStart           string  `json:"cycle_start"`
	CycleEnd             string  `json:"cycle_end"`
	IssueDate            string  `json:"issue_date"`
	DueDate              string  `json:"due_date"`
	ReportingMonth       string  `json:"reporting_month"`
	ReportingMonthReason string  `json:"reporting_month_reason"`
	AmountDueMinor       Money   `json:"amount_due_minor"`
	MinimumDueMinor      *Money  `json:"minimum_due_minor"`
	NewChargesMinor      *Money  `json:"new_charges_minor"`
	FeesMinor            *Money  `json:"fees_minor"`
	InterestMinor        *Money  `json:"interest_minor"`
	RefundsMinor         *Money  `json:"refunds_minor"`
	NetSpendingMinor     *Money  `json:"net_spending_minor"`
	SpendingEvidence     string  `json:"spending_evidence"`
	CarriesStatementID   *string `json:"carries_statement_id"`
	PlanLineID           *string `json:"plan_line_id"`
}
type Statement struct {
	Meta
	StatementInput
	Active         bool    `json:"active"`
	PayableActive  bool    `json:"payable_active"`
	SupersedesID   *string `json:"supersedes_id"`
	RevisionReason string  `json:"revision_reason"`
}
type StatementRevision struct {
	Reason      string         `json:"reason"`
	Replacement StatementInput `json:"replacement"`
}
type SettlementInput struct {
	EntryID     string `json:"entry_id"`
	AmountMinor Money  `json:"amount_minor"`
}
type Settlement struct {
	Meta
	SettlementInput
	StatementID string `json:"statement_id"`
}
type ObligationAdjustmentInput struct {
	AmountMinor   Money   `json:"amount_minor"`
	EffectiveDate string  `json:"effective_date"`
	Reason        string  `json:"reason"`
	RefundEntryID *string `json:"refund_entry_id"`
}
type ObligationAdjustment struct {
	Meta
	ObligationAdjustmentInput
	StatementID string `json:"statement_id"`
}
type SnapshotInput struct {
	AccountID   string    `json:"account_id"`
	AmountMinor Money     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	ObservedAt  time.Time `json:"observed_at"`
	Source      string    `json:"source"`
}
type Snapshot struct {
	Meta
	SnapshotInput
}
type ClaimInput struct {
	AmountMinor         Money   `json:"amount_minor"`
	Currency            string  `json:"currency"`
	ExpenseEntryID      *string `json:"expense_entry_id"`
	StatementID         *string `json:"statement_id"`
	ExpectedReceiptDate *string `json:"expected_receipt_date"`
	Description         string  `json:"description"`
	State               string  `json:"state"`
}
type Claim struct {
	Meta
	ClaimInput
}
type ReceiptMatchInput struct {
	ReceiptEntryID string  `json:"receipt_entry_id"`
	AmountMinor    Money   `json:"amount_minor"`
	RecoveryMonth  *string `json:"recovery_month"`
}
type ReceiptMatch struct {
	Meta
	ReceiptMatchInput
	ClaimID string `json:"claim_id"`
}
type PlanLine struct {
	ID                      string  `json:"id"`
	Kind                    string  `json:"kind"`
	ExpectedMinor           Money   `json:"expected_minor"`
	EstimatedRemainingMinor *Money  `json:"estimated_remaining_minor"`
	Complete                bool    `json:"complete"`
	AccountID               *string `json:"account_id"`
	RecurrenceID            *string `json:"recurrence_id"`
	Description             string  `json:"description"`
}
type PlanInput struct {
	Lines []PlanLine `json:"lines"`
}
type Plan struct {
	Version int64      `json:"version"`
	Lines   []PlanLine `json:"lines"`
}
type Checklist struct {
	IncomeReviewed      bool              `json:"income_reviewed"`
	CardsReviewed       bool              `json:"cards_reviewed"`
	ExpensesReviewed    bool              `json:"expenses_reviewed"`
	AllocationsReviewed bool              `json:"allocations_reviewed"`
	NoStatementReasons  map[string]string `json:"no_statement_reasons"`
}
type Month struct {
	ReviewedCardIDs []string   `json:"reviewed_card_ids"`
	Month           string     `json:"month"`
	Version         int64      `json:"version"`
	Revision        int64      `json:"revision"`
	State           string     `json:"state"`
	Checklist       Checklist  `json:"checklist"`
	ClosedAt        *time.Time `json:"closed_at"`
}
type MonthTransition struct {
	Reason    string    `json:"reason"`
	Checklist Checklist `json:"checklist"`
}
type RecurrenceInput struct {
	Kind          string  `json:"kind"`
	ExpectedMinor Money   `json:"expected_minor"`
	StartMonth    string  `json:"start_month"`
	EndMonth      *string `json:"end_month"`
	Description   string  `json:"description"`
	AccountID     *string `json:"account_id"`
}
type Recurrence struct {
	Meta
	RecurrenceInput
	SupersedesID *string `json:"supersedes_id"`
}
type PeriodInput struct {
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	ReportingMonth  string `json:"reporting_month"`
	AdditionsMinor  Money  `json:"additions_minor"`
	DeductionsMinor Money  `json:"deductions_minor"`
	Evidence        string `json:"evidence"`
}
type Period struct {
	Meta
	PeriodInput
	RemainderMinor     Money `json:"remainder_minor"`
	ProposedTopUpMinor Money `json:"proposed_top_up_minor"`
}

// DraftStatementCandidate retains an unknown payable until acceptance.
// The pointer deliberately shadows the posted DTO amount to preserve absence.
type DraftStatementCandidate struct {
	StatementInput
	AmountDueMinor *Money `json:"amount_due_minor,omitempty"`
}
type DraftInput struct {
	SourceID   string                   `json:"source_id"`
	ExternalID string                   `json:"external_id"`
	Entry      *EntryInput              `json:"entry"`
	Statement  *DraftStatementCandidate `json:"statement"`
	Evidence   string                   `json:"evidence"`
}
type Draft struct {
	Meta
	DraftInput
	State            string  `json:"state"`
	AcceptedRecordID *string `json:"accepted_record_id"`
	RejectionReason  string  `json:"rejection_reason"`
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
type LegacySummary struct {
	Meta
	Month             string `json:"month"`
	AmountMinor       Money  `json:"amount_minor"`
	SourceCoordinates string `json:"source_coordinates"`
	Complete          bool   `json:"complete"`
}
type State struct {
	ChatGPTConnection     *ChatGPTConnection              `json:"chatgpt_connection,omitempty"`
	MonthlySheet          MonthlySheetState               `json:"monthly_sheet"`
	Settings              Settings                        `json:"settings"`
	Accounts              map[string]Account              `json:"accounts"`
	Entries               map[string]Entry                `json:"entries"`
	Statements            map[string]Statement            `json:"statements"`
	Settlements           map[string]Settlement           `json:"settlements"`
	ObligationAdjustments map[string]ObligationAdjustment `json:"obligation_adjustments"`
	Snapshots             map[string]Snapshot             `json:"snapshots"`
	Claims                map[string]Claim                `json:"claims"`
	ReceiptMatches        map[string]ReceiptMatch         `json:"receipt_matches"`
	Plans                 map[string]Plan                 `json:"plans"`
	Months                map[string]Month                `json:"months"`
	Recurrences           map[string]Recurrence           `json:"recurrences"`
	Periods               map[string]Period               `json:"periods"`
	Drafts                map[string]Draft                `json:"drafts"`
	AuditEvents           []Audit                         `json:"audit_events"`
	LegacySummaries       []LegacySummary                 `json:"legacy_summaries"`
}

// ChatGPTConnection keeps the installation identity and encrypted OAuth credentials.
// API exports omit this private provider state.
type ChatGPTConnection struct {
	HostID      string `json:"host_id"`
	ClientID    string `json:"client_id"`
	Credentials string `json:"credentials"`
}

// NewState initializes empty collections and SGD settings in the Asia/Singapore timezone.
func NewState() *State {
	return &State{MonthlySheet: newMonthlySheetState(), Settings: Settings{1, "SGD", 2, "Asia/Singapore"}, Accounts: map[string]Account{}, Entries: map[string]Entry{}, Statements: map[string]Statement{}, Settlements: map[string]Settlement{}, ObligationAdjustments: map[string]ObligationAdjustment{}, Snapshots: map[string]Snapshot{}, Claims: map[string]Claim{}, ReceiptMatches: map[string]ReceiptMatch{}, Plans: map[string]Plan{}, Months: map[string]Month{}, Recurrences: map[string]Recurrence{}, Periods: map[string]Period{}, Drafts: map[string]Draft{}, AuditEvents: []Audit{}, LegacySummaries: []LegacySummary{}}
}

// Audit appends an event with a new identity and the current UTC timestamp.
func (s *State) Audit(action, id, reason string) {
	s.AuditEvents = append(s.AuditEvents, Audit{ID: NewID(), Action: action, EntityID: id, Reason: reason, At: time.Now().UTC()})
}

// EnsureOpen rejects invalid months and returns MONTH_CLOSED for closed months.
// A valid month without a stored review is treated as open.
func (s *State) EnsureOpen(month string) error {
	if !ValidMonth(month) {
		return Invalid("Invalid reporting month")
	}
	if s.Months[month].State == "closed" {
		return Fail(409, "MONTH_CLOSED", "Reopen the month before changing financial records")
	}
	return nil
}

// BumpMonth advances a month's version and revision, creating an open review if absent.
// Callers must validate month and check that it is open before financial mutations.
func (s *State) BumpMonth(month string) {
	m := s.Months[month]
	if m.Month == "" {
		m = Month{Month: month, Version: 1, State: "open", ReviewedCardIDs: []string{}, Checklist: Checklist{NoStatementReasons: map[string]string{}}}
	}
	m.Version++
	m.Revision++
	s.Months[month] = m
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
