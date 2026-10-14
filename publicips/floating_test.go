package publicips_test

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/publicips"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"testing"
)

func neutronRef(kind, id string) types.ResourceRef {
	return types.ResourceRef{Service: "network", APIGeneration: "v2", Kind: kind, ID: id, Scope: types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}, EndpointID: "neutron-endpoint", EndpointURL: "https://neutron.test"}
}
func TestNeutronFloatingIPCreateAndExplicitAssociation(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "neutron.test" {
			t.Fatal("wrong API", r.URL)
		}
		var body map[string]map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&body)
		v := body["floatingip"]
		if r.Method == "POST" && string(v["floating_network_id"]) != `"external"` {
			t.Fatal("external parent")
		}
		if r.Method == "PUT" {
			if string(v["port_id"]) != `"internal-port"` || string(v["fixed_ip_address"]) != `"10.0.0.7"` || r.Header.Get("If-Match") != "revision_number=0" {
				t.Fatalf("association %s", v)
			}
		}
		status := 200
		if r.Method == "POST" {
			status = 201
		}
		return response(status, `{"floatingip":{"id":"f","floating_network_id":"external","project_id":"project","port_id":"internal-port","fixed_ip_address":"10.0.0.7","created_at":"2026-10-14T12:00:00.000000"}}`), nil
	})
	external := neutronRef("network", "external")
	port := neutronRef("port", "internal-port")
	parent := neutronRef("network", "private")
	port.Parent = &parent
	made, e := c.FloatingIPs.Create(context.Background(), publicips.FloatingIPCreateRequest{Network: external}, types.CallOptions{})
	if e != nil || made.Data.Ref.Kind != "floating_ip" || made.Data.Ref.Parent.ID != "external" || *made.Data.CreatedAt != "2026-10-14T12:00:00.000000" {
		t.Fatalf("neutron create %+v %v", made, e)
	}
	zero, _ := types.ParseInteger("0")
	bound, e := c.FloatingIPs.Update(context.Background(), publicips.FloatingIPUpdateRequest{Ref: *made.Data.Ref, Port: types.Value(port), Spec: publicips.FloatingIPUpdateSpec{FixedIPAddress: types.Value("10.0.0.7")}, Revision: &zero}, types.CallOptions{})
	if e != nil || bound.Receipt.Outcome != types.Accepted || calls != 2 {
		t.Fatalf("association %+v %v", bound, e)
	}
}

func TestNeutronListProjectionGetDisassociateAndDelete(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "GET" && r.URL.Path == "/v2.0/floatingips" {
			if r.URL.Query().Get("tags-any") != "one,two" {
				t.Fatal("tag filter")
			}
			return response(200, `{"floatingips":[{"id":"f","floating_network_id":"external","project_id":"other-owner"}],"floatingips_links":[]}`), nil
		}
		if r.Method == "GET" {
			if r.URL.RawQuery != "fields=status" {
				t.Fatal(r.URL)
			}
			return response(200, `{"floatingip":{"status":"DOWN"}}`), nil
		}
		if r.Method == "PUT" {
			var body map[string]map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&body)
			if string(body["floatingip"]["port_id"]) != "null" || string(body["floatingip"]["fixed_ip_address"]) != "null" {
				t.Fatal("disassociate null")
			}
			return response(200, `{"floatingip":{"id":"f","floating_network_id":"external","project_id":"other-owner","port_id":null,"fixed_ip_address":null}}`), nil
		}
		return response(204, ""), nil
	})
	list, e := c.FloatingIPs.Collect(context.Background(), publicips.FloatingIPListOptions{Filters: map[publicips.FloatingIPFilter][]string{publicips.FloatingIPFilterTagsAny: {"one,two"}}}, pagination.Options{})
	if e != nil || !list.Complete || len(list.Items) != 1 || list.Items[0].Ref.Scope.ProjectID != "project" {
		t.Fatalf("read owner view %+v %v", list, e)
	}
	ref := *list.Items[0].Ref
	got, e := c.FloatingIPs.Get(context.Background(), ref, publicips.FloatingIPGetOptions{Fields: []publicips.FloatingIPField{publicips.FloatingIPFieldStatus}}, types.CallOptions{})
	if e != nil || got.Data.Ref.ID != "f" || got.Data.ID != nil {
		t.Fatalf("projection %+v %v", got, e)
	}
	gone, e := c.FloatingIPs.Update(context.Background(), publicips.FloatingIPUpdateRequest{Ref: ref, Port: types.Null[types.ResourceRef](), Spec: publicips.FloatingIPUpdateSpec{FixedIPAddress: types.Null[string]()}}, types.CallOptions{})
	if e != nil || gone.Receipt.Outcome != types.Accepted {
		t.Fatalf("detach %+v %v", gone, e)
	}
	deleted, e := c.FloatingIPs.Delete(context.Background(), ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 4 {
		t.Fatalf("delete %+v %v", deleted, e)
	}
}

