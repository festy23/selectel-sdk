package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/projects"
	"github.com/festy23/selectel-sdk/quotas"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestQuotaManagerReadsExactNumbersAndRepeatedFilters(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Host != "ru-1.cloud.api.selcloud.ru" || r.URL.Path != "/quota-manager/v1/projects/p1/quotas" || strings.Join(r.URL.Query()["resource"], ",") != "compute_cores,compute_ram" {
			t.Fatalf("quota wire %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"quotas":{"compute_cores":[{"zone":"ru-1a","value":9007199254740993,"used":0}],"compute_ram":[{"value":-1,"used":4}]}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Project("p1")
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Location("spb", "ru-1")
	if e != nil {
		t.Fatal(e)
	}
	got, e := r.Quotas().V1.Get(context.Background(), quotas.Filters{Resources: []string{"compute_cores", "compute_ram"}}, types.CallOptions{})
	if e != nil || got.Data.Quotas["compute_cores"][0].Value.String() != "9007199254740993" || got.Data.Quotas["compute_cores"][0].Used.String() != "0" || got.Data.Quotas["compute_ram"][0].Value.String() != "-1" {
		t.Fatalf("quotas %+v %v", got, e)
	}
}

func quotaView(t *testing.T, f func(*http.Request) (*http.Response, error)) *quotas.V1Client {
	t.Helper()
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), ReadMaxAttempts: 1, Transport: transportFunc(f)})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Project("p1")
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Location("spb", "ru-1")
	if e != nil {
		t.Fatal(e)
	}
	return r.Quotas().V1
}
func quotaResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestQuotaManagerPartial207ReadAndMutation(t *testing.T) {
	for _, mutation := range []bool{false, true} {
		t.Run(fmt.Sprint(mutation), func(t *testing.T) {
			calls := 0
			v := quotaView(t, func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				calls++
				if mutation {
					b, _ := io.ReadAll(r.Body)
					if r.Method != "PATCH" || string(b) != `{"quotas":{"compute_cores":[{"value":7,"zone":"ru-1a"}]}}` {
						t.Fatalf("update %s %s", r.Method, b)
					}
				}
				return quotaResponse(207, `{"quotas":{"compute_cores":[{"value":7,"used":1,"zone":"ru-1a"}]},"error":{"code":9007199254740993,"message":"private-quota-message","errors":[{"code":731,"resource":"compute_ram","zone":"ru-1b","message":"private-resource-message"}]}}`), nil
			})
			var provider *quotas.ProviderError
			if mutation {
				n, _ := types.ParseInteger("7")
				z := "ru-1a"
				got, e := v.Update(context.Background(), quotas.UpdateRequest{Quotas: map[string][]quotas.QuotaSet{"compute_cores": {{Value: &n, Zone: &z}}}}, types.CallOptions{})
				if types.ErrorCode(e) != types.Incomplete || got.Receipt == nil || got.Receipt.Outcome != types.Accepted || got.Receipt.Attempts != 1 || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].ID != "p1" || got.Data.Quotas["compute_cores"][0].Value.String() != "7" || got.Data.Quotas["compute_cores"][0].Used.String() != "1" {
					t.Fatalf("partial update %+v %v", got, e)
				}
				provider = got.Data.Error
			} else {
				got, e := v.Get(context.Background(), quotas.Filters{}, types.CallOptions{})
				if types.ErrorCode(e) != types.Incomplete || got.Data.Quotas["compute_cores"][0].Used.String() != "1" {
					t.Fatalf("partial get %+v %v", got, e)
				}
				provider = got.Data.Error
			}
			if calls != 1 || provider == nil || provider.Code.String() != "9007199254740993" || provider.Errors[0].Code.String() != "731" || *provider.Errors[0].Resource != "compute_ram" || *provider.Errors[0].Zone != "ru-1b" {
				t.Fatalf("partial errors %+v", provider)
			}
			rendered := fmt.Sprintf("%+v %#v", provider, provider)
			j, _ := json.Marshal(provider)
			if strings.Contains(rendered+string(j), "private-") {
				t.Fatal("provider message leaked")
			}
		})
	}
}

func TestQuotaManagerAccountAuthKeepsTargetScopeAndLimits(t *testing.T) {
	v := quotaView(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			if requestedScope(r).ProjectID != "" {
				t.Fatal("account alternative requested project token")
			}
			return response(types.Scope{AccountID: "test-account"}, time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/quota-manager/v1/projects/p1/limits" || r.URL.Query().Get("resource") != "compute_cores" {
			t.Fatalf("limits target %s", r.URL)
		}
		return quotaResponse(200, `{"quotas":{"compute_cores":[{"value":9007199254740993,"zone":"ru-1a","used":0}]}}`), nil
	})
	got, e := v.Limits(context.Background(), quotas.Filters{Resources: []string{"compute_cores"}}, types.CallOptions{AuthMode: types.IAMAccount})
	if e != nil || got.Metadata.Scope.ProjectID != "p1" || got.Metadata.Scope.Pool != "ru-1" || got.Data.Quotas["compute_cores"][0].Value.String() != "9007199254740993" || got.Data.Quotas["compute_cores"][0].Used.String() != "0" {
		t.Fatalf("account limits %+v %v", got, e)
	}
}

