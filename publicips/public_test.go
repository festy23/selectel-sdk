package publicips_test

import (
	"context"
	"encoding/json"
	"errors"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/publicips"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func client(t *testing.T, wire transport) *publicips.Client {
	t.Helper()
	scope := types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}
	source := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Scheme: credentials.IAMToken, Binding: scope, Audience: "selectel", Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	})
	c, e := sdk.New(sdk.Config{AccountID: "account", Credentials: source, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{
		{Service: "network", APIGeneration: "v2", Scope: scope}: {ID: "neutron-endpoint", URL: "https://neutron.test", Scope: scope, APIGeneration: "v2", Audience: "selectel"}, {Service: "public_network", APIGeneration: "v1", Scope: scope}: {ID: "direct-endpoint", URL: "https://direct.test/public-net", Scope: scope, APIGeneration: "v1", Audience: "selectel"}}, Transport: transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(200, `{"token":{"expires_at":"2099-01-01T00:00:00Z","project":{"id":"project","domain":{"name":"account"}}}}`), nil
		}
		if r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal("auth")
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Project("project")
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Location("region", "pool")
	if e != nil {
		t.Fatal(e)
	}
	return r.PublicIPs()
}
func TestDirectPortCreateUsesCurrentProjectTokenBodyAndSeparateKind(t *testing.T) {
	c := client(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/public-net/v1/public_ports" {
			t.Fatal(r.Method, r.URL)
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("body")
		}
		if _, ok := body["project_id"]; ok {
			t.Fatal("legacy project body leaked")
		}
		if string(body["description"]) != "null" || string(body["admin_state_up"]) != "false" || string(body["security_group_ids"]) != "[]" {
			t.Fatalf("current API fields %s", body)
		}
		return response(201, `{"port":{"id":"direct-1","project_id":"project","network_id":"service-network","ip_address":"192.0.2.4","created_at":"2026-10-14T12:30:00+03:00","description":null}}`), nil
	})
	off := false
	got, e := c.DirectPorts.Create(context.Background(), publicips.DirectPortCreateSpec{Description: types.Null[string](), AdminStateUp: &off, SecurityGroupIDs: []string{}}, types.CallOptions{})
	if e != nil || got.Receipt == nil || got.Receipt.Outcome != types.Accepted || got.Data.Ref.Kind != "direct_public_port" || got.Data.Ref.Service != "public_network" || got.Data.CreatedAt.Format(time.RFC3339) != "2026-10-14T09:30:00Z" || got.Data.Description == nil {
		t.Fatalf("direct port %+v %v", got, e)
	}
}

