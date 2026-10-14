package loadbalancers_test

import (
	"context"
	"encoding/json"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/loadbalancers"
	"github.com/festy23/selectel-sdk/pagination"
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
func clients(t *testing.T, wire transport) (*sdk.Client, *loadbalancers.Client) {
	return clientsAt(t, wire, "https://octavia.test/v2")
}
func clientsAt(t *testing.T, wire transport, endpoint string) (*sdk.Client, *loadbalancers.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}
	c, e := sdk.New(sdk.Config{AccountID: "a", PollInterval: time.Millisecond, Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "s", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "loadbalancers", APIGeneration: "v2", Scope: scope}: {ID: "octavia", URL: endpoint, Scope: scope, APIGeneration: "v2", Audience: "selectel"}}, Transport: transport(func(r *http.Request) (*http.Response, error) {
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
	r, e := p.Location("r", "pool")
	if e != nil {
		t.Fatal(e)
	}
	return c, r.LoadBalancers()
}
func TestCreateKeepsProvisioningAndOperatingStatesSeparate(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v2/lbaas/loadbalancers" {
			t.Fatal(r.Method, r.URL)
		}
		var body map[string]map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("body")
		}
		if v, ok := body["loadbalancer"]["admin_state_up"]; !ok || v != false {
			t.Fatal("explicit false lost")
		}
		return response(201, `{"loadbalancer":{"id":"lb","project_id":"different-owner","provisioning_status":"ACTIVE","operating_status":"OFFLINE","vip_address":"10.0.0.7"}}`), nil
	})
	disabled := false
	got, e := c.Create(context.Background(), loadbalancers.LoadBalancerCreate{AdminStateUp: &disabled, VIPSubnetID: ptr("subnet")}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if got.Data.ProvisioningStatus != "ACTIVE" || got.Data.OperatingStatus != "OFFLINE" || got.Data.Ref.Scope.ProjectID != "p" || got.Data.ProjectID != "different-owner" || got.Receipt.Outcome != types.Accepted || got.Receipt.ResourceRefs[0].EndpointURL != "https://octavia.test/v2" {
		t.Fatal("state, auth view or receipt lost")
	}
}

func TestExactReadAndWaitUseProvisioningRatherThanOperating(t *testing.T) {
	reads := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/lbaas/loadbalancers/lb" {
			t.Fatal(r.Method, r.URL)
		}
		reads++
		state := "PENDING_CREATE"
		if reads > 1 {
			state = "ACTIVE"
		}
		return response(200, `{"loadbalancer":{"id":"lb","provisioning_status":"`+state+`","operating_status":"OFFLINE"}}`), nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	got, e := c.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || got.Data.Ref.ID != "lb" {
		t.Fatal(e)
	}
	h, e := root.Resource(ref)
	if e != nil {
		t.Fatal(e)
	}
	waited, e := h.Wait(context.Background(), "loadbalancer.provisioned")
	if e != nil || !waited.Complete || waited.Last.ProviderState != "ACTIVE" {
		t.Fatal(e)
	}
}

func TestAcknowledgedCreateWithoutIDRecoversReceiptWithoutObservation(t *testing.T) {
	calls := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return response(201, `{"loadbalancer":{}}`), nil
	})
	got, e := c.Create(context.Background(), loadbalancers.LoadBalancerCreate{VIPSubnetID: ptr("subnet")}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || got.Receipt == nil || got.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if e != nil || recovered.ProofStatus != "conclusive" || calls != 1 {
		t.Fatal(e, calls)
	}
	if _, e = h.Wait(context.Background(), "loadbalancer.provisioned"); e == nil {
		t.Fatal("no-ID receipt must not promise readiness")
	}
}

