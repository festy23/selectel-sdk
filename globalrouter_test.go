package selectelsdk_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/festy23/selectel-sdk/globalrouter"
	"github.com/festy23/selectel-sdk/types"
)

func TestGlobalRouterExactNestedReadUsesAccountIAMAndPreservesSourceIDs(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.selectel.ru" || r.URL.Path != "/naas/v1/routers/router1" || r.URL.Query().Get("nested") != "true" {
			t.Fatal("wrong source endpoint/query")
		}
		return inventoryReply(200, `{"id":"router1","account_id":"test-account","project_id":"origin-project","status":"ACTIVE","enabled":false,"name":"private-name","networks":[{"id":"n1","router_id":"router1","account_id":"test-account","project_id":"origin-project","os_network_id":"original-neutron-network","subnets":[{"id":"s1","network_id":"n1","account_id":"test-account"}]}]}`), nil
	}, 0)
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "global_router", APIGeneration: "v1", Kind: "router", ID: "router1", EndpointID: "selectel-global_router.v1:global", EndpointURL: "https://api.selectel.ru/naas/v1"}
	out, err := c.Account().GlobalRouter().Routers.Get(context.Background(), ref, globalrouter.GetOptions{Nested: managementPtr(true)}, types.CallOptions{})
	if err != nil || out.Data.Ref.ID != "router1" || *out.Data.Enabled || *out.Data.Networks[0].OsNetworkID != "original-neutron-network" || out.Data.Networks[0].Ref.Parent.ID != "router1" || out.Data.Networks[0].Subnets[0].Ref.Parent.ID != "n1" {
		t.Fatal("nested refs/source metadata lost", err)
	}
}

func TestGlobalRouterMutationsKeepReceiptsAndExplicitDeleteChoices(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "POST" {
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["name"] != "synthetic-name" {
				t.Fatal("protected request not sent")
			}
			return inventoryReply(202, `{"id":"new-router","account_id":"test-account","created_at":"invalid"}`), nil
		}
		if r.Method != "DELETE" || r.URL.Query().Get("cascade") != "false" || r.URL.Query().Get("disconnect") != "true" {
			t.Fatal("delete choices changed")
		}
		return nil, io.ErrUnexpectedEOF
	}, 0)
	out, err := c.Account().GlobalRouter().Routers.Create(context.Background(), globalrouter.RouterCreateSpec{Name: managementPtr(types.Protect([]byte("synthetic-name")))}, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "new-router" {
		t.Fatal("accepted malformed identity lost", err)
	}
	ref := out.Receipt.ResourceRefs[0]
	deleted, err := c.Account().GlobalRouter().Routers.Delete(context.Background(), ref, globalrouter.DeleteOptions{Cascade: managementPtr(false), Disconnect: managementPtr(true)}, types.CallOptions{})
	if err == nil || deleted.Receipt.Outcome != types.Unknown || calls != 2 {
		t.Fatal("unknown deletion replayed or receipt lost", err)
	}
}

func TestGlobalRouterPagesPreserveCallerPageAndNoCompletionClaim(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("limit") != "200" || r.URL.Query().Get("sort_direction") != "desc" {
			t.Fatal("query not exact")
		}
		return inventoryReply(200, `[{"id":"r2","account_id":"test-account","enabled":false,"vpn_id":12}]`), nil
	}, 0)
	page, _ := types.ParseInteger("2")
	limit, _ := types.ParseInteger("200")
	out, err := c.Account().GlobalRouter().Routers.ListPage(context.Background(), globalrouter.PageOptions{Page: &page, Limit: &limit, SortDirection: managementPtr("desc")}, types.CallOptions{})
	if err != nil || len(out.Data.Items) != 1 || out.Data.Items[0].Ref.ID != "r2" || *out.Data.Items[0].Enabled {
		t.Fatal("page data lost", err)
	}
}