func TestNeutronUnknownDeleteAndAssociationRefGuards(t *testing.T) {
	calls := 0
	c := client(t, func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })
	parent := neutronRef("network", "external")
	ref := neutronRef("floating_ip", "f")
	ref.Parent = &parent
	gone, e := c.FloatingIPs.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || gone.Receipt.Outcome != types.Unknown || calls != 1 {
		t.Fatalf("delete replay %+v %v", gone, e)
	}
	foreign := neutronRef("port", "p")
	foreign.Parent = &parent
	foreign.Scope.ProjectID = "foreign"
	rejected, e := c.FloatingIPs.Update(context.Background(), publicips.FloatingIPUpdateRequest{Ref: ref, Port: types.Value(foreign)}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || rejected.Receipt.Outcome != types.NotSent || calls != 1 {
		t.Fatalf("foreign association %+v %v", rejected, e)
	}
	direct := foreign
	direct.Scope = ref.Scope
	direct.Service = "public_network"
	direct.APIGeneration = "v1"
	direct.Kind = "direct_public_port"
	_, e = c.FloatingIPs.Update(context.Background(), publicips.FloatingIPUpdateRequest{Ref: ref, Port: types.Value(direct)}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatalf("direct kind crossuse %v", e)
	}
}
func TestNeutronCursorPreservesFiltersAndRejectsForeignLinks(t *testing.T) {
	calls := 0
	c := client(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("description") != "" || !r.URL.Query().Has("description") {
			t.Fatal("empty filter omitted")
		}
		if r.URL.Query().Get("marker") == "" {
			return response(200, `{"floatingips":[{"id":"f1","floating_network_id":"external"}],"floatingips_links":[{"rel":"next","href":"?marker=f1"}]}`), nil
		}
		return response(200, `{"floatingips":[],"floatingips_links":[{"rel":"next","href":"https://foreign.test/v2.0/floatingips?marker=secret"}]}`), nil
	})
	filter := publicips.FloatingIPListOptions{Filters: map[publicips.FloatingIPFilter][]string{publicips.FloatingIPFilterDescription: {""}}}
	got, e := c.FloatingIPs.Collect(context.Background(), filter, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || got.Complete || got.NextCursor == nil || len(got.Items) != 1 {
		t.Fatalf("bounded %+v %v", got, e)
	}
	resumed, e := c.FloatingIPs.Collect(context.Background(), filter, pagination.Options{Cursor: got.NextCursor})
	if types.ErrorCode(e) != types.ProtocolError || resumed.Complete || calls != 2 {
		t.Fatalf("foreign link %+v %v %d", resumed, e, calls)
	}
	filter.Filters[publicips.FloatingIPFilterDescription] = []string{"changed"}
	_, e = c.FloatingIPs.Iterate(filter, pagination.Options{Cursor: got.NextCursor})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 2 {
		t.Fatal("cursor selection changed", e)
	}
}
