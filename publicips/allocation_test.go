package publicips_test

import (
	"context"
	"encoding/json"
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

func allocationClient(t *testing.T, wire transport) *sdk.Client {
	source := credentials.SourceFunc(func(_ context.Context, req types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Scheme: credentials.IAMToken, Binding: req.Scope, Audience: "selectel", Token: types.Protect([]byte("synthetic-" + req.Scope.ProjectID)), ExpiresAt: time.Now().Add(time.Hour)}, nil
	})
	overrides := map[types.EndpointKey]types.EndpointBinding{}
	for _, project := range []string{"", "project"} {
		scope := types.Scope{AccountID: "account", ProjectID: project}
		overrides[types.EndpointKey{Service: "projects", APIGeneration: "v2", Scope: scope}] = types.EndpointBinding{ID: "selectel-resell", URL: "https://resell.test", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
	}
	c, e := sdk.New(sdk.Config{AccountID: "account", Credentials: source, EndpointOverrides: overrides, Transport: transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			body := `{"domain":{"name":"account"}}`
			if r.Header.Get("X-Subject-Token") == "synthetic-project" {
				body = `{"project":{"id":"project","domain":{"name":"account"}}}`
			}
			var token map[string]any
			json.Unmarshal([]byte(body), &token)
			token["expires_at"] = "2099-01-01T00:00:00Z"
			raw, _ := json.Marshal(map[string]any{"token": token})
			return response(200, string(raw)), nil
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestResellAllocationPreservesBulkRequestAndSeparateIdentity(t *testing.T) {
	c := allocationClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/floatingips/projects/project" || r.Method != "POST" || r.Header.Get("X-Auth-Token") != "synthetic-" {
			t.Fatal(r.Method, r.URL)
		}
		var body map[string][]map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&body)
		if len(body["floatingips"]) != 1 || string(body["floatingips"][0]["quantity"]) != "2" || string(body["floatingips"][0]["region"]) != `"ru-3"` {
			t.Fatalf("bulk %s", body)
		}
		return response(200, `{"floatingips":[{"id":"f1","project_id":"project","region":"ru-3"},{"id":"f2","project_id":"project","region":"ru-3"}]}`), nil
	})
	quantity, _ := types.ParseInteger("2")
	pool := "ru-3"
	got, e := c.Account().PublicIPAllocations().Create(context.Background(), publicips.AllocationCreateRequest{ProjectID: "project", Specs: []publicips.AllocationSpec{{Region: &pool, Quantity: &quantity}}}, types.CallOptions{})
	if e != nil || len(got.Data) != 2 || got.Receipt.Outcome != types.Accepted || got.Data[0].Ref.Kind != "floating_ip_allocation" || got.Data[0].Ref.Service != "projects" || got.Data[0].Ref.Scope.ProjectID != "" || got.Data[0].Ref.Parent.ID != "project" {
		t.Fatalf("allocation %+v %v", got, e)
	}
}

func TestResellAccountListExactGetReleaseAndExplicitProjectAuth(t *testing.T) {
	calls := 0
	c := allocationClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "GET" && r.URL.Path == "/v2/floatingips" {
			if r.URL.RawQuery != "detailed=false" || r.Header.Get("X-Auth-Token") != "synthetic-" {
				t.Fatal(r.URL)
			}
			return response(200, `{"floatingips":[{"id":"f1","project_id":"project","region":"ru-3"}]}`), nil
		}
		if r.Method == "DELETE" {
			return response(204, ""), nil
		}
		if r.Method == "POST" {
			if r.Header.Get("X-Auth-Token") != "synthetic-project" {
				t.Fatal("project auth not used")
			}
			return response(200, `{"floatingip":{"id":"f2","project_id":"project","region":"ru-3"}}`), nil
		}
		return response(200, `{"floatingip":{"id":"f1","project_id":"project","region":"ru-3","port_id":null,"fixed_ip_address":null}}`), nil
	})
	off := false
	all, e := c.Account().PublicIPAllocations().Collect(context.Background(), publicips.AllocationListOptions{Detailed: &off}, pagination.Options{})
	if e != nil || len(all.Items) != 1 || !all.Complete {
		t.Fatalf("account list %+v %v", all, e)
	}
	ref := *all.Items[0].Ref
	one, e := c.Account().PublicIPAllocations().Get(context.Background(), ref, types.CallOptions{})
	if e != nil || one.Data.PortID == nil {
		t.Fatalf("get %+v %v", one, e)
	}
	gone, e := c.Account().PublicIPAllocations().Delete(context.Background(), ref, types.CallOptions{})
	if e != nil || gone.Receipt.Outcome != types.Accepted {
		t.Fatalf("release %+v %v", gone, e)
	}
	project, e := c.Project("project")
	if e != nil {
		t.Fatal(e)
	}
	qty, _ := types.ParseInteger("1")
	pool := "ru-3"
	made, e := project.PublicIPAllocations().Create(context.Background(), publicips.AllocationCreateRequest{ProjectID: "project", Specs: []publicips.AllocationSpec{{Quantity: &qty, Region: &pool}}}, types.CallOptions{})
	if e != nil || made.Data[0].Ref.Scope.ProjectID != "project" || calls != 4 {
		t.Fatalf("project allocate %+v %v", made, e)
	}
}