func TestUpdateUnknownIsObservedWithoutReplayAndDeleteKeepsCascade(t *testing.T) {
	mutations := 0
	reads := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		switch r.Method {
		case "PUT":
			mutations++
			return nil, io.ErrUnexpectedEOF
		case "GET":
			reads++
			return response(200, `{"loadbalancer":{"id":"lb","provisioning_status":"ACTIVE","operating_status":"ONLINE"}}`), nil
		case "DELETE":
			mutations++
			if r.URL.Query().Get("cascade") != "false" {
				t.Fatal("explicit cascade")
			}
			return response(204, ""), nil
		}
		t.Fatal(r.Method)
		return nil, nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	got, e := c.Update(context.Background(), ref, loadbalancers.LoadBalancerUpdate{}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || got.Receipt.Outcome != types.Unknown {
		t.Fatal(e)
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || recovered.ProofStatus != "indeterminate" || reads != 1 || mutations != 1 {
		t.Fatal(e, reads, mutations)
	}
	no := false
	deleted, e := c.Delete(context.Background(), ref, &no, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || mutations != 2 {
		t.Fatal(e)
	}
}

func TestListenerLifecycleLinksExactLoadBalancerAndRejectsForeignParent(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.HasPrefix(r.URL.Path, "/v2/lbaas/listeners") {
			t.Fatal(r.URL)
		}
		if r.Method == "POST" {
			var b map[string]map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["listener"]["loadbalancer_id"] != "lb" || b["listener"]["protocol_port"] != float64(80) {
				t.Fatal(b)
			}
		}
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		code := 200
		if r.Method == "POST" {
			code = 201
		}
		if r.Method == "PUT" {
			code = 202
		}
		return response(code, `{"listener":{"id":"listener","loadbalancers":[{"id":"lb"}],"protocol":"HTTP","protocol_port":80,"provisioning_status":"ACTIVE","operating_status":"OFFLINE"}}`), nil
	})
	lb := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	port, _ := types.ParseInteger("80")
	created, e := c.Listeners.Create(context.Background(), lb, loadbalancers.ListenerCreate{Protocol: "HTTP", ProtocolPort: port}, types.CallOptions{})
	if e != nil || created.Data.Ref.Parent.ID != "lb" || created.Receipt.ResourceRefs[len(created.Receipt.ResourceRefs)-1].Kind != "listener" {
		t.Fatal(e)
	}
	ref := *created.Data.Ref
	got, e := c.Listeners.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || got.Data.Ref.Parent.ID != "lb" {
		t.Fatal(e)
	}
	updated, e := c.Listeners.Update(context.Background(), ref, loadbalancers.ListenerUpdate{}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	deleted, e := c.Listeners.Delete(context.Background(), ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	ref.Parent.Scope.ProjectID = "other"
	failed, e := c.Listeners.Update(context.Background(), ref, loadbalancers.ListenerUpdate{}, types.CallOptions{})
	if e == nil || failed.Receipt.Outcome != types.NotSent || calls != 4 {
		t.Fatal(e, calls)
	}
}

func TestPoolLifecyclePreservesParentAndNullablePersistence(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		if r.Method == "PUT" {
			var b map[string]map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if v, ok := b["pool"]["session_persistence"]; !ok || v != nil {
				t.Fatal("explicit null lost")
			}
		}
		code := 200
		if r.Method == "POST" {
			code = 201
		}
		return response(code, `{"pool":{"id":"pool-id","loadbalancers":[{"id":"lb"}],"listeners":[{"id":"li"}],"provisioning_status":"ACTIVE","operating_status":"DEGRADED","session_persistence":null,"members":[{"id":"member"}]}}`), nil
	})
	lb := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	created, e := c.Pools.Create(context.Background(), lb, loadbalancers.PoolCreate{Protocol: "HTTP", LbAlgorithm: "ROUND_ROBIN"}, types.CallOptions{})
	if e != nil || created.Data.Ref.Parent.ID != "lb" {
		t.Fatal(e)
	}
	got, e := c.Pools.Get(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil || got.Data.Members[0].ID != "member" {
		t.Fatal(e)
	}
	updated, e := c.Pools.Update(context.Background(), *created.Data.Ref, loadbalancers.PoolUpdate{SessionPersistence: types.Null[loadbalancers.SessionPersistencePUT]()}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	_, e = c.Pools.Delete(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
}

func TestHealthMonitorLifecycleLinksPoolAndPreservesTimingUnits(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		if r.Method == "POST" {
			var b map[string]map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["healthmonitor"]["pool_id"] != "pool-id" || b["healthmonitor"]["delay"] != float64(5) {
				t.Fatal(b)
			}
		}
		code := 200
		if r.Method == "POST" {
			code = 201
		}
		if r.Method == "PUT" {
			code = 202
		}
		return response(code, `{"healthmonitor":{"id":"monitor","pools":[{"id":"pool-id"}],"delay":5,"timeout":3,"max_retries":2,"http_version":1.1,"provisioning_status":"ACTIVE","operating_status":"ONLINE"}}`), nil
	})
	pool := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "pool", ID: "pool-id", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	delay, _ := types.ParseInteger("5")
	timeout, _ := types.ParseInteger("3")
	retries, _ := types.ParseInteger("2")
	created, e := c.Monitors.Create(context.Background(), pool, loadbalancers.MonitorCreate{Type: "HTTP", Delay: delay, Timeout: timeout, MaxRetries: retries}, types.CallOptions{})
	if e != nil || created.Data.Ref.Parent.ID != "pool-id" {
		t.Fatal(e)
	}
	got, e := c.Monitors.Get(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil || got.Data.Delay.String() != "5" || got.Data.HTTPVersion.String() != "1.1" {
		t.Fatal(e)
	}
	_, e = c.Monitors.Update(context.Background(), *created.Data.Ref, loadbalancers.MonitorUpdate{}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Monitors.Delete(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
}

func TestMemberLifecycleUsesPoolAddressAndKeepsExplicitZeroWeight(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if !strings.HasPrefix(r.URL.Path, "/v2/lbaas/pools/pool-id/members") {
			t.Fatal(r.URL)
		}
		if r.Method == "POST" {
			var b map[string]map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["member"]["weight"] != float64(0) {
				t.Fatal(b)
			}
		}
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		code := 200
		if r.Method == "POST" {
			code = 201
		}
		return response(code, `{"member":{"id":"member","address":"10.0.0.1","protocol_port":80,"weight":0,"provisioning_status":"ACTIVE","operating_status":"NO_MONITOR"}}`), nil
	})
	pool := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "pool", ID: "pool-id", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	port, _ := types.ParseInteger("80")
	zero, _ := types.ParseInteger("0")
	created, e := c.Members.Create(context.Background(), pool, loadbalancers.MemberCreate{Address: "10.0.0.1", ProtocolPort: port, Weight: &zero}, types.CallOptions{})
	if e != nil || created.Data.Ref.Parent.ID != "pool-id" {
		t.Fatal(e)
	}
	got, e := c.Members.Get(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil || got.Data.Weight.String() != "0" {
		t.Fatal(e)
	}
	_, e = c.Members.Update(context.Background(), *created.Data.Ref, loadbalancers.MemberUpdate{Weight: &zero}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = c.Members.Delete(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
}

func TestBoundedLoadBalancerPagesPreserveFiltersAndProjectedFields(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		if q.Get("tags-any") != "red,blue" || len(q["fields"]) != 2 || q.Get("limit") != "2" {
			t.Fatal(q)
		}
		if q.Get("marker") == "" {
			return response(200, `{"loadbalancers":[{"id":"a","name":"one"},{"id":"b","name":"two"}],"loadbalancers_links":[{"rel":"next","href":"https://octavia.test/v2/lbaas/loadbalancers?marker=b&limit=2&tags-any=red%2Cblue&fields=id&fields=name"}]}`), nil
		}
		if q.Get("marker") != "b" {
			t.Fatal(q)
		}
		return response(200, `{"loadbalancers":[{"id":"c","name":"three"}],"loadbalancers_links":[]}`), nil
	})
	f := loadbalancers.ListOptions{Filters: map[string][]string{"tags-any": {"red,blue"}}, Fields: []string{"id", "name"}}
	got, e := c.Collect(context.Background(), f, pagination.Options{PageSize: 2, MaxItems: 2})
	if types.ErrorCode(e) != types.Incomplete || got.Complete || len(got.Items) != 2 || got.NextCursor == nil || calls != 1 {
		t.Fatal(e, calls)
	}
	tail, e := c.Collect(context.Background(), f, pagination.Options{PageSize: 2, Cursor: got.NextCursor})
	if e != nil || !tail.Complete || len(tail.Items) != 1 || tail.Items[0].ID != "c" {
		t.Fatal(e)
	}
}

func TestChildCollectionsKeepImmediateParentsAndProjectionIsNotIdentity(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v2/lbaas/listeners":
			return response(200, `{"listeners":[{"id":"li","loadbalancers":[{"id":"lb"}]}]}`), nil
		case "/v2/lbaas/pools":
			return response(200, `{"pools":[{"id":"po","loadbalancers":[{"id":"lb"}]}]}`), nil
		case "/v2/lbaas/healthmonitors":
			return response(200, `{"healthmonitors":[{"name":"projected"}]}`), nil
		case "/v2/lbaas/pools/po/members":
			return response(200, `{"members":[{"id":"m"}]}`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	})
	listeners, e := c.Listeners.Collect(context.Background(), loadbalancers.ListOptions{}, pagination.Options{})
	if e != nil || !listeners.Complete || listeners.Items[0].Ref.Parent.ID != "lb" {
		t.Fatal(e)
	}
	pools, e := c.Pools.Collect(context.Background(), loadbalancers.ListOptions{}, pagination.Options{})
	if e != nil || !pools.Complete || pools.Items[0].Ref.Parent.ID != "lb" {
		t.Fatal(e)
	}
	monitors, e := c.Monitors.Collect(context.Background(), loadbalancers.ListOptions{Fields: []string{"name"}}, pagination.Options{})
	if e != nil || !monitors.Complete || monitors.Items[0].Ref != nil {
		t.Fatal(e)
	}
	members, e := c.Members.Collect(context.Background(), *pools.Items[0].Ref, loadbalancers.ListOptions{}, pagination.Options{})
	if e != nil || !members.Complete || members.Items[0].Ref.Parent.ID != "po" || members.Items[0].Ref.Parent.Parent.ID != "lb" {
		t.Fatal(e)
	}
}

func TestStatusTreeAndStatisticsRetainStatesAndExactCounters(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/stats") {
			return response(200, `{"stats":{"bytes_in":9007199254740993,"bytes_out":0,"active_connections":0,"total_connections":1,"request_errors":0}}`), nil
		}
		return response(200, `{"statuses":{"loadbalancer":{"id":"lb","provisioning_status":"ACTIVE","operating_status":"DEGRADED","listeners":[{"id":"li","pools":[{"id":"po","members":[{"id":"m","provisioning_status":"ACTIVE","operating_status":"ERROR"}],"health_monitor":{"id":"hm","operating_status":"ONLINE"}}]}]}}}`), nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	stats, e := c.Statistics(context.Background(), ref, types.CallOptions{})
	if e != nil || stats.Data.BytesIn.String() != "9007199254740993" {
		t.Fatal(e)
	}
	tree, e := c.StatusTree(context.Background(), ref, types.CallOptions{})
	if e != nil || tree.Data.Loadbalancer.OperatingStatus != "DEGRADED" || tree.Data.Loadbalancer.Listeners[0].Pools[0].Members[0].OperatingStatus != "ERROR" {
		t.Fatal(e)
	}
	ref.Kind = "listener"
	ref.ID = "li"
	ls, e := c.Listeners.Statistics(context.Background(), ref, types.CallOptions{})
	if e != nil || ls.Data.BytesIn.String() != "9007199254740993" {
		t.Fatal(e)
	}
}

func TestFailoverAndMemberReplacementAreAcceptedWithoutInventedCompletion(t *testing.T) {
	mutations := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" {
			t.Fatal("unexpected read/replay", r.Method)
		}
		mutations++
		if strings.HasSuffix(r.URL.Path, "/members") {
			var b map[string][]any
			json.NewDecoder(r.Body).Decode(&b)
			if b["members"] == nil || len(b["members"]) != 0 {
				t.Fatal("empty replacement changed")
			}
		}
		return response(202, ""), nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	got, e := c.Failover(context.Background(), ref, types.CallOptions{})
	if e != nil || got.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.Wait(context.Background(), "loadbalancer.provisioned"); types.ErrorCode(e) != types.CapabilityUnavailable {
		t.Fatal("failover has no causal readiness proof", e)
	}
	ref.Kind = "pool"
	ref.ID = "po"
	batch, e := c.Members.Replace(context.Background(), ref, []loadbalancers.MemberCreate{}, types.CallOptions{})
	if e != nil || batch.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	h, e = root.Resume(*batch.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if e != nil || recovered.ProofStatus != "conclusive" || mutations != 2 {
		t.Fatal(e, mutations)
	}
	if _, e = h.Wait(context.Background(), "pool.provisioned"); types.ErrorCode(e) != types.CapabilityUnavailable {
		t.Fatal("pool active cannot prove member replacement", e)
	}
}

func TestEndpointRootAndPublishedVersionAliasesPreserveExactBinding(t *testing.T) {
	for _, pair := range [][2]string{{"https://octavia.test/proxy", "/proxy/v2/lbaas/loadbalancers/lb"}, {"https://octavia.test/proxy/v2", "/proxy/v2/lbaas/loadbalancers/lb"}, {"https://octavia.test/proxy/v2.0", "/proxy/v2.0/lbaas/loadbalancers/lb"}} {
		t.Run(pair[0], func(t *testing.T) {
			_, c := clientsAt(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != pair[1] {
					t.Fatal(r.URL)
				}
				return response(200, `{"loadbalancer":{"id":"lb"}}`), nil
			}, pair[0])
			ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: pair[0]}
			got, e := c.Get(context.Background(), ref, types.CallOptions{})
			if e != nil || got.Data.Ref.EndpointURL != pair[0] {
				t.Fatal(e)
			}
		})
	}
}

