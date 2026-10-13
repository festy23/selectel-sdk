// Package billing provides typed balance and consumption reads.
package billing

import "github.com/festy23/selectel-sdk/types"

type BalanceSettings struct {
	Mode              *string  `json:"mode,omitempty"`
	Currency          *string  `json:"currency,omitempty"`
	AvailableBillings []string `json:"avaliable_billings,omitzero"`
	AvailableBalances []string `json:"avaliable_balances,omitzero"`
}
type Debt struct {
	ServiceID   *types.Integer `json:"service_id,omitempty"`
	ServiceType *string        `json:"service_type,omitempty"`
	Value       *types.Money   `json:"debt_value,omitempty"`
}
type Balance struct {
	ID    *types.Integer `json:"balance_id,omitempty"`
	Type  *string        `json:"balance_type,omitempty"`
	Value *types.Money   `json:"value,omitempty"`
}
type Billing struct {
	Type              *string      `json:"billing_type,omitempty"`
	BalancesValuesSum *types.Money `json:"balances_values_sum,omitempty"`
	Debt              []Debt       `json:"debt,omitzero"`
	FinalSum          *types.Money `json:"final_sum,omitempty"`
	DebtSum           *types.Money `json:"debt_sum,omitempty"`
	Balances          []Balance    `json:"balances,omitzero"`
}
type BalancesData struct {
	Settings   *BalanceSettings `json:"settings,omitempty"`
	Billings   []Billing        `json:"billings,omitzero"`
	DebtStatus *string          `json:"debt_status,omitempty"`
}
type BalancesResponse struct {
	Status *string       `json:"status,omitempty"`
	Data   *BalancesData `json:"data,omitempty"`
}
type SummaryRow struct {
	AccountID      *string        `json:"account_id,omitempty"`
	PeriodStartRaw *string        `json:"period_start,omitempty"`
	PeriodEndRaw   *string        `json:"period_end,omitempty"`
	Value          *types.Money   `json:"value,omitempty"`
	ItemsCount     *types.Integer `json:"items_count,omitempty"`
}
type SummaryResponse struct {
	Status *string      `json:"status,omitempty"`
	Data   []SummaryRow `json:"data"`
}
type Metric struct {
	ID       *string        `json:"id,omitempty"`
	Locale   *string        `json:"locale,omitempty"`
	Name     *string        `json:"name,omitempty"`
	Quantity *types.Integer `json:"quantity,omitempty"`
	Unit     *string        `json:"unit,omitempty"`
}
type ConsumptionRow struct {
	AccountID   *string      `json:"account_id,omitempty"`
	Metric      *Metric      `json:"metric,omitempty"`
	PeriodRaw   *string      `json:"period,omitempty"`
	ProviderKey *string      `json:"provider_key,omitempty"`
	Value       *types.Money `json:"value,omitempty"`
}
type ConsumptionResponse struct {
	Status *string          `json:"status,omitempty"`
	Data   []ConsumptionRow `json:"data"`
}
