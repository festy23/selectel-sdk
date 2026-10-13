package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

// Bind is used by the SDK's scoped client factory; Runtime is SDK-internal.
func Bind(runtime *core.Runtime, scope types.Scope) *Client {
	return &Client{runtime: runtime, scope: scope}
}

func operation(id, generation string) core.Operation {
	return core.Operation{ID: id, Service: "billing", APIGeneration: generation, Method: http.MethodGet, Effect: "read", Auth: types.IAMAccount, Alternatives: []types.AuthMode{types.StaticToken}, EndpointID: "selectel-billing", FixedEndpoint: "https://api.selectel.ru", Success: []int{200}}
}

func (c *Client) Balances(ctx context.Context, options types.CallOptions) (types.Result[BalancesResponse], error) {
	return core.DoJSON(c.runtime, ctx, operation("billing.v3.balances.get", "v3"), core.Request{Scope: c.scope, Path: "/v3/balances", Options: options}, decodeBalances)
}

type StatisticsRequest struct {
	ProviderKeys    []string
	Start, End      time.Time
	GroupType       string
	PeriodGroupType string
}
type SummaryRequest struct {
	StatisticsRequest
	ItemsCountPeriodGroupType string
}
type ConsumptionRequest struct {
	StatisticsRequest
	Locale string
}