func TestFullyPopulatedCreatePreservesNestedCoreResourcesAndParentChain(t *testing.T) {
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		return response(201, `{"loadbalancer":{"id":"lb","listeners":[{"id":"li","protocol":"HTTP","protocol_port":80}],"pools":[{"id":"po","protocol":"HTTP","members":[{"id":"m","address":"10.0.0.1","protocol_port":80}],"healthmonitor":{"id":"hm","type":"HTTP","delay":5}}]}}`), nil
	})
	got, e := c.Create(context.Background(), loadbalancers.LoadBalancerCreate{VIPSubnetID: ptr("subnet")}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if got.Data.Listeners[0].Protocol != "HTTP" || got.Data.Listeners[0].Ref.Parent.ID != "lb" || got.Data.Pools[0].Members[0].Ref.Parent.Parent.ID != "lb" || got.Data.Pools[0].Healthmonitor.Delay.String() != "5" {
		t.Fatal("inline response discarded")
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if e != nil || recovered.ProofStatus != "conclusive" {
		t.Fatal(e)
	}
}

func TestAdditiveMemberBatchPreservesProviderQuery(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("additive_only") != "true" {
			t.Fatal(r.URL)
		}
		return response(202, ""), nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "pool", ID: "po", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	yes := true
	got, e := c.Members.BatchUpdate(context.Background(), ref, []loadbalancers.MemberCreate{}, &yes, types.CallOptions{})
	if e != nil || got.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
}

func TestProjectedChildCannotInjectRefAndConfirmedParentFilterMapsExactly(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("loadbalancer_id") != "lb" || r.URL.Query().Get("not-tags-any") != "x,y" {
			t.Fatal(r.URL)
		}
		return response(200, `{"listeners":[{"name":"partial","ref":{"id":"foreign"}}]}`), nil
	})
	got, e := c.Listeners.Collect(context.Background(), loadbalancers.ListOptions{Fields: []string{"name"}, Filters: map[string][]string{"loadbalancer_id": {"lb"}, "not-tags-any": {"x,y"}}}, pagination.Options{})
	if e != nil || !got.Complete || got.Items[0].Ref != nil {
		t.Fatal("provider supplied SDK ref", e)
	}
}

