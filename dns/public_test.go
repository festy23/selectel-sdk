package dns_test

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/dns"
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
func clients(t *testing.T, wire transport) (*sdk.Client, *dns.Client) {
	return clientsAt(t, wire, "https://dns.test/v2")
}
func clientsAt(t *testing.T, wire transport, endpoint string) (*sdk.Client, *dns.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	c, e := sdk.New(sdk.Config{AccountID: "a", PollInterval: time.Millisecond, Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "s", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "dns", APIGeneration: "v2", Scope: scope}: {ID: "dns", URL: endpoint, Scope: scope, APIGeneration: "v2", Audience: "selectel"}}, Transport: transport(func(r *http.Request) (*http.Response, error) {
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
	return c, p.DNS()
}
func TestCreateZonePreservesOwnerMetadataAndOptionalProtection(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v2/zones" {
			t.Fatal(r.Method, r.URL)
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		if len(b) != 1 || b["name"] != "example.test." {
			t.Fatal(b)
		}
		return response(200, `{"id":"zone","name":"example.test.","project_id":"other-owner","protected":false,"disabled":false,"created_at":"2026-10-01T12:13:14"}`), nil
	})
	got, e := c.Zones.Create(context.Background(), dns.ZoneCreate{Name: "example.test."}, types.CallOptions{})
	if e != nil || got.Receipt.Outcome != types.Accepted || got.Data.Ref.ID != "zone" || got.Data.ProjectID != "other-owner" || got.Data.Ref.Scope.ProjectID != "p" || got.Data.Protected == nil || *got.Data.Protected || *got.Data.CreatedAt != "2026-10-01T12:13:14" {
		t.Fatal(e)
	}
}

func TestCreateRRSetPreservesTTLRecordsAndProtectsContent(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v2/zones/zone/rrset" {
			t.Fatal(r.Method, r.URL)
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		rows := b["records"].([]any)
		if b["ttl"] != float64(60) || rows[0].(map[string]any)["content"] != "sensitive-marker" || rows[0].(map[string]any)["disabled"] != false || b["comment"] != nil {
			t.Fatal("wire mapping")
		}
		return response(200, `{"id":"rr","zone_id":"zone","name":"example.test.","type":"TXT","ttl":9007199254740993,"records":[{"content":"sensitive-marker","disabled":false}],"managed_by":null,"comment":null}`), nil
	})
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
	ttl, _ := types.ParseInteger("60")
	disabled := false
	got, e := c.RRSets.Create(context.Background(), parent, dns.RRSetCreate{Name: "example.test.", TTL: ttl, Type: dns.AllowedRecordTypesTXT, Records: []dns.Records{{Content: types.Protect([]byte("sensitive-marker")), Disabled: &disabled}}, Comment: types.Null[string]()}, types.CallOptions{})
	if e != nil || got.Receipt.Outcome != types.Accepted || got.Data.Ref.Parent.ID != "zone" || got.Data.TTL.String() != "9007199254740993" || string(got.Data.Records[0].Content.Reveal()) != "sensitive-marker" {
		t.Fatal(e)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), "sensitive-marker") || strings.Contains(fmt.Sprintf("%+v %#v", got, got), "sensitive-marker") {
		t.Fatal("record content leaked")
	}
}

