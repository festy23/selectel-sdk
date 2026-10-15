package privatedns_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/privatedns"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, b string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(b))}
}
func clients(t *testing.T, wire transport) (*sdk.Client, *privatedns.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "spb", Pool: "ru-1"}
	c, e := sdk.New(sdk.Config{AccountID: "a", PollInterval: time.Millisecond, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Mode != types.IAMProject || r.Service != "private_dns" || r.Scope != scope {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "private_dns", APIGeneration: "v1", Scope: scope}: {ID: "dns", URL: "https://private.test/v1", Scope: scope, APIGeneration: "v1", Audience: "selectel"}}, Transport: transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(200, `{"token":{"expires_at":"2099-01-01T00:00:00Z","project":{"id":"p","domain":{"name":"a"}}}}`), nil
		}
		if r.Header.Get("X-Auth-Token") != "synthetic" {
			t.Fatal("auth")
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Project("p")
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Location("spb", "ru-1")
	if e != nil {
		t.Fatal(e)
	}
	return c, r.PrivateDNS()
}
func TestCreateServicePreservesFalseAndExactIdentity(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v1/services" || string(b) != `{"is_recursor_enabled":false,"network_id":"b706af9b-b048-42a5-a87d-d69a601abec8"}` {
			t.Fatal(r.Method, r.URL, string(b))
		}
		return response(201, `{"service":{"id":"s","project":"owner","network_id":"b706af9b-b048-42a5-a87d-d69a601abec8","high_availability":true,"is_recursor_enabled":false,"addresses":[{"address":"192.168.0.1","cidr":"192.168.0.0/24"}]}}`), nil
	})
	f := false
	got, e := c.Services.Create(context.Background(), privatedns.ServiceCreate{NetworkID: "b706af9b-b048-42a5-a87d-d69a601abec8", IsRecursorEnabled: &f}, types.CallOptions{})
	if e != nil || got.Receipt == nil || got.Receipt.Outcome != types.Accepted || got.Data.Ref == nil || got.Data.Ref.ID != "s" || got.Data.Ref.Scope.ProjectID != "p" || got.Data.Project == nil || *got.Data.Project != "owner" || len(got.Data.Addresses) != 1 {
		t.Fatal(e)
	}
}
func TestServiceLifecycleAndZoneCreate(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if r.Method != "GET" || r.URL.Path != "/v1/services/s" {
				t.Fatal(r.Method, r.URL)
			}
			return response(200, `{"service":{"id":"s"}}`), nil
		case 2:
			b, _ := io.ReadAll(r.Body)
			if r.Method != "PUT" || string(b) != `{"is_recursor_enabled":false}` {
				t.Fatal(r.Method, string(b))
			}
			return response(204, ""), nil
		case 3:
			if r.Method != "DELETE" {
				t.Fatal(r.Method)
			}
			return response(204, ""), nil
		case 4:
			b, _ := io.ReadAll(r.Body)
			if r.Method != "POST" || r.URL.Path != "/v1/zones" || string(b) != `{"domain":"example.test","name":"my-zone","ttl":0}` {
				t.Fatal(r.Method, r.URL, string(b))
			}
			return response(201, `{"zone":{"id":"z","ttl":0,"serial_number":9007199254740993,"project":"other"}}`), nil
		}
		t.Fatal("unexpected")
		return nil, nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "spb", Pool: "ru-1"}, Service: "private_dns", APIGeneration: "v1", Kind: "service", ID: "s", EndpointID: "dns", EndpointURL: "https://private.test/v1"}
	if _, e := c.Services.Get(context.Background(), ref, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	f := false
	if got, e := c.Services.Update(context.Background(), ref, privatedns.ServiceUpdate{IsRecursorEnabled: &f}, types.CallOptions{}); e != nil || got.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	if _, e := c.Services.Delete(context.Background(), ref, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	zero, _ := types.ParseInteger("0")
	v, e := c.Zones.Create(context.Background(), privatedns.ZoneCreate{Name: "my-zone", Domain: "example.test", TTL: &zero}, types.CallOptions{})
	if e != nil || v.Data.SerialNumber.String() != "9007199254740993" || v.Data.Ref.ID != "z" {
		t.Fatal(e)
	}
}
func TestZoneUpdateAndRecordReplaceDifferFromValuePatch(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		b, _ := io.ReadAll(r.Body)
		switch calls {
		case 1:
			if r.Method != "GET" || r.URL.Path != "/v1/zones/z" {
				t.Fatal(r.URL)
			}
			return response(200, `{"zone":{"id":"z","records":[{"type":"TXT","domain":"example.test.","values":["private-data"]}],"bindings":[{"resource_id":"x","resource_type":"database"}]}}`), nil
		case 2:
			if r.Method != "PUT" || string(b) != `{"name":"renamed","ttl":0}` {
				t.Fatal(r.Method, string(b))
			}
			return response(200, `{"zone":{"id":"z","name":"renamed"}}`), nil
		case 3:
			if r.Method != "PUT" || r.URL.Path != "/v1/zones/z/recordset" || string(b) != `{"generated":false,"set":[{"domain":"example.test.","ttl":-1,"type":"TXT","values":["private-data"]}]}` {
				t.Fatal(r.Method, r.URL, string(b))
			}
			return response(200, `{"records":[{"type":"TXT","domain":"example.test.","ttl":-1,"values":["private-data"]}]}`), nil
		case 4:
			if r.Method != "PATCH" || string(b) != `{"records":[{"add":["private-data"],"delete":[],"domain":"example.test.","type":"TXT"}]}` {
				t.Fatal(r.Method, string(b))
			}
			return response(200, `{"records":[]}`), nil
		case 5:
			if r.Method != "DELETE" {
				t.Fatal(r.Method)
			}
			return response(204, ""), nil
		}
		t.Fatal("unexpected")
		return nil, nil
	})
	r := zoneRef()
	z, e := c.Zones.Get(context.Background(), r, types.CallOptions{})
	if e != nil || len(z.Data.Records) != 1 || len(z.Data.Bindings) != 1 {
		t.Fatal(e)
	}
	if string(z.Data.Records[0].Values[0].Reveal()) != "private-data" {
		t.Fatal("lost protected value")
	}
	zero, _ := types.ParseInteger("0")
	name := "renamed"
	if _, e = c.Zones.Update(context.Background(), r, privatedns.ZoneUpdate{Name: &name, TTL: &zero}, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	vals := []types.ProtectedValue{types.Protect([]byte("private-data"))}
	ttl, _ := types.ParseInteger("-1")
	sets := []privatedns.RecordSetAdd{{Type: "TXT", Domain: "example.test.", TTL: &ttl, Values: &vals}}
	f := false
	v, e := c.Records.Replace(context.Background(), r, privatedns.RecordSetReplace{Set: &sets, Generated: &f}, types.CallOptions{})
	if e != nil || v.Receipt.Outcome != types.Accepted || len(v.Data) != 1 {
		t.Fatal(e)
	}
	none := []types.ProtectedValue{}
	items := []privatedns.RecordPatchItem{{Type: "TXT", Domain: "example.test.", Add: &vals, Delete: &none}}
	if _, e = c.Records.PatchValues(context.Background(), r, privatedns.RecordSetPatch{Records: &items}, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Zones.Delete(context.Background(), r, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
}
func zoneRef() types.ResourceRef {
	return types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "spb", Pool: "ru-1"}, Service: "private_dns", APIGeneration: "v1", Kind: "zone", ID: "z", EndpointID: "dns", EndpointURL: "https://private.test/v1"}
}
func TestBoundedServiceZoneAndRecordsCollections(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v1/services":
			if r.URL.Query().Get("project") != "owner" {
				t.Fatal(r.URL)
			}
			return response(200, `{"services":[{"id":"s1"},{"id":"s2"}]}`), nil
		case "/v1/zones":
			if r.URL.Query().Get("project") != "owner" || r.URL.Query().Get("domain") != "example.test" {
				t.Fatal(r.URL)
			}
			return response(200, `{"zones":[{"id":"z"}]}`), nil
		case "/v1/zones/z/recordset":
			if r.URL.Query().Get("type") != "TXT" || r.URL.Query().Get("domain") != "example.test." {
				t.Fatal(r.URL)
			}
			return response(200, `{"records":[{"type":"TXT","domain":"example.test.","ttl":0,"values":["secret"]}]}`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	})
	v, e := c.Services.Collect(context.Background(), privatedns.ServiceListOptions{Project: "owner"}, pagination.Options{MaxItems: 1})
	if e == nil || v.Complete || len(v.Items) != 1 || v.NextCursor == nil {
		t.Fatal(e, v.Complete, len(v.Items))
	}
	z, e := c.Zones.Collect(context.Background(), privatedns.ZoneListOptions{Project: "owner", Domain: "example.test"}, pagination.Options{})
	if e != nil || !z.Complete || len(z.Items) != 1 || z.Items[0].Ref == nil {
		t.Fatal(e)
	}
	r, e := c.Records.Collect(context.Background(), zoneRef(), privatedns.RecordListOptions{Type: "TXT", Domain: "example.test."}, pagination.Options{})
	if e != nil || !r.Complete || len(r.Items) != 1 {
		t.Fatal(e)
	}
}
func TestExactRecoveryUnknownDoesNotReplayAndKnownNoIDConclusive(t *testing.T) {
	calls := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		if r.Method != "GET" || r.URL.Path != "/v1/zones/z" {
			t.Fatal("replay", r.Method, r.URL)
		}
		return response(200, `{"zone":{"id":"z"}}`), nil
	})
	got, e := c.Zones.Delete(context.Background(), zoneRef(), types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || got.Receipt.Outcome != types.Unknown || calls != 1 {
		t.Fatal(e)
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || recovered.ProofStatus != "indeterminate" || calls != 2 {
		t.Fatal(e)
	}
	status, e := h.Status(context.Background())
	if e != nil || status.Classification != "succeeded" || status.ProviderState != "present" {
		t.Fatal(e)
	}
	bad, e := c.Services.Create(context.Background(), privatedns.ServiceCreate{NetworkID: "bad"}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || bad.Receipt.Outcome != types.NotSent {
		t.Fatal(e)
	}
	h, e = root.Resume(*bad.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	before := calls
	known, e := h.Recover(context.Background())
	if e != nil || known.ProofStatus != "conclusive" || calls != before {
		t.Fatal(e)
	}
	if _, e = h.Wait(context.Background(), "zone.present"); types.ErrorCode(e) != types.CapabilityUnavailable {
		t.Fatal(e)
	}
}
func TestAcceptedMalformedIdentityAndValidationDoNotLoseReceipt(t *testing.T) {
	for _, tc := range []struct {
		body string
		refs int
	}{{`{"service":{"id":"s","addresses":"bad"}}`, 1}, {`{"service":{"id":123}}`, 0}} {
		t.Run(tc.body, func(t *testing.T) {
			root, c := clients(t, func(*http.Request) (*http.Response, error) { return response(201, tc.body), nil })
			v, e := c.Services.Create(context.Background(), privatedns.ServiceCreate{NetworkID: "b706af9b-b048-42a5-a87d-d69a601abec8"}, types.CallOptions{})
			if types.ErrorCode(e) != types.ProtocolError || v.Receipt.Outcome != types.Accepted || len(v.Receipt.ResourceRefs) != tc.refs {
				t.Fatal(e)
			}
			h, e := root.Resume(*v.Receipt)
			if e != nil {
				t.Fatal(e)
			}
			r, e := h.Recover(context.Background())
			if e != nil || r.ProofStatus != "conclusive" {
				t.Fatal(e)
			}
		})
	}
}
func TestForeignAndMovedExactReferencesNeverReachTarget(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) { t.Fatal("wrong target I/O"); return nil, nil })
	for _, change := range []func(*types.ResourceRef){func(r *types.ResourceRef) { r.Scope.ProjectID = "other" }, func(r *types.ResourceRef) { r.EndpointURL = "https://old.test/v1" }, func(r *types.ResourceRef) { r.Kind = "service" }, func(r *types.ResourceRef) { r.Parent = &types.ResourceRef{} }} {
		r := zoneRef()
		change(&r)
		v, e := c.Zones.Delete(context.Background(), r, types.CallOptions{})
		if e == nil || v.Receipt.Outcome != types.NotSent {
			t.Fatal(e)
		}
	}
}
func TestMalformedListRejectsDotIdentityWithoutCompleteClaim(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		return response(200, `{"zones":[{"id":"z"},{"id":".."}]}`), nil
	})
	v, e := c.Zones.Collect(context.Background(), privatedns.ZoneListOptions{}, pagination.Options{})
	if types.ErrorCode(e) != types.ProtocolError || v.Complete || len(v.Items) != 0 {
		t.Fatal(e, len(v.Items))
	}
}
func TestStructuredErrorsPreserveNumericCodeAndDiscardProviderText(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		return response(400, `{"code":9007199254740993,"message":"private-error-text"}`), nil
	})
	v, e := c.Zones.Create(context.Background(), privatedns.ZoneCreate{Name: "zone", Domain: "example.test"}, types.CallOptions{})
	var sdkerr *types.SDKError
	if !errors.As(e, &sdkerr) || sdkerr.Code != types.InvalidArgument || sdkerr.ProviderCode != "9007199254740993" || v.Receipt.Outcome != types.Rejected || strings.Contains(fmt.Sprintf("%+v", e), "private-error-text") {
		t.Fatal(e)
	}
}
func TestSourceConstraintsRejectBeforeHTTPAndProtectedEncoding(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached target")
		return nil, nil
	})
	badTTL, _ := types.ParseInteger("-2")
	values := []types.ProtectedValue{types.Protect([]byte("first")), types.Protect([]byte("second"))}
	for _, r := range []privatedns.RecordSetAdd{{Type: "TXT", Domain: "example.test.", TTL: &badTTL}, {Type: "CNAME", Domain: "example.test.", Values: &values}, {Type: "BOGUS", Domain: "example.test."}} {
		sets := []privatedns.RecordSetAdd{r}
		v, e := c.Records.Replace(context.Background(), zoneRef(), privatedns.RecordSetReplace{Set: &sets}, types.CallOptions{})
		if types.ErrorCode(e) != types.InvalidArgument || v.Receipt.Outcome != types.NotSent {
			t.Fatal(e)
		}
	}
	marker := []byte("private-record-encoded-marker")
	protected := []types.ProtectedValue{types.Protect(marker)}
	row := privatedns.RecordSet{Values: protected}
	for _, v := range []any{row, &row, []privatedns.RecordSet{row}, protected, &protected} {
		formatted := fmt.Sprintf("%+v %#v", v, v)
		encoded, _ := json.Marshal(v)
		if strings.Contains(formatted, string(marker)) || strings.Contains(formatted, fmt.Sprint(marker)) || strings.Contains(formatted, fmt.Sprintf("%#v", marker)) || strings.Contains(string(encoded), string(marker)) {
			t.Fatal("disclosure")
		}
	}
}
func TestWaitIsExistenceOnlyAndDefaultLocationIsSourceBound(t *testing.T) {
	root, _ := clients(t, func(*http.Request) (*http.Response, error) {
		return response(404, `{"code":4,"message":"private"}`), nil
	})
	h, e := root.Resource(zoneRef())
	if e != nil {
		t.Fatal(e)
	}
	v, e := h.Wait(context.Background(), "zone.absent")
	if e != nil || !v.Complete || v.Last.Classification != "absent" {
		t.Fatal(e)
	}
	if _, e = h.Wait(context.Background(), "dns.propagated"); types.ErrorCode(e) != types.CapabilityUnavailable {
		t.Fatal(e)
	}
	c, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		t.Fatal("unsupported pool credential lookup")
		return credentials.Material{}, nil
	}), Transport: transport(func(*http.Request) (*http.Response, error) { t.Fatal("unsupported pool request"); return nil, nil })})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("p")
	r, _ := p.Location("msk", "ru-6")
	v2, e := r.PrivateDNS().Services.Create(context.Background(), privatedns.ServiceCreate{NetworkID: "b706af9b-b048-42a5-a87d-d69a601abec8"}, types.CallOptions{})
	if types.ErrorCode(e) != types.EndpointUnavailable || v2.Receipt.Outcome != types.NotSent {
		t.Fatal(e)
	}
}
func TestOptionalDiagnosticBodyPreservesRefreshRetryAndForbidden(t *testing.T) {
	for _, status := range []int{401, 503, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			auth, gets := 0, 0
			scope := zoneRef().Scope
			c, e := sdk.New(sdk.Config{AccountID: "a", MaxJSONBytes: 256, RetryBaseDelay: time.Millisecond, RetryMaxDelay: time.Millisecond, Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
				return credentials.Material{Identity: "s", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMPassword, Username: types.Protect([]byte("synthetic-user")), Password: types.Protect([]byte("synthetic-password"))}, nil
			}), Transport: transport(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					auth++
					out := response(201, `{"token":{"expires_at":"2099-01-01T00:00:00Z","project":{"id":"p","domain":{"name":"a"}}}}`)
					out.Header.Set("X-Subject-Token", "synthetic")
					return out, nil
				}
				gets++
				if gets == 1 {
					return response(status, strings.Repeat("x", 1024)), nil
				}
				return response(200, `{"zone":{"id":"z"}}`), nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			p, _ := c.Project("p")
			r, _ := p.Location("spb", "ru-1")
			ref := zoneRef()
			ref.EndpointID = "selectel-private_dns.v1:ru-1"
			ref.EndpointURL = "https://ru-1.cloud.api.selcloud.ru/private-dns/v1"
			_, e = r.PrivateDNS().Zones.Get(context.Background(), ref, types.CallOptions{})
			if status == 403 {
				if types.ErrorCode(e) != types.PermissionDenied || gets != 1 {
					t.Fatal(e, gets)
				}
			} else if e != nil || gets != 2 || (status == 401 && auth != 2) {
				t.Fatal(e, gets, auth)
			}
		})
	}
}