func TestResellUnknownReleaseNeverReplaysAndAccountListCannotUseProjectView(t *testing.T) {
	calls := 0
	c := allocationClient(t, func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })
	ref := types.ResourceRef{Service: "projects", APIGeneration: "v2", Kind: "floating_ip_allocation", ID: "f", Scope: types.Scope{AccountID: "account"}, EndpointID: "selectel-resell", EndpointURL: "https://resell.test"}
	parent := ref
	parent.ID = "project"
	parent.Kind = "project"
	ref.Parent = &parent
	got, e := c.Account().PublicIPAllocations().Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || got.Receipt.Outcome != types.Unknown || calls != 1 {
		t.Fatalf("release replay %+v %v", got, e)
	}
	project, _ := c.Project("project")
	_, e = project.PublicIPAllocations().ListPage(context.Background(), publicips.AllocationListOptions{}, pagination.Options{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatalf("project account list %v %d", e, calls)
	}
}
func TestResellAcceptedIncompleteAllocationRetainsEveryKnownID(t *testing.T) {
	c := allocationClient(t, func(*http.Request) (*http.Response, error) {
		return response(200, `{"floatingip":{"id":"f1","project_id":"project","region":"ru-3"}}`), nil
	})
	qty, _ := types.ParseInteger("2")
	pool := "ru-3"
	got, e := c.Account().PublicIPAllocations().Create(context.Background(), publicips.AllocationCreateRequest{ProjectID: "project", Specs: []publicips.AllocationSpec{{Quantity: &qty, Region: &pool}}}, types.CallOptions{})
	if types.ErrorCode(e) != types.Incomplete || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != 1 || len(got.Data) != 1 || got.Data[0].Ref.ID != "f1" {
		t.Fatalf("incomplete allocation %+v %v", got, e)
	}
}

func TestResellDuplicateIDsCannotSatisfyRequestedCardinality(t *testing.T) {
	c := allocationClient(t, func(*http.Request) (*http.Response, error) {
		return response(200, `{"floatingips":[{"id":"f","project_id":"project","region":"ru-3"},{"id":"f","project_id":"project","region":"ru-3"}]}`), nil
	})
	qty, _ := types.ParseInteger("2")
	pool := "ru-3"
	got, e := c.Account().PublicIPAllocations().Create(context.Background(), publicips.AllocationCreateRequest{ProjectID: "project", Specs: []publicips.AllocationSpec{{Quantity: &qty, Region: &pool}}}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || got.Receipt.Outcome != types.Accepted || len(got.Receipt.ResourceRefs) != 1 {
		t.Fatalf("duplicate success %+v %v", got, e)
	}
}

func TestResellExplicitNativeCreateUsesRegionOnlyWithoutFallback(t *testing.T) {
	calls := 0
	c := allocationClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"floatingips":[{"region":"ru-3"}]}` {
			t.Fatalf("native allocation wire %s", raw)
		}
		return response(400, `{"message":"native-not-enabled"}`), nil
	})
	pool := "ru-3"
	got, e := c.Account().PublicIPAllocations().CreateNative(context.Background(), publicips.AllocationNativeCreateRequest{ProjectID: "project", Spec: publicips.AllocationNativeSpec{Region: &pool}}, types.CallOptions{})
	if e == nil || got.Receipt.Outcome != types.Rejected || calls != 1 {
		t.Fatalf("native fallback %+v %v %d", got, e, calls)
	}
}

func TestResellNativeCreateRetainsReturnedIDsWithoutInventingQuantity(t *testing.T) {
	c := allocationClient(t, func(*http.Request) (*http.Response, error) {
		return response(200, `{"floatingips":[{"id":"f1","project_id":"project","region":"ru-3"},{"id":"f2","project_id":"project","region":"ru-3"}]}`), nil
	})
	pool := "ru-3"
	got, e := c.Account().PublicIPAllocations().CreateNative(context.Background(), publicips.AllocationNativeCreateRequest{ProjectID: "project", Spec: publicips.AllocationNativeSpec{Region: &pool}}, types.CallOptions{})
	if e != nil || len(got.Data) != 2 || len(got.Receipt.ResourceRefs) != 2 || got.Receipt.Outcome != types.Accepted {
		t.Fatalf("native default cardinality %+v %v", got, e)
	}
}