func TestGlobalRouterAttachmentRetainsOriginalServiceScopeAndReceipts(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["project_id"] != "source-project" || body["os_network_id"] != "neutron-n1" || body["router_id"] != "r1" || body["zone_id"] != "z1" {
			t.Fatal("source attachment identity lost")
		}
		return inventoryReply(202, `{"id":"attached1","account_id":"test-account","router_id":"r1","project_id":"source-project","os_network_id":"neutron-n1","zone_id":"z1","status":"CREATING"}`), nil
	}, 0)
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "global_router", APIGeneration: "v1", Kind: "router", ID: "r1", EndpointID: "selectel-global_router.v1:global", EndpointURL: "https://api.selectel.ru/naas/v1"}
	zone := ref
	zone.Kind = "zone"
	zone.ID = "z1"
	origin := types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "source-project", Region: "spb", Pool: "ru-3"}, Service: "networking", APIGeneration: "v2", Kind: "network", ID: "neutron-n1", EndpointID: "source-endpoint", EndpointURL: "https://ru-3.cloud.api.selcloud.ru/network/v2"}
	out, err := c.Account().GlobalRouter().Networks.AttachVPC(context.Background(), ref, zone, origin, globalrouter.NetworkCreateSpec{}, types.CallOptions{})
	if err != nil || out.Data.Ref.Parent.ID != "r1" || len(out.Receipt.ResourceRefs) != 4 {
		t.Fatal("attachment refs lost", err)
	}
	found := false
	for _, r := range out.Receipt.ResourceRefs {
		if r.Service == "networking" {
			found = r.Scope == origin.Scope && r.EndpointURL == origin.EndpointURL && r.ID == origin.ID
		}
	}
	if !found {
		t.Fatal("origin scope not retained")
	}
}

func TestGlobalRouterPublishedHeadersAndForeignAttachmentsFailBeforeIO(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Accept-Language") != "en" || r.Header.Get("X-Request-ID") != "123e4567-e89b-42d3-a456-426655440000" {
			t.Fatal("documented headers lost")
		}
		return inventoryReply(200, `[]`), nil
	}, 0)
	service, err := c.Account().GlobalRouter().WithHeaders(globalrouter.HeaderOptions{AcceptLanguage: "en", RequestID: "123e4567-e89b-42d3-a456-426655440000"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ServicesPage(context.Background(), globalrouter.PageOptions{}, types.CallOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.WithHeaders(globalrouter.HeaderOptions{RequestID: "not-a-v4-uuid"}); types.ErrorCode(err) != types.InvalidArgument {
		t.Fatal("invalid request ID accepted")
	}
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "global_router", APIGeneration: "v1", Kind: "router", ID: "r1", EndpointID: "selectel-global_router.v1:global", EndpointURL: "https://api.selectel.ru/naas/v1"}
	zone := ref
	zone.Kind = "zone"
	origin := types.ResourceRef{Scope: types.Scope{AccountID: "foreign-account", ProjectID: "p"}, Service: "networking", APIGeneration: "v2", Kind: "network", ID: "n", EndpointID: "network", EndpointURL: "https://example.test/network"}
	out, err := service.Networks.AttachVPC(context.Background(), ref, zone, origin, globalrouter.NetworkCreateSpec{}, types.CallOptions{})
	if types.ErrorCode(err) != types.ScopeMismatch || calls != 1 || out.Receipt.Outcome != types.NotSent {
		t.Fatal("foreign origin reached provider", err)
	}
}

func TestGlobalRouterAttachmentRejectsForeignReturnedSourceBeforeAdoption(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(202, `{"id":"foreign-attachment","router_id":"r1","project_id":"other-project","os_network_id":"other-network","account_id":"test-account"}`), nil
	}, 0)
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "global_router", APIGeneration: "v1", Kind: "router", ID: "r1", EndpointID: "selectel-global_router.v1:global", EndpointURL: "https://api.selectel.ru/naas/v1"}
	zone := ref
	zone.Kind = "zone"
	zone.ID = "z1"
	origin := types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "source-project"}, Service: "networking", APIGeneration: "v2", Kind: "network", ID: "source-network", EndpointID: "network", EndpointURL: "https://example.test/network"}
	out, err := c.Account().GlobalRouter().Networks.AttachVPC(context.Background(), ref, zone, origin, globalrouter.NetworkCreateSpec{}, types.CallOptions{})
	if types.ErrorCode(err) != types.ScopeMismatch || out.Receipt.Outcome != types.Accepted {
		t.Fatal("foreign source association adopted", err)
	}
	for _, r := range out.Receipt.ResourceRefs {
		if r.ID == "foreign-attachment" {
			t.Fatal("foreign returned ref adopted")
		}
	}
}