func TestQuotaManagerSDKFlatPartialErrors(t *testing.T) {
	v := quotaView(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return quotaResponse(207, `{"quotas":{},"errors":[{"code":451,"resource":"volume_gigabytes","zone":"ru-1b","message":"private-flat-message"}]}`), nil
	})
	got, e := v.Get(context.Background(), quotas.Filters{}, types.CallOptions{})
	if types.ErrorCode(e) != types.Incomplete || len(got.Data.Errors) != 1 || got.Data.Errors[0].Code.String() != "451" {
		t.Fatalf("flat error %+v %v", got, e)
	}
}

func TestQuotaManagerFailureOutcomesAndNoReplay(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		loss    bool
		want    types.Code
		outcome types.Outcome
	}{
		{"forbidden", 403, `{"message":"private-message"}`, false, types.PermissionDenied, types.Rejected},
		{"throttle", 429, `{}`, false, types.RateLimited, types.Unknown},
		{"server", 500, `{}`, false, types.ProviderError, types.Unknown},
		{"loss", 0, "", true, types.TransportError, types.Unknown},
		{"bad-accepted", 200, `{"quotas":{"x":[{"value":1.2}]}}`, false, types.ProtocolError, types.Accepted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			v := quotaView(t, func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				calls++
				if tc.loss {
					return nil, io.ErrUnexpectedEOF
				}
				return quotaResponse(tc.status, tc.body), nil
			})
			n, _ := types.ParseInteger("0")
			got, e := v.Update(context.Background(), quotas.UpdateRequest{Quotas: map[string][]quotas.QuotaSet{"x": {{Value: &n}}}}, types.CallOptions{})
			want := tc.want
			if tc.outcome == types.Unknown {
				want = types.UnknownOutcome
			}
			if types.ErrorCode(e) != want || got.Receipt == nil || got.Receipt.Outcome != tc.outcome || calls != 1 || len(got.Receipt.ResourceRefs) != 1 {
				t.Fatalf("failure %+v %v calls%d", got, e, calls)
			}
			if strings.Contains(fmt.Sprint(e), "private-message") {
				t.Fatal("leaked provider text")
			}
		})
	}
}
func TestQuotaManagerReadsKeepErrorStatusAndLimitsReject207(t *testing.T) {
	for _, status := range []int{403, 429, 400, 500, 207} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			v := quotaView(t, func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				calls++
				return quotaResponse(status, `{"quotas":{"x":[{"value":8}]}}`), nil
			})
			got, e := v.Limits(context.Background(), quotas.Filters{}, types.CallOptions{})
			want := types.ProviderError
			if status == 403 {
				want = types.PermissionDenied
			}
			if status == 429 {
				want = types.RateLimited
			}
			if types.ErrorCode(e) != want || got.Data.Quotas != nil || calls != 1 {
				t.Fatalf("read failure %+v %v", got, e)
			}
		})
	}
}
func TestQuotaManagerValidationBeforeHTTPAndNotSentReceipt(t *testing.T) {
	calls := 0
	v := quotaView(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, io.EOF })
	for _, request := range []quotas.UpdateRequest{{}, {Quotas: map[string][]quotas.QuotaSet{"x": {}}}, {Quotas: map[string][]quotas.QuotaSet{"x": {{}}}}, {Quotas: map[string][]quotas.QuotaSet{" ": {{}}}}} {
		got, e := v.Update(context.Background(), request, types.CallOptions{})
		if types.ErrorCode(e) != types.InvalidArgument || got.Receipt == nil || got.Receipt.Outcome != types.NotSent {
			t.Fatalf("validation %+v %v", got, e)
		}
	}
	if _, e := v.Get(context.Background(), quotas.Filters{Resources: []string{""}}, types.CallOptions{}); types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal(e)
	}
	if _, e := v.Get(context.Background(), quotas.Filters{}, types.CallOptions{AuthMode: types.Basic}); types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal(e)
	}
	if calls != 0 {
		t.Fatal("invalid calls did HTTP")
	}
}