func TestDirectLifecycleListsCurrentFiltersAndExactRefs(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "quotas") {
			return response(200, `{"network_direct_public_ips":[{"value":9007199254740993,"used":0}]}`), nil
		}
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "public_ports") {
			if r.URL.RawQuery != "description=&network_id=service-network&subnet=192.0.2.0%2F24" {
				t.Fatal(r.URL.RawQuery)
			}
			return response(200, `{"ports":[{"id":"p","project_id":"project","network_id":"service-network"}]}`), nil
		}
		if r.URL.Path != "/public-net/v1/public_ports/p" {
			t.Fatal(r.URL)
		}
		if r.Method == "PATCH" {
			var v map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&v)
			if string(v["description"]) != "null" || string(v["admin_state_up"]) != "false" || string(v["security_group_ids"]) != `["security-group"]` {
				t.Fatalf("patch %s", v)
			}
		}
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		return response(200, `{"port":{"id":"p","project_id":"project","network_id":"service-network","description":null}}`), nil
	})
	empty, n, subnet := "", "service-network", "192.0.2.0/24"
	got, e := c.DirectPorts.Collect(context.Background(), publicips.DirectPortListOptions{Description: &empty, NetworkID: &n, Subnet: &subnet}, pagination.Options{})
	if e != nil || !got.Complete || len(got.Items) != 1 {
		t.Fatalf("list %+v %v", got, e)
	}
	ref := *got.Items[0].Ref
	_, e = c.DirectPorts.Get(context.Background(), ref, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	off := false
	_, e = c.DirectPorts.Update(context.Background(), publicips.DirectPortUpdateRequest{Ref: ref, Spec: publicips.DirectPortUpdateSpec{Description: types.Null[string](), AdminStateUp: &off, SecurityGroupIDs: []string{"security-group"}}}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	q, e := c.DirectPorts.Quotas(context.Background(), types.CallOptions{})
	if e != nil || q.Data.NetworkDirectPublicIPs[0].Value.String() != "9007199254740993" {
		t.Fatalf("quota %+v %v", q, e)
	}
	deleted, e := c.DirectPorts.Delete(context.Background(), ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 5 {
		t.Fatalf("delete %+v %v %d", deleted, e, calls)
	}
}

func TestDirectPortUnknownDeleteAndMalformedAcceptedIdentity(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "DELETE" {
			return nil, io.ErrUnexpectedEOF
		}
		return response(201, `{"port":{"id":"p","project_id":"project","created_at":"invalid"}}`), nil
	})
	made, e := c.DirectPorts.Create(context.Background(), publicips.DirectPortCreateSpec{}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || made.Receipt.Outcome != types.Accepted || len(made.Receipt.ResourceRefs) != 1 || made.Receipt.ResourceRefs[0].ID != "p" {
		t.Fatalf("accepted %+v %v", made, e)
	}
	gone, e := c.DirectPorts.Delete(context.Background(), made.Receipt.ResourceRefs[0], types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || gone.Receipt.Outcome != types.Unknown || calls != 2 {
		t.Fatalf("delete replay %+v %v %d", gone, e, calls)
	}
}

func TestDirectErrorPreservesNumericProviderCodeWithoutMessage(t *testing.T) {
	c := client(t, func(*http.Request) (*http.Response, error) {
		return response(400, `{"code":9007199254740993,"msg":"synthetic-private-detail"}`), nil
	})
	got, e := c.DirectPorts.Create(context.Background(), publicips.DirectPortCreateSpec{}, types.CallOptions{})
	var sdkErr *types.SDKError
	if !errors.As(e, &sdkErr) || sdkErr.Code != types.ProviderError || sdkErr.ProviderCode != "9007199254740993" || strings.Contains(e.Error(), "synthetic-private-detail") || got.Receipt.Outcome != types.Rejected {
		t.Fatalf("direct error %+v %v", got, e)
	}
}

func TestDirectPortCollectionUsesBoundedLocalReplayAndValidatesUpdateSchema(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Has("limit") || r.URL.Query().Has("marker") {
			t.Fatal("undocumented remote pagination")
		}
		return response(200, `{"ports":[{"id":"p1","project_id":"project"},{"id":"p2","project_id":"project"}]}`), nil
	})
	one, e := c.DirectPorts.Collect(context.Background(), publicips.DirectPortListOptions{}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || one.NextCursor == nil || len(one.Items) != 1 {
		t.Fatalf("bounded %+v %v", one, e)
	}
	rest, e := c.DirectPorts.Collect(context.Background(), publicips.DirectPortListOptions{}, pagination.Options{Cursor: one.NextCursor})
	if e != nil || !rest.Complete || len(rest.Items) != 1 || *rest.Items[0].ID != "p2" || calls != 2 {
		t.Fatalf("replay %+v %v", rest, e)
	}
	invalid, e := c.DirectPorts.Update(context.Background(), publicips.DirectPortUpdateRequest{Ref: *one.Items[0].Ref, Spec: publicips.DirectPortUpdateSpec{SecurityGroupIDs: []string{}}}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || invalid.Receipt.Outcome != types.NotSent || calls != 2 {
		t.Fatalf("schema empty update %+v %v", invalid, e)
	}
}

func TestDirectExplicitNativeVariantsPreservePublishedSDKWire(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "POST" {
			var v map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&v)
			if string(v["project_id"]) != `"project"` || string(v["security_group_ids"]) != "null" {
				t.Fatalf("native body %s", v)
			}
			return response(201, `{"port":{"id":"p","project_id":"project"}}`), nil
		}
		want := "force=true"
		if calls == 3 {
			want = ""
		}
		if r.URL.RawQuery != want {
			t.Fatalf("current/native force %s want %s", r.URL, want)
		}
		return response(204, ""), nil
	})
	project := "project"
	made, e := c.DirectPorts.CreateNative(context.Background(), publicips.DirectPortNativeCreateSpec{ProjectID: &project}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	deleted, e := c.DirectPorts.DeleteNative(context.Background(), publicips.DirectPortNativeDeleteRequest{Ref: *made.Data.Ref, Force: true}, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 2 {
		t.Fatalf("native delete %+v %v", deleted, e)
	}
	_, e = c.DirectPorts.Delete(context.Background(), *made.Data.Ref, types.CallOptions{})
	if e != nil || calls != 3 {
		t.Fatal("current delete changed", e)
	}
}
func TestDirectNativeDeleteUnknownDoesNotFallbackOrReplay(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("force") != "true" {
			t.Fatal("native force")
		}
		return nil, io.ErrUnexpectedEOF
	})
	scope := types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}
	ref := types.ResourceRef{Service: "public_network", APIGeneration: "v1", Kind: "direct_public_port", ID: "p", Scope: scope, EndpointID: "direct-endpoint", EndpointURL: "https://direct.test/public-net"}
	got, e := c.DirectPorts.DeleteNative(context.Background(), publicips.DirectPortNativeDeleteRequest{Ref: ref, Force: true}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || got.Receipt.Outcome != types.Unknown || calls != 1 {
		t.Fatalf("native replay %+v %v %d", got, e, calls)
	}
}