func TestInvalidMandatoryInputsRetainNotSentReceiptWithoutIO(t *testing.T) {
	calls := 0
	root, c := clients(t, func(*http.Request) (*http.Response, error) { calls++; return response(500, ""), nil })
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	got, e := c.Listeners.Create(context.Background(), ref, loadbalancers.ListenerCreate{}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || got.Receipt == nil || got.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatal(e, calls)
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if e != nil || recovered.ProofStatus != "conclusive" || calls != 0 {
		t.Fatal(e, calls)
	}
	empty, e := c.Create(context.Background(), loadbalancers.LoadBalancerCreate{}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || empty.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatal(e, calls)
	}
}

func ptr[T any](v T) *T { return &v }

func TestAdditionalVIPOptionalAddressAndNullableMemberMonitorFields(t *testing.T) {
	step := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		step++
		var b map[string]map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&b)
		if step == 1 {
			if string(b["loadbalancer"]["additional_vips"]) != `[{"subnet_id":"extra"}]` {
				t.Fatal(string(b["loadbalancer"]["additional_vips"]))
			}
			return response(201, `{"loadbalancer":{"id":"lb"}}`), nil
		}
		if string(b["member"]["monitor_port"]) != "null" || string(b["member"]["monitor_address"]) != "null" {
			t.Fatal("null lost")
		}
		return response(200, `{"member":{"id":"m","monitor_port":null,"monitor_address":null}}`), nil
	})
	_, e := c.Create(context.Background(), loadbalancers.LoadBalancerCreate{VIPSubnetID: ptr("subnet"), AdditionalVips: &[]loadbalancers.AdditionalVipsType{{SubnetID: "extra"}}}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	pool := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "pool", ID: "po", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	ref := pool
	ref.Kind = "member"
	ref.ID = "m"
	ref.Parent = &pool
	_, e = c.Members.Update(context.Background(), ref, loadbalancers.MemberUpdate{MonitorPort: types.Null[types.Integer](), MonitorAddress: types.Null[string]()}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
}

func TestChildReceiptResumeSelectsChildAndUnknownCreateHasNoTargetProof(t *testing.T) {
	step := 0
	gets := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			gets++
			if r.URL.Path != "/v2/lbaas/listeners/li" {
				t.Fatal(r.URL)
			}
			return response(200, `{"listener":{"id":"li","loadbalancers":[{"id":"lb"}],"provisioning_status":"ACTIVE"}}`), nil
		}
		step++
		if step == 2 {
			return nil, io.ErrUnexpectedEOF
		}
		return response(201, `{"listener":{"id":"li","loadbalancers":[{"id":"lb"}],"provisioning_status":"PENDING_CREATE"}}`), nil
	})
	lb := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	port, _ := types.ParseInteger("80")
	got, e := c.Listeners.Create(context.Background(), lb, loadbalancers.ListenerCreate{Protocol: "HTTP", ProtocolPort: port}, types.CallOptions{})
	if e != nil || len(got.Receipt.ResourceRefs) != 2 {
		t.Fatal(e)
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	waited, e := h.Wait(context.Background(), "listener.provisioned")
	if e != nil || !waited.Complete || gets != 1 {
		t.Fatal(e)
	}
	uncertain, e := c.Listeners.Create(context.Background(), lb, loadbalancers.ListenerCreate{Protocol: "HTTP", ProtocolPort: port}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome {
		t.Fatal(e)
	}
	if _, e = root.Resume(*uncertain.Receipt); types.ErrorCode(e) != types.RecoveryUnavailable || gets != 1 || step != 2 {
		t.Fatal("parent is not child identity", e)
	}
}

func TestWaitTerminalUnknownDeadlineAndScopedAbsence(t *testing.T) {
	for _, row := range []struct {
		state  string
		status int
		goal   string
		want   types.Code
	}{{"ERROR", 200, "loadbalancer.provisioned", types.OperationFailed}, {"FUTURE", 200, "loadbalancer.provisioned", types.StateUnknown}, {"PENDING_UPDATE", 200, "loadbalancer.provisioned", types.DeadlineExceeded}, {"", 404, "loadbalancer.absent", ""}} {
		t.Run(row.state+row.goal, func(t *testing.T) {
			root, _ := clients(t, func(*http.Request) (*http.Response, error) {
				if row.status == 404 {
					return response(404, `{"faultstring":"do-not-disclose-provider-message"}`), nil
				}
				return response(200, `{"loadbalancer":{"id":"lb","provisioning_status":"`+row.state+`","operating_status":"ONLINE"}}`), nil
			})
			ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
			h, e := root.Resource(ref)
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			got, e := h.Wait(ctx, row.goal)
			if types.ErrorCode(e) != row.want || got.Complete != (row.want == "") {
				t.Fatal(e)
			}
		})
	}
}

func TestUnaddressableParentAndBlankVIPFailBeforeIO(t *testing.T) {
	calls := 0
	root, c := clients(t, func(*http.Request) (*http.Response, error) {
		calls++
		return response(200, `{"loadbalancer":{"id":"lb"}}`), nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	p := ref
	p.ID = "fake-parent"
	ref.Parent = &p
	if _, e := root.Resource(ref); types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal(e)
	}
	got, e := c.Create(context.Background(), loadbalancers.LoadBalancerCreate{VIPSubnetID: ptr("")}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || got.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatal(e, calls)
	}
}

func TestSourceErrorStatusesStaySanitizedAndRejectedReceiptsRemain(t *testing.T) {
	for _, row := range []struct {
		status int
		want   types.Code
	}{{400, types.ProviderError}, {403, types.PermissionDenied}, {404, types.NotFound}, {409, types.Conflict}, {429, types.UnknownOutcome}} {
		t.Run(string(row.want), func(t *testing.T) {
			_, c := clients(t, func(*http.Request) (*http.Response, error) {
				return response(row.status, `{"faultcode":"Client","faultstring":"sensitive-provider-marker","debuginfo":"sensitive-provider-marker"}`), nil
			})
			ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
			got, e := c.Delete(context.Background(), ref, nil, types.CallOptions{})
			outcome := types.Rejected
			if row.status == 429 {
				outcome = types.Unknown
			}
			if types.ErrorCode(e) != row.want || got.Receipt == nil || got.Receipt.Outcome != outcome {
				t.Fatal(e)
			}
			b, _ := json.Marshal(e)
			if strings.Contains(string(b), "sensitive-provider-marker") {
				t.Fatal("provider text leaked")
			}
		})
	}
}

func TestCollectionFailureRetainsPrefixAndRejectsForeignNextEndpoint(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("marker") == "" {
			return response(200, `{"pools":[{"id":"po","loadbalancers":[{"id":"lb"}]}],"pools_links":[{"rel":"next","href":"https://octavia.test/v2/lbaas/pools?marker=po&limit=100"}]}`), nil
		}
		return response(200, `{"pools":[{"id":"po2","loadbalancers":[{"id":"lb"}]}],"pools_links":[{"rel":"next","href":"https://foreign.test/v2/lbaas/pools?marker=po2"}]}`), nil
	})
	got, e := c.Pools.Collect(context.Background(), loadbalancers.ListOptions{}, pagination.Options{PageSize: 100})
	if types.ErrorCode(e) != types.ProtocolError || got.Complete || len(got.Items) != 1 || got.Items[0].ID != "po" {
		t.Fatal(e, len(got.Items))
	}
}