func TestZoneLifecycleActionsUseDistinctPathsAndPreserveFalse(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "GET" {
			return response(200, `{"id":"zone","name":"example.test.","project_id":"p","protected":true,"disabled":false}`), nil
		}
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		switch r.URL.Path {
		case "/v2/zones/zone":
			if b["comment"] != "" {
				t.Fatal(b)
			}
		case "/v2/zones/zone/state":
			if b["disabled"] != false {
				t.Fatal(b)
			}
		case "/v2/zones/zone/protection":
			if b["protected"] != false {
				t.Fatal(b)
			}
		case "/v2/zones/zone/owner":
			if r.Method != "PUT" || b["project_id"] != "00000000-0000-4000-8000-000000000001" {
				t.Fatal(b)
			}
		default:
			t.Fatal(r.URL)
		}
		return response(204, ""), nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
	got, e := c.Zones.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || !*got.Data.Protected {
		t.Fatal(e)
	}
	for _, call := range []func() (types.Result[dns.Empty], error){func() (types.Result[dns.Empty], error) {
		return c.Zones.UpdateComment(context.Background(), ref, "", types.CallOptions{})
	}, func() (types.Result[dns.Empty], error) {
		return c.Zones.SetDisabled(context.Background(), ref, false, types.CallOptions{})
	}, func() (types.Result[dns.Empty], error) {
		return c.Zones.SetProtected(context.Background(), ref, false, types.CallOptions{})
	}, func() (types.Result[dns.Empty], error) {
		return c.Zones.TransferOwner(context.Background(), ref, "00000000-0000-4000-8000-000000000001", types.CallOptions{})
	}, func() (types.Result[dns.Empty], error) {
		return c.Zones.Delete(context.Background(), ref, types.CallOptions{})
	}} {
		r, e := call()
		if e != nil || r.Receipt.Outcome != types.Accepted || r.Receipt.ResourceRefs[0].Scope.ProjectID != "p" {
			t.Fatal(e)
		}
	}
	if calls != 6 {
		t.Fatal(calls)
	}
}

func TestRRSetUpdateReplacesValuesAndMapsManagedByHeader(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v2/zones/zone/rrset/rr" {
			t.Fatal(r.URL)
		}
		if r.Method == "GET" {
			return response(200, `{"id":"rr","zone_id":"zone","name":"example.test.","type":"TXT","ttl":60,"records":[{"content":"old"}]}`), nil
		}
		if r.Header.Get("X-Managed-By") != "00000000-0000-4000-8000-000000000001" {
			t.Fatal("managed-by header")
		}
		if r.Method == "PATCH" {
			var b map[string]any
			json.NewDecoder(r.Body).Decode(&b)
			if len(b) != 3 || b["ttl"] != float64(120) || b["comment"] != nil || len(b["records"].([]any)) != 1 || b["records"].([]any)[0].(map[string]any)["content"] != "replacement" {
				t.Fatal("replacement wire")
			}
		}
		return response(204, ""), nil
	})
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
	ref := parent
	ref.ID = "rr"
	ref.Kind = "rrset"
	ref.Parent = &parent
	got, e := c.RRSets.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || string(got.Data.Records[0].Content.Reveal()) != "old" {
		t.Fatal(e)
	}
	ttl, _ := types.ParseInteger("120")
	managed := "00000000-0000-4000-8000-000000000001"
	options := dns.RRSetOptions{ManagedBy: &managed}
	updated, e := c.RRSets.Update(context.Background(), ref, dns.RRSetUpdate{TTL: ttl, Records: []dns.Records{{Content: types.Protect([]byte("replacement"))}}, Comment: types.Null[string]()}, options)
	if e != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	deleted, e := c.RRSets.Delete(context.Background(), ref, options)
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 3 {
		t.Fatal(e, calls)
	}
}