func TestDirectNativeProjectFilterKeepsOwnerSeparateFromAuthView(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if r.URL.RawQuery != "project_id=other-owner" {
				t.Fatal("native query", r.URL)
			}
			return response(200, `{"ports":[{"id":"p","project_id":"other-owner"}]}`), nil
		}
		return response(200, `{"port":{"id":"p","project_id":"other-owner"}}`), nil
	})
	found, e := c.DirectPorts.CollectNative(context.Background(), publicips.DirectPortNativeListOptions{ProjectID: "other-owner", Description: ""}, pagination.Options{})
	if e != nil || !found.Complete || len(found.Items) != 1 || *found.Items[0].ProjectID != "other-owner" || found.Items[0].Ref.Scope.ProjectID != "project" {
		t.Fatalf("native owner/auth view %+v %v", found, e)
	}
	read, e := c.DirectPorts.Get(context.Background(), *found.Items[0].Ref, types.CallOptions{})
	if e != nil || read.Data.Ref.Scope.ProjectID != "project" || calls != 2 {
		t.Fatalf("exact visible read %+v %v", read, e)
	}
}

func TestDirectExplicitNativeUpdateKeepsSDKNullGroups(t *testing.T) {
	c := client(t, func(r *http.Request) (*http.Response, error) {
		var v map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&v)
		if r.Method != "PATCH" || string(v["security_group_ids"]) != "null" || string(v["description"]) != `""` {
			t.Fatalf("native update wire %s", v)
		}
		return response(200, `{"port":{"id":"p","project_id":"project"}}`), nil
	})
	scope := types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}
	ref := types.ResourceRef{Service: "public_network", APIGeneration: "v1", Kind: "direct_public_port", ID: "p", Scope: scope, EndpointID: "direct-endpoint", EndpointURL: "https://direct.test/public-net"}
	empty := ""
	got, e := c.DirectPorts.UpdateNative(context.Background(), publicips.DirectPortNativeUpdateRequest{Ref: ref, Spec: publicips.DirectPortNativeUpdateSpec{Description: &empty}}, types.CallOptions{})
	if e != nil || got.Receipt.Outcome != types.Accepted {
		t.Fatalf("native update %+v %v", got, e)
	}
}
