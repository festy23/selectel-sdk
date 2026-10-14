package billing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/billing"
	"github.com/festy23/selectel-sdk/catalog"
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

func TestPredictionRetainsExplicitZeroNullAndHours(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/billing/prediction" || r.Header.Get("X-Token") == "" {
			t.Fatal("wrong prediction endpoint/auth")
		}
		return reply(`{"status":"success","data":{"primary":900719925474099312345,"storage":null,"vpc":0}}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.Account().Billing().Prediction(context.Background(), options)
	if e != nil || out.Data.Data.Primary.String() != "900719925474099312345" || out.Data.Data.Storage == nil || func() bool { _, present := out.Data.Data.Storage.Value(); return present }() || out.Data.Data.Vpc == nil {
		t.Fatal("prediction fields lost", e)
	}
	v, present := out.Data.Data.Vpc.Value()
	if !present || v.String() != "0" {
		t.Fatal("prediction zero lost")
	}
}

func TestBillingProjectsPreservesRepeatedProviderKeysAndProtectsNames(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if r.URL.Path != "/v1/cloud_billing/statistic/projects" || len(q["provider_keys"]) != 2 {
			t.Fatal("project filter lost")
		}
		return reply(`{"status":"success","data":[{"id":"p1","name":"synthetic-private-project-name"}]}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.Account().Billing().Projects(context.Background(), billing.ProjectsRequest{ProviderKeys: []string{"vpc", "mks"}}, options)
	if e != nil || len(out.Data.Data) != 1 || *out.Data.Data[0].ID != "p1" || string(out.Data.Data[0].Name.Reveal()) != "synthetic-private-project-name" {
		t.Fatal("project fields lost", e)
	}
	raw, e := json.Marshal(out.Data)
	if e != nil || strings.Contains(string(raw), "synthetic-private-project-name") {
		t.Fatal("private project label exposed")
	}
}