func TestResellQuotaReadRemainsSeparateFromQuotaManager(t *testing.T) {
	for _, status := range []int{200, 400, 207} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), ReadMaxAttempts: 1, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					if requestedScope(r).ProjectID != "" {
						t.Fatal("Resell requested project auth")
					}
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				if r.URL.Host != "api.selectel.ru" || r.URL.Path != "/vpc/resell/v2/projects/p1" || r.URL.Query().Get("exclude_quotas") != "false" {
					t.Fatalf("Resell %s", r.URL)
				}
				return quotaResponse(status, `{"project":{"id":"p1","quotas":{"compute_cores":[{"region":"ru-1","zone":"ru-1a","value":9007199254740993,"used":0}]}}}`), nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			ref := types.ResourceRef{Service: "projects", APIGeneration: "v2", Kind: "project", ID: "p1", Scope: types.Scope{AccountID: "test-account"}, EndpointID: "selectel-resell", EndpointURL: "https://api.selectel.ru/vpc/resell"}
			exclude := false
			got, e := c.Account().Projects().Get(context.Background(), ref, projects.GetOptions{ExcludeQuotas: &exclude}, types.CallOptions{})
			if status == 200 {
				if e != nil || got.Data.Quotas["compute_cores"][0].Value.String() != "9007199254740993" || got.Data.Quotas["compute_cores"][0].Used.String() != "0" {
					t.Fatalf("Resell %+v %v", got, e)
				}
			} else {
				want := types.ProviderError
				if status == 400 {
					want = types.NotFound
				}
				if types.ErrorCode(e) != want || got.Data.Quotas != nil {
					t.Fatalf("existing error path %+v %v", got, e)
				}
			}
		})
	}
}

func TestQuotaManagerCatalogBindingsAndOfflineBoundary(t *testing.T) {
	c := catalog.Default()
	rows, e := c.List(catalog.Filter{Service: "quotas"})
	if e != nil || len(rows) != 3 {
		t.Fatalf("quota inventory %d %v", len(rows), e)
	}
	for _, id := range []string{"quotas.v1.limits.get", "quotas.v1.quotas.get", "quotas.v1.quotas.update"} {
		v, e := c.Describe(id)
		if e != nil || v.Implementation.Status != "implemented" || v.InputSchemaRef == nil || v.ResponseBindingRef == nil || v.Auth.Scheme != types.IAMProject || len(v.Auth.Alternatives) != 1 || v.Auth.Alternatives[0] != types.IAMAccount {
			t.Fatalf("binding %+v %v", v, e)
		}
		if strings.Join(v.ScopeRequirements, ",") != "account,project,region,pool" {
			t.Fatalf("scope %+v", v.ScopeRequirements)
		}
	}
	summary := c.Summary()
	if summary.InventoryComplete || summary.CoverageRatio != nil {
		t.Fatal("overclaimed denominator")
	}
	resell, e := c.Describe("projects.v2.projects.get")
	if e != nil || resell.Service != "projects" || resell.Auth.Scheme != types.IAMAccount {
		t.Fatalf("Resell binding %+v %v", resell, e)
	}
}

func TestQuotaManagerRetainsErrorOnlyPartialResponse(t *testing.T) {
	v := quotaView(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return quotaResponse(207, `{"error":{"code":17,"errors":[{"code":18,"resource":"compute_ram"}]}}`), nil
	})
	got, e := v.Get(context.Background(), quotas.Filters{}, types.CallOptions{})
	if types.ErrorCode(e) != types.Incomplete || got.Data.Error == nil || got.Data.Error.Errors[0].Code.String() != "18" || got.Data.Quotas == nil {
		t.Fatalf("error-only response %+v %v", got, e)
	}
}

func TestQuotaManager207WithoutErrorEnvelopeIsStillIncomplete(t *testing.T) {
	v := quotaView(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return quotaResponse(207, `{"quotas":{"compute_cores":[{"value":3}]}}`), nil
	})
	got, e := v.Get(context.Background(), quotas.Filters{}, types.CallOptions{})
	if types.ErrorCode(e) != types.Incomplete || got.Data.Quotas["compute_cores"][0].Value.String() != "3" {
		t.Fatalf("207 %+v %v", got, e)
	}
}
func TestQuotaManagerAccountReadRefreshesOnlyAccountToken(t *testing.T) {
	tokens, calls := 0, 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), ReadMaxAttempts: 2, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			tokens++
			if requestedScope(r).ProjectID != "" {
				t.Fatal("refreshed foreign project auth")
			}
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if calls == 1 {
			return quotaResponse(401, `{}`), nil
		}
		return quotaResponse(200, `{"quotas":{}}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Project("p1")
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Location("spb", "ru-1")
	if e != nil {
		t.Fatal(e)
	}
	got, e := r.Quotas().V1.Get(context.Background(), quotas.Filters{}, types.CallOptions{AuthMode: types.IAMAccount})
	if e != nil || tokens != 2 || calls != 2 || got.Metadata.Scope.ProjectID != "p1" {
		t.Fatalf("refresh %+v %v tokens%d calls%d", got, e, tokens, calls)
	}
}