func TestMissingStatusTreeRootIsStructuredProtocolError(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) { return response(200, `{"statuses":{}}`), nil })
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
	_, e := c.StatusTree(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError {
		t.Fatal(e)
	}
}

func TestExactPathsRejectChangedOverrideBeforeTargetIO(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}
	lb := types.ResourceRef{Scope: scope, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://old.test/v2"}
	pool := lb
	pool.Kind = "pool"
	pool.ID = "po"
	pool.Parent = &lb
	listener := lb
	listener.Kind = "listener"
	listener.ID = "li"
	listener.Parent = &lb
	member := pool
	member.Kind = "member"
	member.ID = "m"
	member.Parent = &pool
	port, _ := types.ParseInteger("80")
	calls := []struct {
		name string
		run  func(*sdk.Client, *loadbalancers.Client) error
	}{
		{"get", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Get(context.Background(), lb, types.CallOptions{})
			return e
		}},
		{"delete", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Delete(context.Background(), lb, nil, types.CallOptions{})
			return e
		}},
		{"update", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Update(context.Background(), lb, loadbalancers.LoadBalancerUpdate{}, types.CallOptions{})
			return e
		}},
		{"failover", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Failover(context.Background(), lb, types.CallOptions{})
			return e
		}},
		{"statistics", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Statistics(context.Background(), lb, types.CallOptions{})
			return e
		}},
		{"statuses", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.StatusTree(context.Background(), lb, types.CallOptions{})
			return e
		}},
		{"child-create", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Listeners.Create(context.Background(), lb, loadbalancers.ListenerCreate{Protocol: "HTTP", ProtocolPort: port}, types.CallOptions{})
			return e
		}},
		{"child-get", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Listeners.Get(context.Background(), listener, types.CallOptions{})
			return e
		}},
		{"child-update", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Listeners.Update(context.Background(), listener, loadbalancers.ListenerUpdate{}, types.CallOptions{})
			return e
		}},
		{"child-delete", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Members.Delete(context.Background(), member, types.CallOptions{})
			return e
		}},
		{"batch", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Members.Replace(context.Background(), pool, []loadbalancers.MemberCreate{}, types.CallOptions{})
			return e
		}},
		{"parent-list", func(_ *sdk.Client, c *loadbalancers.Client) error {
			_, e := c.Members.Collect(context.Background(), pool, loadbalancers.ListOptions{}, pagination.Options{})
			return e
		}},
		{"resource-status", func(root *sdk.Client, _ *loadbalancers.Client) error {
			h, e := root.Resource(lb)
			if e != nil {
				return e
			}
			_, e = h.Status(context.Background())
			return e
		}},
		{"resume-status", func(root *sdk.Client, _ *loadbalancers.Client) error {
			cap, e := root.Catalog().Describe("loadbalancers.v2.loadbalancers.update")
			if e != nil {
				return e
			}
			r := types.Receipt{SchemaVersion: 1, CallID: "call", Scope: scope, OperationID: cap.OperationID, ContractRevision: cap.ContractRevision, Outcome: types.Accepted, Recovery: "resource", ResourceRefs: []types.ResourceRef{lb}, Endpoint: &types.EndpointBinding{ID: lb.EndpointID, URL: lb.EndpointURL, Scope: scope, APIGeneration: "v2", Audience: "selectel"}}
			h, e := root.Resume(r)
			if e != nil {
				return e
			}
			_, e = h.Status(context.Background())
			return e
		}},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			targets := 0
			root, c := clientsAt(t, func(*http.Request) (*http.Response, error) { targets++; return response(200, `{}`), nil }, "https://new.test/v2")
			e := call.run(root, c)
			if types.ErrorCode(e) != types.EndpointUnavailable || targets != 0 {
				t.Fatal(e, targets)
			}
		})
	}
}

