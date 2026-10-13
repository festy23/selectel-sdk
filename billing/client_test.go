package billing_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/billing"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func source() credentials.Source {
	return credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "revision", Scheme: credentials.StaticToken, Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Token: types.Protect([]byte("synthetic-token"))}, nil
	})
}
func reply(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

var options = types.CallOptions{AuthMode: types.StaticToken}

func TestBalancesPreserveAmountsAndUnknownUnits(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v3/balances" || req.Header.Get("X-Token") == "" || req.Header.Get("X-Auth-Token") != "" {
			t.Error("wrong endpoint or auth")
		}
		return reply(`{"status":"success","data":{"settings":{"currency":"RUB","mode":"postpaid","avaliable_billings":["main"]},"billings":[{"billing_type":"main","balances_values_sum":900719925474099312345,"final_sum":-10,"debt_sum":0,"balances":[{"balance_id":1,"balance_type":"main","value":0}],"debt":[{"service_id":2,"service_type":"vpc","debt_value":25}]}],"debt_status":"none"}}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Account().Billing().Balances(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	row := result.Data.Data.Billings[0]
	if row.BalancesValuesSum.Raw != "900719925474099312345" || row.FinalSum.Raw != "-10" || row.Balances[0].Value.Raw != "0" || row.BalancesValuesSum.Scale != nil || row.BalancesValuesSum.Unit != "provider_unit_unknown" || *row.BalancesValuesSum.Currency != "RUB" {
		t.Fatal("amount precision or semantics changed")
	}
}

func TestStatisticsQueryAndProviderTimestampArePreserved(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		q := req.URL.Query()
		if len(q["provider_keys"]) != 2 || q.Get("group_type") != "project" || q.Get("items_count_period_group_type") != "all" || q.Get("start") != "2026-10-01T00:00:00Z" {
			t.Error("query mapping changed")
		}
		return reply(`{"status":"success","data":[{"account_id":"test-account","period_start":"2026-10-01T00:00:00","period_end":"2026-10-02T00:00:00","value":123,"items_count":0}]}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	request := billing.SummaryRequest{StatisticsRequest: billing.StatisticsRequest{ProviderKeys: []string{"vpc", "mks"}, Start: start, End: start.Add(24 * time.Hour), GroupType: "project", PeriodGroupType: "day"}, ItemsCountPeriodGroupType: "all"}
	result, err := c.Account().Billing().Summary(context.Background(), request, options)
	if err != nil {
		t.Fatal(err)
	}
	if *result.Data.Data[0].PeriodStartRaw != "2026-10-01T00:00:00" || result.Data.Data[0].Value.Currency != nil || result.Data.Data[0].Value.Raw != "123" {
		t.Fatal("provider timezone or currency invented")
	}
}

func TestConsumptionKeepsQuantitySeparateFromCost(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("locale") != "en" {
			t.Error("locale missing")
		}
		return reply(`{"data":[{"account_id":"test-account","metric":{"id":"volume","quantity":672,"unit":"GB"},"period":"2026-10-01T00:00:00","provider_key":"vpc","value":1231}]}`), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	req := billing.ConsumptionRequest{StatisticsRequest: billing.StatisticsRequest{ProviderKeys: []string{"vpc"}, Start: start, End: start.Add(time.Hour), GroupType: "metric", PeriodGroupType: "hour"}, Locale: "en"}
	r, err := c.Account().Billing().Consumption(context.Background(), req, options)
	if err != nil {
		t.Fatal(err)
	}
	if r.Data.Data[0].Metric.Quantity.String() != "672" || *r.Data.Data[0].Metric.Unit != "GB" || r.Data.Data[0].Value.Raw != "1231" {
		t.Fatal("cost mixed with resource quantity")
	}
}

func TestMalformedNumberAndForeignAccountAreRejected(t *testing.T) {
	for _, raw := range []string{`{"data":[{"account_id":"different-account","value":1}]}`, `{"data":[{"account_id":"test-account","value":1.5}]}`, `{"data":[{"account_id":"test-account","value":"12"}]}`} {
		c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { return reply(raw), nil })})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		r, err := c.Account().Billing().Summary(context.Background(), billing.SummaryRequest{StatisticsRequest: billing.StatisticsRequest{ProviderKeys: []string{"vpc"}, Start: start, End: start.Add(time.Hour), GroupType: "project", PeriodGroupType: "hour"}, ItemsCountPeriodGroupType: "all"}, options)
		if err == nil || r.Data.Data != nil {
			t.Fatal("foreign/malformed financial data returned")
		}
		encoded, _ := json.Marshal(err)
		if strings.Contains(string(encoded), "different-account") {
			t.Fatal("provider payload in error")
		}
	}
}

func TestInvalidPeriodPerformsNoIO(t *testing.T) {
	count := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { count++; return reply(`{}`), nil })})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Account().Billing().Summary(context.Background(), billing.SummaryRequest{}, options)
	if types.ErrorCode(err) != types.InvalidArgument || count != 0 {
		t.Fatal("invalid statistics performed I/O")
	}
}
