package network_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/network"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func client(t *testing.T, wire transport) *network.Client {
	t.Helper()
	scope := types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}
	source := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Scheme: credentials.IAMToken, Binding: scope, Audience: "selectel", Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	})
	c, e := sdk.New(sdk.Config{AccountID: "account", Credentials: source, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "network", APIGeneration: "v2", Scope: scope}: {ID: "network-endpoint", URL: "https://neutron.test", Scope: scope, APIGeneration: "v2", Audience: "selectel"}}, Transport: transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return reply(200, `{"token":{"expires_at":"2099-01-01T00:00:00Z","project":{"id":"project","domain":{"name":"account"}}}}`), nil
		}
		if r.URL.Host != "neutron.test" || r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal("endpoint/auth lost")
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
	return r.Network()
}
func TestNetworkCreatePreservesExplicitValuesAndExactReceipt(t *testing.T) {
	c := client(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v2.0/networks" {
			t.Fatalf("network wire %s %s", r.Method, r.URL)
		}
		var envelope map[string]map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&envelope) != nil {
			t.Fatal("body")
		}
		body := envelope["network"]
		if string(body["name"]) != `""` || string(body["admin_state_up"]) != "false" || string(body["shared"]) != "false" || string(body["dns_domain"]) != `"private.example."` {
			t.Fatalf("source values %s", envelope["network"])
		}
		return reply(201, `{"network":{"id":"n1","project_id":"project","tenant_id":"project","admin_state_up":false,"mtu":9007199254740993,"created_at":"2026-10-14T12:00:00.000000","blocked":false}}`), nil
	})
	empty, domain := "", "private.example."
	off := false
	got, e := c.Networks.Create(context.Background(), network.NetworkCreateSpec{Name: &empty, AdminStateUp: &off, Shared: &off, DNSDomain: &domain}, types.CallOptions{})
	if e != nil || got.Receipt == nil || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != 1 || got.Data.Ref == nil || got.Data.Ref.ID != "n1" || got.Data.Ref.Service != "network" || got.Data.MTU.String() != "9007199254740993" || *got.Data.CreatedAt != "2026-10-14T12:00:00.000000" {
		t.Fatalf("network %+v %v", got, e)
	}
}

func TestNetworkAcceptedMalformedBodyKeepsExactIdentity(t *testing.T) {
	c := client(t, func(*http.Request) (*http.Response, error) {
		return reply(201, `{"network":{"id":"n1","project_id":"project","mtu":1.25}}`), nil
	})
	got, e := c.Networks.Create(context.Background(), network.NetworkCreateSpec{}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || got.Receipt == nil || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].ID != "n1" {
		t.Fatalf("accepted malformed %+v %v", got, e)
	}
}

