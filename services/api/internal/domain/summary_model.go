package domain

type StatementBalance struct {
	StatementID    string `json:"statement_id"`
	AmountDueMinor Money  `json:"amount_due_minor"`
	RemainingMinor Money  `json:"remaining_minor"`
	CreditMinor    Money  `json:"credit_minor"`
	SpendingMinor  *Money `json:"spending_minor"`
	PayableActive  bool   `json:"payable_active"`
}
type ForecastLine struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	ActualMinor    Money  `json:"actual_minor"`
	RemainingMinor Money  `json:"remaining_minor"`
	ForecastMinor  Money  `json:"forecast_minor"`
}
type Summary struct {
	Month                          string             `json:"month"`
	Currency                       string             `json:"currency"`
	Revision                       int64              `json:"revision"`
	IncomeMinor                    Money              `json:"income_minor"`
	GrossSpendingMinor             Money              `json:"gross_spending_minor"`
	ConfirmedRecoveriesMinor       Money              `json:"confirmed_recoveries_minor"`
	KnownSpendingMinor             Money              `json:"known_spending_minor"`
	NetPersonalCostMinor           Money              `json:"net_personal_cost_minor"`
	AllocationMinor                Money              `json:"allocation_minor"`
	MonthlySurplusMinor            *Money             `json:"monthly_surplus_minor"`
	ProvisionalSurplusMinor        Money              `json:"provisional_surplus_minor"`
	ConsumptionSavingsMinor        *Money             `json:"consumption_savings_minor"`
	SavingsRate                    *string            `json:"savings_rate"`
	FundingInflowsMinor            Money              `json:"funding_inflows_minor"`
	FundingOutflowsMinor           Money              `json:"funding_outflows_minor"`
	FundingRemainderMinor          Money              `json:"funding_remainder_minor"`
	ProjectedSurplusMinor          *Money             `json:"projected_surplus_minor"`
	ProjectedFundingRemainderMinor *Money             `json:"projected_funding_remainder_minor"`
	IsProvisional                  bool               `json:"is_provisional"`
	SpendingComplete               bool               `json:"spending_complete"`
	FundingComplete                bool               `json:"funding_complete"`
	MissingItems                   []string           `json:"missing_items"`
	Statements                     []StatementBalance `json:"statements"`
	ForecastLines                  []ForecastLine     `json:"forecast_lines"`
	Snapshots                      []Snapshot         `json:"snapshots"`
	UnmatchedClaimsMinor           Money              `json:"unmatched_claims_minor"`
	UnmatchedReceiptsMinor         Money              `json:"unmatched_receipts_minor"`
}
type Insight struct {
	Code    string           `json:"code"`
	Message string           `json:"message"`
	Values  map[string]Money `json:"values"`
}