func TestBoundedZonePaginationUsesProviderOffsetsAndNullableTerminal(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		if q.Get("filter") != "example" || q.Get("sort_by") != "name.descend" || q.Get("limit") != "1" {
			t.Fatal(q)
		}
		if q.Get("offset") == "0" {
			return response(200, `{"count":9007199254740993,"next_offset":7,"result":[{"id":"one","name":"one.example.","project_id":"p"}]}`), nil
		}
		if q.Get("offset") != "7" {
			t.Fatal(q)
		}
		return response(200, `{"count":9007199254740993,"next_offset":null,"result":[{"id":"two","name":"two.example.","project_id":"p"}]}`), nil
	})
	filter := "example"
	f := dns.ZoneListOptions{Filter: &filter, SortBy: []dns.ZonesSortingOptions{dns.ZonesSortingOptionsNameDescend}}
	first, e := c.Zones.Collect(context.Background(), f, pagination.Options{PageSize: 1, MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || first.Complete || first.NextCursor == nil || calls != 1 || first.ProviderPages[0].ReportedCount.String() != "9007199254740993" {
		t.Fatal(e, calls)
	}
	rest, e := c.Zones.Collect(context.Background(), f, pagination.Options{PageSize: 1, Cursor: first.NextCursor})
	if e != nil || !rest.Complete || rest.Items[0].ID != "two" || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestRRSetPaginationPreservesRepeatedFiltersAndPrivateSearchCursor(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		if len(q["rrset_types"]) != 2 || q.Get("search") != "sensitive-search" || q.Get("sort_by") != "type.ascend" {
			t.Fatal("query")
		}
		return response(200, `{"count":2,"next_offset":1,"result":[{"id":"rr","zone_id":"zone","name":"example.test.","ttl":60,"type":"TXT","records":[{"content":"sensitive-content"}]}]}`), nil
	})
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
	search := types.Protect([]byte("sensitive-search"))
	f := dns.RRSetListOptions{Search: &search, RRSetTypes: []string{"TXT", "A"}, SortBy: []dns.RRSetSortingOptions{dns.RRSetSortingOptionsTypeAscend}}
	got, e := c.RRSets.Collect(context.Background(), parent, f, pagination.Options{PageSize: 1, MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || got.Complete || got.NextCursor == nil || got.Items[0].Ref.Parent.ID != "zone" {
		t.Fatal(e)
	}
	b, _ := json.Marshal(got.NextCursor)
	if strings.Contains(string(b), "sensitive-search") {
		t.Fatal("cursor disclosed search")
	}
	other := types.Protect([]byte("other-search"))
	f.Search = &other
	_, e = c.RRSets.Collect(context.Background(), parent, f, pagination.Options{Cursor: got.NextCursor})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatal(e, calls)
	}
}

func TestNativeSDKManagedByWireVariantIsExplicit(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		if b["managed_by"] != "00000000-0000-4000-8000-000000000001" || r.Header.Get("X-Managed-By") != "" {
			t.Fatal("native wire variant")
		}
		if r.Method == "POST" {
			return response(200, `{"id":"rr","zone_id":"zone","name":"example.test.","type":"TXT","ttl":60,"records":[]}`), nil
		}
		return response(204, ""), nil
	})
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
	ttl, _ := types.ParseInteger("60")
	managed := "00000000-0000-4000-8000-000000000001"
	rows := []dns.Records{{Content: types.Protect([]byte("value"))}}
	got, e := c.RRSets.CreateNative(context.Background(), parent, dns.RRSetCreate{Name: "example.test.", Type: dns.AllowedRecordTypesTXT, TTL: ttl, Records: rows}, managed, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	updated, e := c.RRSets.UpdateNative(context.Background(), *got.Data.Ref, dns.RRSetUpdate{TTL: ttl, Records: rows}, managed, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestUnknownMutationRecoveryObservesExactIDWithoutReplay(t *testing.T) {
	mutations := 0
	reads := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			mutations++
			return nil, io.ErrUnexpectedEOF
		}
		reads++
		if r.Method != "GET" || r.URL.Path != "/v2/zones/zone" {
			t.Fatal(r.Method, r.URL)
		}
		return response(200, `{"id":"zone","name":"example.test.","project_id":"p"}`), nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
	got, e := c.Zones.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || got.Receipt.Outcome != types.Unknown {
		t.Fatal(e)
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovery, e := h.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || recovery.ProofStatus != "indeterminate" || mutations != 1 || reads != 1 {
		t.Fatal(e, mutations, reads)
	}
	h, e = root.Resource(ref)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.Status(context.Background()); e != nil || reads != 2 {
		t.Fatal(e, reads)
	}
}

func TestInvalidDNSInputsFailLocallyWithRecoverableNotSentReceipt(t *testing.T) {
	calls := 0
	root, c := clients(t, func(*http.Request) (*http.Response, error) { calls++; return response(500, ""), nil })
	bad, e := c.Zones.Create(context.Background(), dns.ZoneCreate{Name: "missing-dot"}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || bad.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatal(e, calls)
	}
	h, e := root.Resume(*bad.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if e != nil || recovered.ProofStatus != "conclusive" || calls != 0 {
		t.Fatal(e, calls)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
	low, _ := types.ParseInteger("59")
	valid, _ := types.ParseInteger("60")
	for _, spec := range []dns.RRSetCreate{{Name: "example.test.", TTL: low, Type: dns.AllowedRecordTypesA, Records: []dns.Records{{Content: types.Protect([]byte("10.0.0.1"))}}}, {Name: "example.test.", TTL: valid, Type: dns.AllowedRecordTypesA, Records: []dns.Records{}}} {
		got, e := c.RRSets.Create(context.Background(), parent, spec, types.CallOptions{})
		if types.ErrorCode(e) != types.InvalidArgument || got.Receipt.Outcome != types.NotSent || calls != 0 {
			t.Fatal(e, calls)
		}
	}
}

func TestAcceptedMalformedIdentityPreservesOnlyVerifiedRefsAndKnownReceipt(t *testing.T) {
	for _, body := range []string{`{"id":"zone","name":"example.test.","project_id":"p","disabled":"bad"}`, `{"id":123,"name":"example.test.","project_id":"p"}`} {
		t.Run(body, func(t *testing.T) {
			root, c := clients(t, func(*http.Request) (*http.Response, error) { return response(200, body), nil })
			got, e := c.Zones.Create(context.Background(), dns.ZoneCreate{Name: "example.test."}, types.CallOptions{})
			if types.ErrorCode(e) != types.ProtocolError || got.Receipt.Outcome != types.Accepted || got.Data.Ref != nil {
				t.Fatal(e)
			}
			want := 1
			if strings.Contains(body, `"id":123`) {
				want = 0
			}
			if len(got.Receipt.ResourceRefs) != want {
				t.Fatal("identity lost/adopted")
			}
			h, e := root.Resume(*got.Receipt)
			if e != nil {
				t.Fatal(e)
			}
			recovered, e := h.Recover(context.Background())
			if e != nil || recovered.ProofStatus != "conclusive" {
				t.Fatal(e)
			}
		})
	}
}

func TestExactReferencesRejectEndpointOrParentChangesBeforeTargetIO(t *testing.T) {
	targets := 0
	_, c := clientsAt(t, func(*http.Request) (*http.Response, error) { targets++; return response(200, `{}`), nil }, "https://changed.test/v2")
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
	_, e := c.Zones.Get(context.Background(), parent, types.CallOptions{})
	if types.ErrorCode(e) != types.EndpointUnavailable || targets != 0 {
		t.Fatal(e, targets)
	}
	_, e = c.RRSets.Collect(context.Background(), parent, dns.RRSetListOptions{}, pagination.Options{})
	if types.ErrorCode(e) != types.EndpointUnavailable || targets != 0 {
		t.Fatal(e, targets)
	}
	ref := parent
	ref.Kind = "rrset"
	ref.ID = "rr"
	ref.Parent = &parent
	ref.Parent.Scope.ProjectID = "foreign"
	got, e := c.RRSets.Delete(context.Background(), ref, dns.RRSetOptions{})
	if e == nil || got.Receipt.Outcome != types.NotSent || targets != 0 {
		t.Fatal(e, targets)
	}
}

func TestRRSetMalformedPayloadKeepsIdentityButRejectsForeignZone(t *testing.T) {
	for _, zone := range []string{"zone", "foreign"} {
		t.Run(zone, func(t *testing.T) {
			root, c := clients(t, func(*http.Request) (*http.Response, error) {
				return response(200, `{"id":"rr","zone_id":"`+zone+`","name":"example.test.","type":"TXT","ttl":"bad","records":[]}`), nil
			})
			parent := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
			ttl, _ := types.ParseInteger("60")
			got, e := c.RRSets.Create(context.Background(), parent, dns.RRSetCreate{Name: "example.test.", TTL: ttl, Type: dns.AllowedRecordTypesTXT, Records: []dns.Records{{Content: types.Protect([]byte("v"))}}}, types.CallOptions{})
			want := 1
			if zone == "zone" {
				want = 2
			}
			if types.ErrorCode(e) != types.ProtocolError || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != want {
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
		})
	}
}

func TestDNSWaitsDescribeScopedExistenceAndNeverPropagation(t *testing.T) {
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		return response(404, `{"error":"sensitive-provider-marker","description":"secret"}`), nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
	got, e := c.Zones.Delete(context.Background(), ref, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	h, e := root.Resume(*got.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	waited, e := h.Wait(context.Background(), "zone.absent")
	if e != nil || !waited.Complete || waited.Last.Classification != "absent" {
		t.Fatal(e)
	}
	if _, e = h.Wait(context.Background(), "dns.propagated"); types.ErrorCode(e) != types.CapabilityUnavailable {
		t.Fatal(e)
	}
}

func TestDNSCurrentAuthHasNoLegacyStaticTokenFallback(t *testing.T) {
	targets := 0
	_, c := clients(t, func(*http.Request) (*http.Response, error) { targets++; return response(200, `{}`), nil })
	got, e := c.Zones.Create(context.Background(), dns.ZoneCreate{Name: "example.test."}, types.CallOptions{AuthMode: types.StaticToken})
	if types.ErrorCode(e) != types.InvalidArgument || got.Receipt.Outcome != types.NotSent || targets != 0 {
		t.Fatal(e, targets)
	}
}

func TestErrorMappingDropsProviderTextAndCollectionFailureRetainsPrefix(t *testing.T) {
	for _, status := range []int{400, 403, 404, 409, 422} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			_, c := clients(t, func(*http.Request) (*http.Response, error) {
				return response(status, `{"error":"sensitive-marker","description":"sensitive-marker","location":"sensitive-marker"}`), nil
			})
			ref := types.ResourceRef{Scope: types.Scope{AccountID: "a", ProjectID: "p"}, Service: "dns", APIGeneration: "v2", Kind: "zone", ID: "zone", EndpointID: "dns", EndpointURL: "https://dns.test/v2"}
			got, e := c.Zones.Delete(context.Background(), ref, types.CallOptions{})
			want := map[int]types.Code{400: types.InvalidArgument, 403: types.PermissionDenied, 404: types.NotFound, 409: types.Conflict, 422: types.InvalidArgument}[status]
			if types.ErrorCode(e) != want || got.Receipt.Outcome != types.Rejected {
				t.Fatal(e)
			}
			b, _ := json.Marshal(e)
			if strings.Contains(string(b), "sensitive-marker") {
				t.Fatal("provider text leaked")
			}
		})
	}
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("offset") == "0" {
			return response(200, `{"count":2,"next_offset":1,"result":[{"id":"zone","name":"example.test.","project_id":"p"}]}`), nil
		}
		return response(200, `{"count":2,"next_offset":1,"result":[]}`), nil
	})
	got, e := c.Zones.Collect(context.Background(), dns.ZoneListOptions{}, pagination.Options{PageSize: 1})
	if types.ErrorCode(e) != types.ProtocolError || got.Complete || len(got.Items) != 1 {
		t.Fatal(e, len(got.Items))
	}
}

func TestProtectedRecordsDoNotDiscloseEncodedBytes(t *testing.T) {
	marker := []byte("private-record-content")
	row := dns.Records{Content: types.Protect(marker)}
	for _, v := range []any{row, &row, row.Content, &row.Content, []dns.Records{row}} {
		s := fmt.Sprintf("%+v %#v", v, v)
		if strings.Contains(s, string(marker)) || strings.Contains(s, fmt.Sprint(marker)) || strings.Contains(s, fmt.Sprintf("%#v", marker)) {
			t.Fatal("encoded disclosure")
		}
	}
}