func TestGlobalRouterRemainingLifecycleAndDirectoryBindings(t *testing.T) {
	var expectedMethod, expectedPath string
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != expectedMethod || r.URL.Path != "/naas/v1"+expectedPath {
			t.Fatal("wrong operation mapping", r.Method, r.URL.Path)
		}
		if r.Method == "PUT" {
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["name"] != "replacement" || len(body["tags"].([]any)) != 0 {
				t.Fatal("replacement lost explicit empty tags")
			}
		}
		if r.Method == "DELETE" {
			return inventoryReply(204, ""), nil
		}
		if r.Method == "GET" && !strings.Contains(expectedPath, "/n1") && !strings.Contains(expectedPath, "/s1") && !strings.Contains(expectedPath, "/route1") {
			return inventoryReply(200, `[{"id":"directory1","name":"directory","limit":900719925474099312345}]`), nil
		}
		id := "r1"
		if strings.HasPrefix(expectedPath, "/networks") {
			id = "n1"
		}
		if strings.HasPrefix(expectedPath, "/subnets") {
			id = "s1"
		}
		if strings.HasPrefix(expectedPath, "/static_routes") {
			id = "route1"
		}
		status := 200
		if r.Method == "POST" {
			status = 202
		}
		return inventoryReply(status, `{"id":"`+id+`","account_id":"test-account","status":"ERROR"}`), nil
	}, 0)
	svc := c.Account().GlobalRouter()
	ref := types.ResourceRef{Scope: c.Account().Scope(), Service: "global_router", APIGeneration: "v1", Kind: "router", ID: "r1", EndpointID: "selectel-global_router.v1:global", EndpointURL: "https://api.selectel.ru/naas/v1"}
	network := ref
	network.Kind = "network"
	network.ID = "n1"
	subnet := ref
	subnet.Kind = "subnet"
	subnet.ID = "s1"
	route := ref
	route.Kind = "static_route"
	route.ID = "route1"
	zone := ref
	zone.Kind = "zone"
	zone.ID = "z1"
	name := managementPtr(types.Protect([]byte("replacement")))
	tags := []string{}
	ctx := context.Background()
	call := types.CallOptions{}
	cases := []struct {
		method, path string
		invoke       func() error
	}{
		{"GET", "/networks/n1", func() error {
			o, e := svc.Networks.Get(ctx, network, globalrouter.GetOptions{}, call)
			if e == nil && *o.Data.Status != "ERROR" {
				t.Fatal("failure state lost")
			}
			return e
		}},
		{"GET", "/subnets/s1", func() error { _, e := svc.Subnets.Get(ctx, subnet, globalrouter.GetOptions{}, call); return e }},
		{"GET", "/static_routes/route1", func() error { _, e := svc.StaticRoutes.Get(ctx, route, call); return e }},
		{"POST", "/networks", func() error {
			_, e := svc.Networks.Create(ctx, ref, zone, globalrouter.NetworkCreateSpec{}, call)
			return e
		}},
		{"POST", "/subnets", func() error {
			_, e := svc.Subnets.Create(ctx, network, globalrouter.SubnetCreateSpec{Cidr: managementPtr("10.0.0.0/24")}, call)
			return e
		}},
		{"POST", "/static_routes", func() error {
			_, e := svc.StaticRoutes.Create(ctx, ref, globalrouter.StaticRouteCreateSpec{Cidr: managementPtr("10.1.0.0/24"), NextHop: managementPtr("10.0.0.1")}, call)
			return e
		}},
		{"PUT", "/routers/r1", func() error {
			_, e := svc.Routers.Update(ctx, ref, globalrouter.RouterUpdateSpec{Name: name, Tags: tags}, call)
			return e
		}},
		{"PUT", "/networks/n1", func() error {
			_, e := svc.Networks.Update(ctx, network, globalrouter.NetworkUpdateSpec{Name: name, Tags: tags}, call)
			return e
		}},
		{"PUT", "/subnets/s1", func() error {
			_, e := svc.Subnets.Update(ctx, subnet, globalrouter.SubnetUpdateSpec{Name: name, Tags: tags}, call)
			return e
		}},
		{"PUT", "/static_routes/route1", func() error {
			_, e := svc.StaticRoutes.Update(ctx, route, globalrouter.StaticRouteUpdateSpec{Name: name, Tags: tags}, call)
			return e
		}},
		{"DELETE", "/networks/n1", func() error {
			_, e := svc.Networks.Delete(ctx, network, globalrouter.DeleteOptions{Disconnect: managementPtr(true)}, call)
			return e
		}},
		{"DELETE", "/subnets/s1", func() error {
			_, e := svc.Subnets.Delete(ctx, subnet, globalrouter.DeleteOptions{Disconnect: managementPtr(true)}, call)
			return e
		}},
		{"DELETE", "/static_routes/route1", func() error { _, e := svc.StaticRoutes.Delete(ctx, route, call); return e }},
		{"GET", "/zones", func() error {
			_, e := svc.ZonesPage(ctx, globalrouter.PageOptions{ShowGroups: managementPtr(true)}, call)
			return e
		}},
		{"GET", "/zone_groups", func() error {
			_, e := svc.ZoneGroupsPage(ctx, globalrouter.PageOptions{ShowZones: managementPtr(true)}, call)
			return e
		}},
		{"GET", "/quotas", func() error {
			o, e := svc.QuotasPage(ctx, globalrouter.PageOptions{}, call)
			if e == nil && o.Data.Items[0].Limit.String() != "900719925474099312345" {
				t.Fatal("quota rounded")
			}
			return e
		}},
	}
	for _, tc := range cases {
		expectedMethod, expectedPath = tc.method, tc.path
		if e := tc.invoke(); e != nil {
			t.Fatal(tc.method, tc.path, e)
		}
	}
}