func TestBillingObjectsKeepsParentsAndOptionalLocale(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/cloud_billing/statistic/objects" || r.URL.Query().Has("locale") {
			t.Fatal("optional locale default invented")
		}
		return reply(`{"status":"success","data":[{"id":"o1","name":null,"parent_id":"parent","project_id":"p1","type":"cloud_vm","type_name":"Server"}]}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.Account().Billing().Objects(context.Background(), billing.ObjectsRequest{ProviderKeys: []string{"vpc"}}, options)
	if e != nil || len(out.Data.Data) != 1 || *out.Data.Data[0].ParentID != "parent" || *out.Data.Data[0].ProjectID != "p1" || out.Data.Data[0].Name != nil {
		t.Fatal("object fields lost", e)
	}
}

func TestDetailedReportRequiresExplicitEndpointAndKeepsExactMoney(t *testing.T) {
	account := "12345"
	scope := types.Scope{AccountID: account}
	lookups := 0
	credentialsSource := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		lookups++
		return credentials.Material{Identity: "synthetic", Revision: "1", Scheme: credentials.StaticToken, Binding: scope, Audience: credentials.SelectelAudience, Token: types.Protect([]byte("synthetic-token"))}, nil
	})
	calls := 0
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("X-Token") == "" || r.Header.Get("X-Auth-Token") != "" || r.URL.Query().Has("locale") {
			t.Fatal("report auth/default lost")
		}
		return reply(`{"status":"success","data":{"account_id":12345,"agreement_id":0,"locale":"en","project_ids":[],"year":2026,"month":10,"projects":[],"value":900719925474099312345,"paid_by_balance":[{"balance":"main","value":0}]}}`), nil
	})
	c, e := sdk.New(sdk.Config{AccountID: account, Credentials: credentialsSource, Transport: transport})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Account().Billing().DetailedReport(context.Background(), billing.ReportRequest{Year: 2026, Month: 10}, types.CallOptions{})
	if types.ErrorCode(e) != types.EndpointUnavailable || calls != 0 || lookups != 0 {
		t.Fatal("undocumented endpoint guessed", e)
	}
	c, e = sdk.New(sdk.Config{AccountID: account, Credentials: credentialsSource, Transport: transport, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "billing", APIGeneration: "v1", Scope: scope}: {ID: "explicit-billing", URL: "https://billing.example.invalid", APIGeneration: "v1", Scope: scope, Audience: credentials.SelectelAudience}}})
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.Account().Billing().DetailedReport(context.Background(), billing.ReportRequest{Year: 2026, Month: 10}, types.CallOptions{})
	if e != nil || out.Data.Data.Value.Raw != "900719925474099312345" || out.Data.Data.Value.Unit != "kopeck" || out.Data.Data.PaidByBalance[0].Value.Raw != "0" {
		t.Fatal("report money lost", e)
	}
}

func TestTransactionsPagePreservesNegativeAmountsAndProtectsContext(t *testing.T) {
	scope := types.Scope{AccountID: "test-account"}
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if r.Header.Get("X-Token") == "" || q.Get("limit") != "2" || q.Get("offset") != "0" || q.Get("without_removed") != "false" || q.Get("created_from") != "2026-10-01T00:00:00" {
			t.Fatal("transaction query/auth lost")
		}
		return reply(`{"status":"success","data":[{"user_id":1,"transaction_type":"withdraw","description":{"en":"synthetic-private-description","de":"synthetic-private-description"},"public_description":{},"balance":"main","dir":"outgoing","created":"2026-10-14T12:00:00.123456","price":-900719925474099312345,"state":"PAID","id_meta":{"id":[1],"billing":"seido"},"transaction_group":"withdraw","server_meta":{"en":{"operation":"payment","service":"synthetic-private-service","full_name":"synthetic-private-name"},"main_resource_uuid":"resource","equip_id":0,"service_type":1,"service_sub_type":0},"reason_for_debt":"synthetic-private-reason","jurbrand_key":"selectel"}]}`), nil
	}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "billing", APIGeneration: "v2", Scope: scope}: {ID: "explicit-transactions", URL: "https://billing.example.invalid", APIGeneration: "v2", Scope: scope, Audience: credentials.SelectelAudience}}})
	if e != nil {
		t.Fatal(e)
	}
	offset, _ := types.ParseInteger("0")
	without := false
	out, e := c.Account().Billing().TransactionsPage(context.Background(), billing.TransactionsRequest{CreatedFrom: "2026-10-01T00:00:00", CreatedTo: "2026-10-14T23:59:59", Offset: &offset, Limit: 2, WithoutRemoved: &without}, types.CallOptions{})
	if e != nil || len(out.Data.Data) != 1 || out.Data.Data[0].Price.Raw != "-900719925474099312345" || out.Data.Data[0].Created.UTC != nil || len(out.Data.Data[0].Description) != 2 {
		t.Fatal("transaction fields lost", e)
	}
	raw, e := json.Marshal(out.Data)
	if e != nil || strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("private transaction context exposed")
	}
	if fmt.Sprintf("%+v", out.Data.Data[0].ReasonForDebt) != "[REDACTED]" || fmt.Sprintf("%#v", out.Data.Data[0].ReasonForDebt) != "[REDACTED]" {
		t.Fatal("nullable protected bytes exposed by formatting")
	}
}

func TestDetailedReportMapsOfficialRecursiveFixture(t *testing.T) {
	scope := types.Scope{AccountID: "12345"}
	credentialsSource := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Scheme: credentials.StaticToken, Binding: scope, Audience: credentials.SelectelAudience, Token: types.Protect([]byte("synthetic-token"))}, nil
	})
	fixture, e := catalog.Default().Artifact("sources/billing-reports.json#/paths/~1v1~1billing~1report~1by_project~1detailed/get/responses/200/content/application~1json/example")
	for _, key := range []string{"paths", "/v1/billing/report/by_project/detailed", "get", "responses", "200", "content", "application/json", "example"} {
		var node map[string]json.RawMessage
		if json.Unmarshal(fixture, &node) != nil {
			t.Fatal("official fixture unavailable")
		}
		fixture = node[key]
	}
	if e != nil {
		t.Fatal(e)
	}
	c, e := sdk.New(sdk.Config{AccountID: "12345", Credentials: credentialsSource, Transport: transportFunc(func(*http.Request) (*http.Response, error) { return reply(string(fixture)), nil }), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "billing", APIGeneration: "v1", Scope: scope}: {ID: "explicit", URL: "https://billing.example.invalid", APIGeneration: "v1", Scope: scope, Audience: credentials.SelectelAudience}}})
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.Account().Billing().DetailedReport(context.Background(), billing.ReportRequest{Year: 2025, Month: 8}, types.CallOptions{})
	if e != nil || out.Data.Data.Value.Raw != "62639000" || len(out.Data.Data.Projects) != 2 || out.Data.Data.Projects[0].ID.Value != "proj-1" || out.Data.Data.Projects[0].Products[0].Objects[0].Resources[0].Quantity.String() != "22" || out.Data.Data.Projects[0].Products[0].Objects[0].Resources[0].ProvisionStart.UTC != nil {
		t.Fatal("nested report fields lost", e)
	}
}

func TestBillingNewReadsValidateBeforeIOAndDoNotExpandStaticOnlyAuth(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return reply(`{}`), nil })})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Account().Billing().Objects(context.Background(), billing.ObjectsRequest{}, options)
	if types.ErrorCode(e) != types.InvalidArgument || calls != 0 {
		t.Fatal("invalid providers reached transport", e)
	}
	_, e = c.Account().Billing().DetailedReport(context.Background(), billing.ReportRequest{Year: 2031, Month: 1}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 0 {
		t.Fatal("invalid report period reached transport", e)
	}
	_, e = c.Account().Billing().TransactionsPage(context.Background(), billing.TransactionsRequest{CreatedFrom: "bad", CreatedTo: "bad", Limit: 501}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 0 {
		t.Fatal("invalid transaction limits reached transport", e)
	}
}
