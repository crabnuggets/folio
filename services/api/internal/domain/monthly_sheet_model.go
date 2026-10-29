package domain

type MonthlySectionDefinition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Direction   string `json:"direction"`
	AmountLabel string `json:"amount_label"`
}
type MonthlyPreference struct {
	Month            string `json:"month"`
	SectionID        string `json:"section_id"`
	RepeatMonthly    bool   `json:"repeat_monthly"`
	UsualAmountMinor *Money `json:"usual_amount_minor"`
}
type MonthlyItemIdentity struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Direction   string              `json:"direction"`
	Aliases     []string            `json:"aliases"`
	Preferences []MonthlyPreference `json:"preferences"`
}
type MonthlyStoredRow struct {
	ItemID      string `json:"item_id"`
	SectionID   string `json:"section_id"`
	AmountMinor *Money `json:"amount_minor"`
	Note        string `json:"note"`
	Removed     bool   `json:"removed"`
	Generated   bool   `json:"generated"`
	Touched     bool   `json:"touched"`
}
type MonthlySheetData struct {
	Sections []MonthlySectionDefinition             `json:"sections"`
	Items    map[string]MonthlyItemIdentity         `json:"items"`
	Rows     map[string]map[string]MonthlyStoredRow `json:"rows"`
}
type MonthlyStoredUndo struct {
	Token   string           `json:"token"`
	Version int64            `json:"version"`
	Before  MonthlySheetData `json:"before"`
}
type MonthlySheetState struct {
	MonthlySheetData
	Version        int64              `json:"version"`
	CurrencyLocked bool               `json:"currency_locked"`
	Undo           *MonthlyStoredUndo `json:"undo"`
}
type MonthlyItemInput struct {
	SectionID        string  `json:"section_id"`
	Name             string  `json:"name"`
	AmountMinor      *Money  `json:"amount_minor"`
	Note             string  `json:"note"`
	RepeatMonthly    *bool   `json:"repeat_monthly,omitempty"`
	UsualAmountMinor *Money  `json:"usual_amount_minor"`
	ItemID           *string `json:"item_id"`
}
type MonthlyItemUpdate struct {
	SectionID        string `json:"section_id"`
	Name             string `json:"name"`
	AmountMinor      *Money `json:"amount_minor"`
	Note             string `json:"note"`
	RepeatMonthly    bool   `json:"repeat_monthly"`
	UsualAmountMinor *Money `json:"usual_amount_minor"`
}
type MonthlyUsualInput struct {
	SectionID *string `json:"section_id"`
}
type MonthlyUndoInput struct {
	UndoToken string `json:"undo_token"`
}
type MonthlySectionInput struct {
	Name      string `json:"name"`
	Direction string `json:"direction"`
}
type MonthlySectionRename struct {
	Name string `json:"name"`
}
type MonthlyItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	SectionID        string `json:"section_id"`
	AmountMinor      *Money `json:"amount_minor"`
	Note             string `json:"note"`
	RepeatMonthly    bool   `json:"repeat_monthly"`
	UsualAmountMinor *Money `json:"usual_amount_minor"`
}
type MonthlySection struct {
	MonthlySectionDefinition
	TotalMinor   Money         `json:"total_minor"`
	MissingCount int           `json:"missing_count"`
	Items        []MonthlyItem `json:"items"`
}
type MonthlySheet struct {
	Month            string           `json:"month"`
	Version          int64            `json:"version"`
	Currency         string           `json:"currency"`
	CurrencyExponent int              `json:"currency_exponent"`
	HasLegacyData    bool             `json:"has_legacy_data"`
	UndoToken        *string          `json:"undo_token"`
	MoneyInMinor     Money            `json:"money_in_minor"`
	MoneyOutMinor    Money            `json:"money_out_minor"`
	RemainderMinor   Money            `json:"remainder_minor"`
	MissingCount     int              `json:"missing_count"`
	Sections         []MonthlySection `json:"sections"`
}
type MonthlyItemSuggestion struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	SectionID          string  `json:"section_id"`
	SectionName        string  `json:"section_name"`
	Direction          string  `json:"direction"`
	RepeatMonthly      bool    `json:"repeat_monthly"`
	UsualAmountMinor   *Money  `json:"usual_amount_minor"`
	CurrentMonthItemID *string `json:"current_month_item_id"`
}
type MonthlyItemSuggestions struct {
	Items []MonthlyItemSuggestion `json:"items"`
}

func newMonthlySheetState() MonthlySheetState {
	return MonthlySheetState{Version: 1, MonthlySheetData: MonthlySheetData{
		Sections: []MonthlySectionDefinition{
			{"00000000-0000-4000-8000-000000000001", "Income", "in", "Amount"},
			{"00000000-0000-4000-8000-000000000002", "Other money in", "in", "Amount"},
			{"00000000-0000-4000-8000-000000000003", "Recurring expenses", "out", "Amount"},
			{"00000000-0000-4000-8000-000000000004", "Credit cards", "out", "Bill amount"},
			{"00000000-0000-4000-8000-000000000005", "Other money out", "out", "Amount"},
		}, Items: map[string]MonthlyItemIdentity{}, Rows: map[string]map[string]MonthlyStoredRow{},
	}}
}