func TestGlobalRouterRefParentsAndSourceCopiesAreIndependent(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		return inventoryReply(202, `{"id":"attached","account_id":"test-account","router_id":"r1"}`), nil
	}, 0)
	router := types.ResourceRef{Scope: c.Account().Scope(), Service: "global_router", APIGeneration: "v1", Kind: "router", ID: "r1", EndpointID: "selectel-global_router.v1:global", EndpointURL: "https://api.selectel.ru/naas/v1"}
	zone := router
	zone.Kind = "zone"
	zone.ID = "z1"
	originParent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "source-project"}, Service: "networking", APIGeneration: "v2", Kind: "source_container", ID: "original-parent", EndpointID: "source", EndpointURL: "https://example.test/network"}
	origin := originParent
	origin.Kind = "network"
	origin.ID = "source-n"
	origin.Parent = &originParent
	out, err := c.Account().GlobalRouter().Networks.AttachVPC(context.Background(), router, zone, origin, globalrouter.NetworkCreateSpec{}, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	originParent.ID = "changed-by-caller"
	router.ID = "changed-router"
	if out.Data.Ref.Parent.ID != "r1" {
		t.Fatal("data ref aliased input")
	}
	for _, ref := range out.Receipt.ResourceRefs {
		if ref.Service == "networking" && ref.Parent.ID != "original-parent" {
			t.Fatal("receipt aliased source parent")
		}
	}
}