func (r StatisticsRequest) query() (url.Values, error) {
	if len(r.ProviderKeys) == 0 || r.Start.IsZero() || r.End.IsZero() || !r.Start.Before(r.End) || !member(r.GroupType, "project", "object", "metric", "project_metric", "region_metric") || !period(r.PeriodGroupType) {
		return nil, types.NewError(types.InvalidArgument)
	}
	q := url.Values{"start": {r.Start.UTC().Format(time.RFC3339Nano)}, "end": {r.End.UTC().Format(time.RFC3339Nano)}, "group_type": {r.GroupType}, "period_group_type": {r.PeriodGroupType}}
	for _, key := range r.ProviderKeys {
		if !member(key, "vpc", "serverless", "mks", "dbaas", "storage", "cdn", "vmware", "craas", "ones", "mobfarm", "ses") {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Add("provider_keys", key)
	}
	return q, nil
}

func (c *Client) Summary(ctx context.Context, request SummaryRequest, options types.CallOptions) (types.Result[SummaryResponse], error) {
	q, validationErr := request.StatisticsRequest.query()
	if !period(request.ItemsCountPeriodGroupType) {
		validationErr = types.NewError(types.InvalidArgument)
	}
	if q != nil {
		q.Set("items_count_period_group_type", request.ItemsCountPeriodGroupType)
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[SummaryResponse], error) {
		var envelope struct {
			Status *string `json:"status"`
			Data   []struct {
				AccountID *string        `json:"account_id"`
				Start     *string        `json:"period_start"`
				End       *string        `json:"period_end"`
				Value     *types.Integer `json:"value"`
				Count     *types.Integer `json:"items_count"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Data == nil {
			return core.Decoded[SummaryResponse]{}, types.NewError(types.ProtocolError)
		}
		out := SummaryResponse{Status: envelope.Status, Data: make([]SummaryRow, len(envelope.Data))}
		for i, row := range envelope.Data {
			if row.AccountID != nil && *row.AccountID != c.scope.AccountID {
				return core.Decoded[SummaryResponse]{}, types.NewError(types.ScopeMismatch)
			}
			out.Data[i] = SummaryRow{AccountID: row.AccountID, PeriodStartRaw: row.Start, PeriodEndRaw: row.End, Value: money(row.Value, nil), ItemsCount: row.Count}
		}
		return core.Decoded[SummaryResponse]{Data: out}, nil
	}
	return core.DoJSON(c.runtime, ctx, operation("billing.v1.statistics.summary", "v1"), core.Request{Scope: c.scope, Path: "/v1/cloud_billing/statistic/summary_stats", Query: q, Options: options, Validate: func() error { return validationErr }}, decode)
}

func (c *Client) Consumption(ctx context.Context, request ConsumptionRequest, options types.CallOptions) (types.Result[ConsumptionResponse], error) {
	q, validationErr := request.StatisticsRequest.query()
	if !member(request.Locale, "ru", "en") {
		validationErr = types.NewError(types.InvalidArgument)
	}
	if q != nil {
		q.Set("locale", request.Locale)
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[ConsumptionResponse], error) {
		var envelope struct {
			Status *string `json:"status"`
			Data   []struct {
				AccountID   *string        `json:"account_id"`
				Metric      *Metric        `json:"metric"`
				Period      *string        `json:"period"`
				ProviderKey *string        `json:"provider_key"`
				Value       *types.Integer `json:"value"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Data == nil {
			return core.Decoded[ConsumptionResponse]{}, types.NewError(types.ProtocolError)
		}
		out := ConsumptionResponse{Status: envelope.Status, Data: make([]ConsumptionRow, len(envelope.Data))}
		for i, row := range envelope.Data {
			if row.AccountID != nil && *row.AccountID != c.scope.AccountID {
				return core.Decoded[ConsumptionResponse]{}, types.NewError(types.ScopeMismatch)
			}
			out.Data[i] = ConsumptionRow{AccountID: row.AccountID, Metric: row.Metric, PeriodRaw: row.Period, ProviderKey: row.ProviderKey, Value: money(row.Value, nil)}
		}
		return core.Decoded[ConsumptionResponse]{Data: out}, nil
	}
	return core.DoJSON(c.runtime, ctx, operation("billing.v1.statistics.consumption", "v1"), core.Request{Scope: c.scope, Path: "/v1/cloud_billing/statistic/consumption", Query: q, Options: options, Validate: func() error { return validationErr }}, decode)
}

func decodeBalances(raw []byte, _ types.EndpointBinding) (core.Decoded[BalancesResponse], error) {
	type rawDebt struct {
		ID    *types.Integer `json:"service_id"`
		Type  *string        `json:"service_type"`
		Value *types.Integer `json:"debt_value"`
	}
	type rawBalance struct {
		ID    *types.Integer `json:"balance_id"`
		Type  *string        `json:"balance_type"`
		Value *types.Integer `json:"value"`
	}
	type rawBilling struct {
		Type     *string        `json:"billing_type"`
		Sum      *types.Integer `json:"balances_values_sum"`
		Final    *types.Integer `json:"final_sum"`
		DebtSum  *types.Integer `json:"debt_sum"`
		Debt     []rawDebt      `json:"debt"`
		Balances []rawBalance   `json:"balances"`
	}
	var envelope struct {
		Status *string `json:"status"`
		Data   *struct {
			Settings   *BalanceSettings `json:"settings"`
			Billings   []rawBilling     `json:"billings"`
			DebtStatus *string          `json:"debt_status"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Data == nil {
		return core.Decoded[BalancesResponse]{}, types.NewError(types.ProtocolError)
	}
	data := &BalancesData{Settings: envelope.Data.Settings, DebtStatus: envelope.Data.DebtStatus}
	var currency *string
	if data.Settings != nil {
		currency = data.Settings.Currency
	}
	if envelope.Data.Billings != nil {
		data.Billings = make([]Billing, len(envelope.Data.Billings))
	}
	for i, row := range envelope.Data.Billings {
		b := Billing{Type: row.Type, BalancesValuesSum: money(row.Sum, currency), FinalSum: money(row.Final, currency), DebtSum: money(row.DebtSum, currency)}
		if row.Debt != nil {
			b.Debt = make([]Debt, len(row.Debt))
		}
		if row.Balances != nil {
			b.Balances = make([]Balance, len(row.Balances))
		}
		for j, d := range row.Debt {
			b.Debt[j] = Debt{ServiceID: d.ID, ServiceType: d.Type, Value: money(d.Value, currency)}
		}
		for j, v := range row.Balances {
			b.Balances[j] = Balance{ID: v.ID, Type: v.Type, Value: money(v.Value, currency)}
		}
		data.Billings[i] = b
	}
	return core.Decoded[BalancesResponse]{Data: BalancesResponse{Status: envelope.Status, Data: data}}, nil
}
func money(value *types.Integer, currency *string) *types.Money {
	if value == nil {
		return nil
	}
	var copy *string
	if currency != nil {
		v := *currency
		copy = &v
	}
	return &types.Money{Raw: value.String(), Currency: copy, Unit: "provider_unit_unknown"}
}
func member(value string, choices ...string) bool {
	for _, c := range choices {
		if value == c {
			return true
		}
	}
	return false
}
func period(p string) bool { return member(p, "hour", "day", "week", "month", "year", "all") }