func TestSharedNetworkLifecycleKeepsOwnerSeparateFromAuthScope(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v2.0/networks/shared" {
			t.Fatal(r.URL)
		}
		switch r.Method {
		case "GET":
			if r.URL.Query().Has("fields") {
				return reply(200, `{"network":{"name":"shared","shared":true}}`), nil
			}
			return reply(200, `{"network":{"id":"shared","project_id":"other-owner","tenant_id":"other-owner","shared":true}}`), nil
		case "PUT":
			if r.Header.Get("If-Match") != "revision_number=0" {
				t.Fatal("revision condition lost")
			}
			return reply(200, `{"network":{"id":"shared","project_id":"other-owner","name":""}}`), nil
		case "DELETE":
			return reply(204, ""), nil
		}
		return nil, nil
	})
	ref := types.ResourceRef{Service: "network", APIGeneration: "v2", Kind: "network", ID: "shared", Scope: types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}, EndpointID: "network-endpoint", EndpointURL: "https://neutron.test"}
	got, e := c.Networks.Get(context.Background(), ref, network.NetworkGetOptions{}, types.CallOptions{})
	if e != nil || *got.Data.ProjectID != "other-owner" || got.Data.Ref.Scope.ProjectID != "project" {
		t.Fatalf("shared owner %+v %v", got, e)
	}
	projected, e := c.Networks.Get(context.Background(), ref, network.NetworkGetOptions{Fields: []network.NetworkField{network.NetworkFieldName, network.NetworkFieldShared}}, types.CallOptions{})
	if e != nil || projected.Data.ID != nil || projected.Data.Ref.ID != "shared" {
		t.Fatalf("projection %+v %v", projected, e)
	}
	zero, _ := types.ParseInteger("0")
	empty := ""
	updated, e := c.Networks.Update(context.Background(), network.NetworkUpdateRequest{Ref: ref, Spec: network.NetworkUpdateSpec{Name: &empty}, Revision: &zero}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted || updated.Data.Ref.Scope.ProjectID != "project" {
		t.Fatalf("update shared %+v %v", updated, e)
	}
	deleted, e := c.Networks.Delete(context.Background(), network.DeleteRequest{Ref: ref}, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 4 {
		t.Fatalf("delete %+v %v", deleted, e)
	}
}

func TestChildrenOfSharedNetworkHaveExactAuthViewAndSourceOnlyBodies(t *testing.T) {
	c := client(t, func(r *http.Request) (*http.Response, error) {
		kind := "subnet"
		if strings.Contains(r.URL.Path, "ports") {
			kind = "port"
		}
		if r.Method == "DELETE" {
			return reply(204, ""), nil
		}
		if r.Method == "POST" || r.Method == "PUT" {
			var body map[string]map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Fatal("body")
			}
			v := body[kind]
			if _, ok := v["ref"]; ok {
				t.Fatal("SDK ref leaked")
			}
			if r.Method == "POST" && string(v["network_id"]) != `"shared"` {
				t.Fatal("parent omitted")
			}
			if kind == "subnet" && string(v["gateway_ip"]) != "null" {
				t.Fatal("explicit null omitted")
			}
			if kind == "port" && r.Method == "PUT" && string(v["security_groups"]) != "[]" {
				t.Fatal("explicit empty list omitted")
			}
		}
		status := 200
		if r.Method == "POST" {
			status = 201
		}
		return reply(status, `{"`+kind+`":{"id":"child","network_id":"shared","project_id":"project"}}`), nil
	})
	parent := types.ResourceRef{Service: "network", APIGeneration: "v2", Kind: "network", ID: "shared", Scope: types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}, EndpointID: "network-endpoint", EndpointURL: "https://neutron.test"}
	four, _ := types.ParseInteger("4")
	null := types.Null[string]()
	s, e := c.Subnets.Create(context.Background(), network.SubnetCreateRequest{Network: parent, Spec: network.SubnetCreateSpec{IPVersion: &four, GatewayIP: null}}, types.CallOptions{})
	if e != nil || s.Data.Ref.Parent.ID != "shared" {
		t.Fatalf("subnet %+v %v", s, e)
	}
	_, e = c.Subnets.Get(context.Background(), *s.Data.Ref, network.SubnetGetOptions{}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Subnets.Update(context.Background(), network.SubnetUpdateRequest{Ref: *s.Data.Ref, Spec: network.SubnetUpdateSpec{GatewayIP: null}}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Subnets.Delete(context.Background(), network.DeleteRequest{Ref: *s.Data.Ref}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Ports.Create(context.Background(), network.PortCreateRequest{Network: parent}, types.CallOptions{})
	if e != nil || p.Data.Ref.Parent.ID != "shared" {
		t.Fatalf("port %+v %v", p, e)
	}
	_, e = c.Ports.Get(context.Background(), *p.Data.Ref, network.PortGetOptions{}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Ports.Update(context.Background(), network.PortUpdateRequest{Ref: *p.Data.Ref, Spec: network.PortUpdateSpec{SecurityGroups: []string{}}}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Ports.Delete(context.Background(), network.DeleteRequest{Ref: *p.Data.Ref}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
}

func TestNetworkListPreservesAccessibleSelectionAndBoundedCursor(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		if q.Has("project_id") || len(q["name"]) != 2 || q["name"][0] != "" || q["name"][1] != "shared" || q.Get("sort_key") != "name" || q.Get("sort_dir") != "asc" {
			t.Fatalf("selection %s", r.URL)
		}
		if q.Get("marker") == "" {
			return reply(200, `{"networks":[{"id":"a","project_id":"other-owner","shared":true}],"networks_links":[{"rel":"next","href":"https://neutron.test/v2.0/networks?marker=a&limit=1"}]}`), nil
		}
		if q.Get("marker") != "a" {
			t.Fatal("cursor")
		}
		return reply(200, `{"networks":[{"id":"b","project_id":"project"}],"networks_links":[]}`), nil
	})
	f := network.NetworkFilters{Filters: map[network.NetworkField][]string{network.NetworkFieldName: {"", "shared"}}, SortKeys: []network.NetworkField{network.NetworkFieldName}, SortDirs: []string{"asc"}}
	got, e := c.Networks.Collect(context.Background(), f, pagination.Options{PageSize: 2, MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || got.Complete || got.NextCursor == nil || len(got.Items) != 1 || got.Items[0].Ref.Scope.ProjectID != "project" {
		t.Fatalf("bounded %+v %v", got, e)
	}
	resumed, e := c.Networks.Collect(context.Background(), f, pagination.Options{Cursor: got.NextCursor})
	if e != nil || !resumed.Complete || len(resumed.Items) != 1 || *resumed.Items[0].ID != "b" || calls != 2 {
		t.Fatalf("resume %+v %v", resumed, e)
	}
	f.Filters[network.NetworkFieldName] = []string{"changed"}
	_, e = c.Networks.Iterate(f, pagination.Options{Cursor: got.NextCursor})
	if types.ErrorCode(e) != types.ScopeMismatch {
		t.Fatal("cursor not bound to selection", e)
	}
}
func TestChildrenProjectionReverseAndUntrustedLink(t *testing.T) {
	reverse := true
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if strings.Contains(r.URL.Path, "subnets") {
			if r.URL.Query().Get("page_reverse") != "true" {
				t.Fatal("reverse omitted")
			}
			return reply(200, `{"subnets":[{"name":"projected"}],"subnets_links":[]}`), nil
		}
		return reply(200, `{"ports":[],"ports_links":[{"rel":"next","href":"https://attacker.test/v2.0/ports?marker=stolen"}]}`), nil
	})
	projected, e := c.Subnets.Collect(context.Background(), network.SubnetFilters{Fields: []network.SubnetField{network.SubnetFieldName}, PageReverse: &reverse}, pagination.Options{})
	if e != nil || len(projected.Items) != 1 || projected.Items[0].Ref != nil {
		t.Fatalf("projection %+v %v", projected, e)
	}
	got, e := c.Ports.Collect(context.Background(), network.PortFilters{}, pagination.Options{})
	if types.ErrorCode(e) != types.ProtocolError || got.Complete || calls != 2 {
		t.Fatalf("unsafe link %+v %v", got, e)
	}
}

func TestNetworkGuardsRejectBeforeWire(t *testing.T) {
	calls := 0
	c := client(t, func(*http.Request) (*http.Response, error) { calls++; return reply(500, ""), nil })
	ref := types.ResourceRef{Service: "network", APIGeneration: "v2", Kind: "network", ID: "shared", Scope: types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}, EndpointID: "network-endpoint", EndpointURL: "https://neutron.test"}
	foreign := ref
	foreign.Scope.ProjectID = "other"
	_, e := c.Networks.Get(context.Background(), foreign, network.NetworkGetOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch {
		t.Fatal(e)
	}
	_, e = c.Networks.Get(context.Background(), ref, network.NetworkGetOptions{Fields: []network.NetworkField{"binding:host_id"}}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal(e)
	}
	project, tenant := "project", "other"
	result, e := c.Networks.Create(context.Background(), network.NetworkCreateSpec{ProjectID: &project, TenantID: &tenant}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || result.Receipt.Outcome != types.NotSent {
		t.Fatalf("aliases %+v %v", result, e)
	}
	child := ref
	child.Kind = "port"
	child.Parent = &foreign
	_, e = c.Ports.Delete(context.Background(), network.DeleteRequest{Ref: child}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch {
		t.Fatal(e)
	}
	if calls != 0 {
		t.Fatal("invalid request reached provider", calls)
	}
}
func TestNetworkUnknownMutationNeverReplaysAndExplicitOwnerDoesNotSwitchAuth(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" {
			t.Fatal(r.Method)
		}
		var body map[string]map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["network"]["project_id"] != "other-owner" {
			t.Fatal("target owner")
		}
		return nil, io.ErrUnexpectedEOF
	})
	owner := "other-owner"
	got, e := c.Networks.Create(context.Background(), network.NetworkCreateSpec{ProjectID: &owner}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || got.Receipt.Outcome != types.Unknown || calls != 1 || got.Receipt.Scope.ProjectID != "project" {
		t.Fatalf("unknown %+v %v %d", got, e, calls)
	}
}
func TestNetworkRejectedMutationAndMalformedParent(t *testing.T) {
	c := client(t, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "subnets") {
			return reply(201, `{"subnet":{"id":"s","network_id":"wrong"}}`), nil
		}
		return reply(409, `{"NeutronError":{"type":"Conflict","message":"provider-private-detail"}}`), nil
	})
	got, e := c.Networks.Create(context.Background(), network.NetworkCreateSpec{}, types.CallOptions{})
	if e == nil || got.Receipt.Outcome != types.Rejected || strings.Contains(e.Error(), "provider-private-detail") {
		t.Fatalf("rejection %+v %v", got, e)
	}
	parent := types.ResourceRef{Service: "network", APIGeneration: "v2", Kind: "network", ID: "shared", Scope: types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}, EndpointID: "network-endpoint", EndpointURL: "https://neutron.test"}
	four, _ := types.ParseInteger("4")
	sub, e := c.Subnets.Create(context.Background(), network.SubnetCreateRequest{Network: parent, Spec: network.SubnetCreateSpec{IPVersion: &four}}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || sub.Receipt.Outcome != types.Accepted {
		t.Fatalf("parent %+v %v", sub, e)
	}
}

func TestPortUpdatePreservesMACAndNullableDHCPOption(t *testing.T) {
	c := client(t, func(r *http.Request) (*http.Response, error) {
		var body map[string]map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("body")
		}
		if string(body["port"]["mac_address"]) != `"fa:16:3e:12:34:56"` || string(body["port"]["extra_dhcp_opts"]) != `[{"opt_name":"bootfile-name","opt_value":null}]` {
			t.Fatalf("port source values %s", body["port"])
		}
		return reply(200, `{"port":{"id":"p","network_id":"n","extra_dhcp_opts":[]}}`), nil
	})
	parent := types.ResourceRef{Service: "network", APIGeneration: "v2", Kind: "network", ID: "n", Scope: types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}, EndpointID: "network-endpoint", EndpointURL: "https://neutron.test"}
	ref := parent
	ref.ID = "p"
	ref.Kind = "port"
	ref.Parent = &parent
	mac, name := "fa:16:3e:12:34:56", "bootfile-name"
	got, e := c.Ports.Update(context.Background(), network.PortUpdateRequest{Ref: ref, Spec: network.PortUpdateSpec{MACAddress: &mac, ExtraDHCPOptions: []network.PortUpdateExtraDHCPOption{{Name: &name, Value: types.Null[string]()}}}}, types.CallOptions{})
	if e != nil || got.Data.ExtraDHCPOptions == nil || len(got.Data.ExtraDHCPOptions) != 0 {
		t.Fatalf("nullable %+v %v", got, e)
	}
}