func TestChangedAuthenticatedCatalogCannotRetargetSavedReference(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}
	targets := 0
	root, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "s", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(200, `{"token":{"expires_at":"2099-01-01T00:00:00Z","project":{"id":"p","domain":{"name":"a"}},"catalog":[{"id":"svc","type":"load-balancer","endpoints":[{"id":"octavia","interface":"public","region_id":"pool","url":"https://changed.test/v2"}]}]}}`), nil
		}
		targets++
		return response(200, `{}`), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	ref := types.ResourceRef{Scope: scope, Service: "loadbalancers", APIGeneration: "v2", Kind: "loadbalancer", ID: "lb", EndpointID: "octavia", EndpointURL: "https://original.test/v2"}
	h, e := root.Resource(ref)
	if e != nil {
		t.Fatal(e)
	}
	_, e = h.Status(context.Background())
	if types.ErrorCode(e) != types.EndpointUnavailable || targets != 0 {
		t.Fatal(e, targets)
	}
}

func TestAcceptedMalformedLoadBalancerKeepsTrustworthyIdentity(t *testing.T) {
	root, c := clients(t, func(*http.Request) (*http.Response, error) {
		return response(201, `{"loadbalancer":{"id":"lb","admin_state_up":"invalid"}}`), nil
	})
	got, e := c.Create(context.Background(), loadbalancers.LoadBalancerCreate{VIPSubnetID: ptr("subnet")}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].ID != "lb" || got.Data.Ref != nil {
		t.Fatal(e, len(got.Receipt.ResourceRefs))
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if e != nil || recovered.ProofStatus != "conclusive" {
		t.Fatal(e)
	}
}

func TestAcceptedMalformedMonitorKeepsValidatedChildAndParent(t *testing.T) {
	for _, row := range []struct {
		body      string
		wantChild bool
	}{
		{`{"healthmonitor":{"id":"hm","pools":[{"id":"po"}],"delay":"bad"}}`, true},
		{`{"healthmonitor":{"id":"hm","pools":[{"id":"foreign"}],"delay":"bad"}}`, false},
		{`{"healthmonitor":{"id":123,"pools":[{"id":"po"}],"delay":"bad"}}`, false},
	} {
		t.Run(row.body, func(t *testing.T) {
			_, c := clients(t, func(*http.Request) (*http.Response, error) { return response(201, row.body), nil })
			parent := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}, Service: "loadbalancers", APIGeneration: "v2", Kind: "pool", ID: "po", EndpointID: "octavia", EndpointURL: "https://octavia.test/v2"}
			delay, _ := types.ParseInteger("5")
			timeout, _ := types.ParseInteger("3")
			retries, _ := types.ParseInteger("2")
			got, e := c.Monitors.Create(context.Background(), parent, loadbalancers.MonitorCreate{Type: "HTTP", Delay: delay, Timeout: timeout, MaxRetries: retries}, types.CallOptions{})
			if types.ErrorCode(e) != types.ProtocolError || got.Receipt.Outcome != types.Accepted || got.Data.Ref != nil {
				t.Fatal(e)
			}
			if len(got.Receipt.ResourceRefs) == 0 || got.Receipt.ResourceRefs[0].Kind != "pool" || got.Receipt.ResourceRefs[0].ID != "po" || got.Receipt.ResourceRefs[0].EndpointURL != parent.EndpointURL {
				t.Fatal("known parent lost")
			}
			found := false
			for _, r := range got.Receipt.ResourceRefs {
				if r.Kind == "healthmonitor" {
					found = true
					if r.ID != "hm" || r.Parent == nil || r.Parent.ID != "po" {
						t.Fatal("wrong identity")
					}
				}
			}
			if found != row.wantChild {
				t.Fatal("trusted child lost/adopted", found)
			}
		})
	}
}

func TestRootIdentitySurvivesMalformedUnrelatedRelationships(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		return response(201, `{"loadbalancer":{"id":"lb","pools":"bad"}}`), nil
	})
	got, e := c.Create(context.Background(), loadbalancers.LoadBalancerCreate{VIPSubnetID: ptr("subnet")}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != 1 || got.Receipt.ResourceRefs[0].ID != "lb" {
		t.Fatal(e, len(got.Receipt.ResourceRefs))
	}
}
