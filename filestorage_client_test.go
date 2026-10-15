package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/filestorage"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFileStorageTypesUsePinnedMicroversionExactPoolAndPreserveFalse(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method != "GET" || r.URL.String() != "https://ru-9.cloud.api.selcloud.ru/share/v2/types" || r.Header.Get("X-OpenStack-Manila-API-Version") != "2.73" {
			t.Fatalf("types %s %s %s", r.Method, r.URL, r.Header.Get("X-OpenStack-Manila-API-Version"))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_types":[{"id":"type-1","name":"synthetic-type","description":"example","share_type_access:is_public":false,"is_default":false,"extra_specs":{"driver_handles_share_servers":"True"},"required_extra_specs":{"driver_handles_share_servers":"True"}}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	out, e := view.FileStorage().ListTypes(context.Background(), filestorage.TypeFilters{}, pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.ID != "type-1" || out.Items[0].Ref.Scope != view.Scope() || out.Items[0].Ref.Parent != nil || out.Items[0].ShareTypeAccessIsPublic == nil || *out.Items[0].ShareTypeAccessIsPublic || out.Items[0].IsDefault == nil || *out.Items[0].IsDefault {
		t.Fatalf("types %+v %v", out, e)
	}
	if out.Items[0].Ref.APIGeneration != "v2_73" || out.Items[0].Ref.Kind != "share_type" || out.Items[0].Ref.Service != "filestorage" {
		t.Fatal("wrong exact identity")
	}
}

func TestFileStorageTypeDescriptionPreservesNullButRejectsMissing(t *testing.T) {
	for _, description := range []string{`,"description":null`, ""} {
		t.Run(description, func(t *testing.T) {
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/identity/v3/auth/tokens" {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_types":[{"id":"type-1","name":"synthetic-type","share_type_access:is_public":false,"is_default":false,"extra_specs":{},"required_extra_specs":{}` + description + `}]}`))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			p, _ := c.Project("test-project")
			view, _ := p.Location("spb", "ru-9")
			out, e := view.FileStorage().ListTypes(context.Background(), filestorage.TypeFilters{}, pagination.Options{})
			if description == "" {
				if types.ErrorCode(e) != types.ProtocolError {
					t.Fatalf("missing %+v %v", out, e)
				}
				return
			}
			if e != nil || len(out.Items) != 1 || out.Items[0].Description == nil {
				t.Fatalf("null %+v %v", out, e)
			}
		})
	}
}

func TestFileStorageTypeFiltersAndCutoffKeepProtectedNullableDiagnostics(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Query().Get("is_public") != "false" || r.URL.Query().Get("extra_specs") != `{"driver_handles_share_servers":"True"}` {
			t.Fatalf("query %s", r.URL.RawQuery)
		}
		item := `{"id":"type-1","name":"synthetic-type","description":"synthetic-private-type-description","share_type_access:is_public":false,"is_default":false,"extra_specs":{"synthetic":"private-spec"},"required_extra_specs":{}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_types":[` + item + `,` + strings.Replace(item, "type-1", "type-2", 1) + `]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	public := false
	out, e := view.FileStorage().ListTypes(context.Background(), filestorage.TypeFilters{IsPublic: &public, ExtraSpecs: map[string]string{"driver_handles_share_servers": "True"}}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.NextCursor == nil {
		t.Fatalf("cutoff %+v %v", out, e)
	}
	description, valid := out.Items[0].Description.Value()
	if !valid || string(description.Reveal()) != "synthetic-private-type-description" {
		t.Fatal("description not preserved")
	}
	safe, _ := json.Marshal(out)
	diagnostics := string(safe) + fmt.Sprintf("%+v %#v %x", out.Items[0].Description, out.Items[0].Description, out.Items[0].Description)
	for _, secret := range []string{"synthetic-private-type-description", "private-spec"} {
		for _, encoding := range []string{secret, fmt.Sprint([]byte(secret)), fmt.Sprintf("%#v", []byte(secret)), fmt.Sprintf("%x", []byte(secret))} {
			if strings.Contains(diagnostics, encoding) {
				t.Fatal("protected nullable or specs leaked")
			}
		}
	}
}

func TestFileStorageNetworkListContinuesAfterServerClampedShortPage(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		offset := r.URL.Query().Get("offset")
		if r.URL.Path != "/share/v2/share-networks/detail" || r.URL.Query().Get("limit") != "3" {
			t.Fatalf("network %s", r.URL)
		}
		payload := `{"share_networks":[]}`
		if offset == "0" || offset == "1" {
			payload = `{"share_networks":[{"id":"network-` + offset + `","project_id":"test-project","name":null,"description":null,"created_at":"2022-01-01T00:00:00","updated_at":null,"status":"active","security_service_update_support":false,"network_allocation_update_support":false,"share_network_subnets":[]}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	out, e := view.FileStorage().ListNetworks(context.Background(), filestorage.NetworkFilters{}, pagination.Options{PageSize: 3})
	if e != nil || !out.Complete || calls != 3 || len(out.Items) != 2 || out.Items[0].Ref == nil || out.Items[0].Ref.ID != "network-0" || out.Items[0].Ref.Parent != nil || out.Items[1].Ref.Scope != view.Scope() {
		t.Fatalf("networks %+v %v calls%d", out, e, calls)
	}
}

func TestFileStorageNetworkPaginationRetainsEarlierPageButExcludesForeignProject(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		project := "test-project"
		if calls > 1 {
			project = "foreign-project"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_networks":[{"id":"network-1","project_id":"` + project + `","name":null,"description":null,"created_at":"2022-01-01T00:00:00","updated_at":null,"status":"active","security_service_update_support":false,"network_allocation_update_support":false,"share_network_subnets":[]}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	out, e := view.FileStorage().ListNetworks(context.Background(), filestorage.NetworkFilters{}, pagination.Options{PageSize: 1})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.Scope.ProjectID != "test-project" || calls != 2 {
		t.Fatalf("partial %+v %v", out, e)
	}
}

func TestFileStorageNetworkCreateRejectsForeignOriginBeforeCredentialsAndRetainsAcceptedID(t *testing.T) {
	credentialsCalls, wireCalls := 0, 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, request types.AuthRequest) (credentials.Material, error) {
		credentialsCalls++
		return source().Lookup(ctx, request)
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		wireCalls++
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/share/v2/share-networks" || !strings.Contains(string(body), `"description":"synthetic-private-description"`) || !strings.Contains(string(body), `"name":null`) || !strings.Contains(string(body), `"neutron_net_id":"neutron-network"`) {
			t.Fatalf("create %s %s", r.URL, body)
		}
		return &http.Response{StatusCode: 202, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_network":{"id":"created-network","project_id":"test-project","status":false}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	network := types.ResourceRef{Scope: view.Scope(), Service: "network", APIGeneration: "v2", Kind: "network", ID: "neutron-network", EndpointID: "synthetic-neutron", EndpointURL: "https://ru-9.cloud.api.selcloud.ru/network"}
	subnet := network
	subnet.Kind = "subnet"
	subnet.ID = "neutron-subnet"
	subnet.Parent = &network
	var input filestorage.NetworkCreateRequest
	_ = json.Unmarshal([]byte(`{"name":null,"description":"synthetic-private-description"}`), &input)
	foreign := network
	foreign.Service = "unrelated"
	out, e := view.FileStorage().CreateNetwork(context.Background(), foreign, subnet, input, types.CallOptions{})
	if e == nil || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || credentialsCalls != 0 || wireCalls != 0 {
		t.Fatalf("gate %+v %v creds%d wire%d", out, e, credentialsCalls, wireCalls)
	}
	out, e = view.FileStorage().CreateNetwork(context.Background(), network, subnet, input, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 3 || out.Receipt.ResourceRefs[0].ID != "created-network" || out.Receipt.ResourceRefs[0].Parent != nil {
		t.Fatalf("accepted %+v %v", out, e)
	}
}

func TestFileStorageCreateNetworkAcceptsSource200AndRejectsContradictoryOriginEcho(t *testing.T) {
	for _, returnedNet := range []string{"neutron-network", "foreign-network"} {
		t.Run(returnedNet, func(t *testing.T) {
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/identity/v3/auth/tokens" {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_network":{"id":"created-network","project_id":"test-project","name":null,"description":null,"created_at":"2022-01-01T00:00:00","updated_at":null,"status":"active","security_service_update_support":false,"network_allocation_update_support":false,"share_network_subnets":[{"id":"share-subnet-1","created_at":"2022-01-01T00:00:00","availability_zone":null,"updated_at":null,"neutron_net_id":"` + returnedNet + `","neutron_subnet_id":"neutron-subnet","cidr":null,"gateway":null,"ip_version":null,"network_type":null,"mtu":null,"segmentation_id":null}]}}`))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			p, _ := c.Project("test-project")
			view, _ := p.Location("spb", "ru-9")
			network := types.ResourceRef{Scope: view.Scope(), Service: "network", APIGeneration: "v2", Kind: "network", ID: "neutron-network", EndpointID: "synthetic-neutron", EndpointURL: "https://ru-9.cloud.api.selcloud.ru/network"}
			subnet := network
			subnet.Kind = "subnet"
			subnet.ID = "neutron-subnet"
			subnet.Parent = &network
			out, e := view.FileStorage().CreateNetwork(context.Background(), network, subnet, filestorage.NetworkCreateRequest{}, types.CallOptions{})
			if out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 4 || out.Receipt.ResourceRefs[0].ID != "created-network" {
				t.Fatalf("receipt %+v %v", out, e)
			}
			if returnedNet == "foreign-network" {
				if types.ErrorCode(e) != types.ScopeMismatch {
					t.Fatalf("foreign %+v %v", out, e)
				}
			} else if e != nil || out.Data.Ref == nil || out.Data.Ref.Parent != nil {
				t.Fatalf("created %+v %v", out, e)
			}
		})
	}
}

func TestFileStorageNetworkCreateUnknownKeepsOnlyCallerAnchorsAndNeverReplays(t *testing.T) {
	writes := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		writes++
		_, _ = io.Copy(io.Discard, r.Body)
		return nil, io.ErrUnexpectedEOF
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("", "ru-9")
	network := types.ResourceRef{Scope: view.Scope(), Service: "network", APIGeneration: "v2", Kind: "network", ID: "neutron-network", EndpointID: "synthetic-neutron", EndpointURL: "https://ru-9.cloud.api.selcloud.ru/network"}
	subnet := network
	subnet.Kind = "subnet"
	subnet.ID = "neutron-subnet"
	subnet.Parent = &network
	out, e := view.FileStorage().CreateNetwork(context.Background(), network, subnet, filestorage.NetworkCreateRequest{}, types.CallOptions{})
	if e == nil || writes != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || out.Receipt.Recovery != "unavailable" || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.ResourceRefs[0].Service != "network" {
		t.Fatalf("unknown %+v %v writes%d", out, e, writes)
	}
	out.Receipt.ResourceRefs[1].Parent.ID = "caller-copy-changed"
	if subnet.Parent.ID != "neutron-network" {
		t.Fatal("receipt aliases caller parent")
	}
}

func TestFileStorageNetworkCreateRetainsAllReturnedDefaultSubnetIDsBeforeDecode(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return &http.Response{StatusCode: 202, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_network":{"id":"created-network","project_id":"test-project","status":false,"share_network_subnets":[{"id":false},{"id":"created-share-subnet"}]}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	network := types.ResourceRef{Scope: view.Scope(), Service: "network", APIGeneration: "v2", Kind: "network", ID: "neutron-network", EndpointID: "synthetic-neutron", EndpointURL: "https://ru-9.cloud.api.selcloud.ru/network"}
	subnet := network
	subnet.Kind = "subnet"
	subnet.ID = "neutron-subnet"
	subnet.Parent = &network
	out, e := view.FileStorage().CreateNetwork(context.Background(), network, subnet, filestorage.NetworkCreateRequest{}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 4 {
		t.Fatalf("effects %+v %v", out, e)
	}
	child := out.Receipt.ResourceRefs[1]
	if child.Service != "filestorage" || child.Kind != "share_network_subnet" || child.ID != "created-share-subnet" || child.Parent == nil || child.Parent.ID != "created-network" {
		t.Fatalf("child %+v", child)
	}
}

func fileStorageShareJSON(id, project string) string {
	return strings.NewReplacer("@ID@", id, "@PROJECT@", project).Replace(`{"id":"@ID@","project_id":"@PROJECT@","size":50,"created_at":"2022-01-01T00:00:00","status":"future-provider-state","share_proto":"NFS","name":null,"description":null,"availability_zone":null,"snapshot_id":null,"share_network_id":null,"share_type":null,"volume_type":null,"share_type_name":null,"task_state":null,"access_rules_status":null,"replication_type":null,"user_id":null,"share_group_id":null,"source_share_group_snapshot_member_id":null,"progress":null,"scheduled_to_be_deleted_at":null,"metadata":{"synthetic":"private-share-metadata"},"links":[],"is_public":false,"has_replicas":false,"snapshot_support":false,"create_share_from_snapshot_support":false,"revert_to_snapshot_support":false,"mount_snapshot_support":false,"is_soft_deleted":false}`)
}

func TestFileStorageSharesKeepUnknownStateExactCountAndEmptyPageEndRule(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if r.URL.Path != "/share/v2/shares/detail" || r.URL.Query().Get("with_count") != "true" || r.URL.Query().Get("metadata") != `{"owner":"synthetic-private-filter"}` {
			t.Fatalf("shares %s", r.URL)
		}
		body := `{"shares":[],"count":9007199254740993}`
		if r.URL.Query().Get("offset") == "0" {
			body = `{"shares":[` + fileStorageShareJSON("share-1", "test-project") + `],"count":9007199254740993}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	count := true
	var metadata types.ProtectedJSON
	_ = json.Unmarshal([]byte(`{"owner":"synthetic-private-filter"}`), &metadata)
	out, e := view.FileStorage().ListShares(context.Background(), filestorage.ShareFilters{WithCount: &count, Metadata: &metadata}, pagination.Options{PageSize: 2})
	if e != nil || !out.Complete || calls != 2 || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.ID != "share-1" || out.Items[0].Ref.Parent != nil || out.Items[0].Status == nil || *out.Items[0].Status != "future-provider-state" || len(out.ProviderPages) != 2 || out.ProviderPages[0].TotalCount == nil || out.ProviderPages[0].TotalCount.String() != "9007199254740993" {
		t.Fatalf("shares %+v %v calls%d", out, e, calls)
	}
	safe, _ := json.Marshal(out)
	if strings.Contains(string(safe)+fmt.Sprintf("%+v %#v", out, out), "private-share-metadata") || strings.Contains(string(safe), "synthetic-private-filter") {
		t.Fatal("metadata or filter leaked")
	}
}

func TestFileStorageSourceObjectFieldsRejectScalarShapes(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"share_types":[{"id":"type-1","name":"synthetic-type","description":null,"share_type_access:is_public":false,"is_default":false,"extra_specs":"not-an-object","required_extra_specs":{}}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	out, e := view.FileStorage().ListTypes(context.Background(), filestorage.TypeFilters{}, pagination.Options{})
	if types.ErrorCode(e) != types.ProtocolError || out.Complete {
		t.Fatalf("shape %+v %v", out, e)
	}
}

func TestFileStorageProtectedFiltersBindCursorWithoutDisclosingSelectors(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"shares":[` + fileStorageShareJSON("share-1", "test-project") + `]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	var first, second types.ProtectedJSON
	_ = json.Unmarshal([]byte(`{"owner":"synthetic-secret-first"}`), &first)
	_ = json.Unmarshal([]byte(`{"owner":"synthetic-secret-second"}`), &second)
	out, e := view.FileStorage().ListShares(context.Background(), filestorage.ShareFilters{Metadata: &first}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || out.NextCursor == nil || calls != 1 {
		t.Fatalf("cursor %+v %v", out, e)
	}
	safe, _ := json.Marshal(out.NextCursor)
	if strings.Contains(string(safe)+fmt.Sprintf("%+v %#v", out.NextCursor, out.NextCursor), "synthetic-secret-first") {
		t.Fatal("selector leaked")
	}
	_, e = view.FileStorage().ListShares(context.Background(), filestorage.ShareFilters{Metadata: &second}, pagination.Options{Cursor: out.NextCursor})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatalf("cursor reused across protected filters %v calls%d", e, calls)
	}
}

func TestFileStorageAccessRulesUseExplicitShareParentAndProtectKeys(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/share/v2/share-access-rules" || r.URL.Query().Get("share_id") != "share-1" || r.Header.Get("X-OpenStack-Manila-API-Version") != "2.73" {
			t.Fatalf("request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_list":[{"id":"rule-1","access_level":"rw","access_to":"synthetic-private-ip","access_type":"ip","state":"future-state","access_key":"synthetic-private-key","created_at":null,"updated_at":null,"metadata":{}}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	view, _ := p.Location("spb", "ru-9")
	share := types.ResourceRef{Service: "filestorage", APIGeneration: "v2_73", Kind: "share", ID: "share-1", Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}, EndpointID: "selectel-filestorage.v2_73:ru-9", EndpointURL: "https://ru-9.cloud.api.selcloud.ru/share/v2"}
	out, e := view.FileStorage().ListAccessRules(context.Background(), share, nil, pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.Parent == nil || *out.Items[0].Ref.Parent != share || out.Items[0].State == nil || *out.Items[0].State != "future-state" {
		t.Fatalf("access %+v %v", out, e)
	}
	safe, _ := json.Marshal(out)
	if strings.Contains(string(safe)+fmt.Sprintf("%+v %#v", out, out), "synthetic-private-") {
		t.Fatal("access secret leaked")
	}
}

func TestFileStorageAccessRuleForeignParentNeverAdopted(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_list":[{"id":"foreign-rule","share_id":"other-share","access_level":"rw","access_to":"synthetic-private-ip","access_type":"ip","state":"active","access_key":null,"created_at":null,"updated_at":null,"metadata":{}}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	share := types.ResourceRef{Service: "filestorage", APIGeneration: "v2_73", Kind: "share", ID: "share-1", Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}, EndpointID: "selectel-filestorage.v2_73:ru-9", EndpointURL: "https://ru-9.cloud.api.selcloud.ru/share/v2"}
	out, e := v.FileStorage().ListAccessRules(context.Background(), share, nil, pagination.Options{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Complete || len(out.Items) != 0 {
		t.Fatalf("foreign %+v %v", out, e)
	}
	share.APIGeneration = "v2_0"
	_, e = v.FileStorage().ListAccessRules(context.Background(), share, nil, pagination.Options{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatalf("generation %v calls%d", e, calls)
	}
}

func TestFileStorageAllowRetainsAcceptedRuleBeforeBadProtectedField(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"allow_access":{"access_level":"ro","access_to":"192.0.2.1","access_type":"ip"}}` {
			t.Fatalf("body %s", raw)
		}
		return &http.Response{StatusCode: 202, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access":{"id":"created-rule","share_id":"share-1","access_level":"ro","access_to":17,"access_type":"ip","state":"queued","access_key":null,"created_at":null,"updated_at":null,"metadata":{}}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	share := types.ResourceRef{Service: "filestorage", APIGeneration: "v2_73", Kind: "share", ID: "share-1", Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}, EndpointID: "selectel-filestorage.v2_73:ru-9", EndpointURL: "https://ru-9.cloud.api.selcloud.ru/share/v2"}
	accessType, level := "ip", "ro"
	target := types.Protect([]byte("192.0.2.1"))
	out, e := v.FileStorage().AllowAccess(context.Background(), share, filestorage.AccessCreateRequest{AccessType: &accessType, AccessLevel: &level, AccessTo: &target}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.ResourceRefs[1].ID != "created-rule" || out.Receipt.ResourceRefs[1].Parent == nil || *out.Receipt.ResourceRefs[1].Parent != share || calls != 1 {
		t.Fatalf("allow %+v %v", out, e)
	}
}

func TestFileStorageAllowUnknownNeverReplaysOrInfersRule(t *testing.T) {
	writes := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		writes++
		return nil, io.ErrUnexpectedEOF
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	share := types.ResourceRef{Service: "filestorage", APIGeneration: "v2_73", Kind: "share", ID: "share-1", Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}, EndpointID: "selectel-filestorage.v2_73:ru-9", EndpointURL: "https://ru-9.cloud.api.selcloud.ru/share/v2"}
	kind := "ip"
	target := types.Protect([]byte("192.0.2.1"))
	out, e := v.FileStorage().AllowAccess(context.Background(), share, filestorage.AccessCreateRequest{AccessType: &kind, AccessTo: &target}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != share || out.Receipt.Recovery != "unavailable" || writes != 1 {
		t.Fatalf("unknown %+v %v writes%d", out, e, writes)
	}
}

func TestFileStorageDenyRequiresExactRuleParentAndPreservesReceipt(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"deny_access":{"access_id":"rule-1"}}` || r.URL.Path != "/share/v2/shares/share-1/action" {
			t.Fatalf("deny %s %s", r.URL, raw)
		}
		return &http.Response{StatusCode: 202, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	share := types.ResourceRef{Service: "filestorage", APIGeneration: "v2_73", Kind: "share", ID: "share-1", Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}, EndpointID: "selectel-filestorage.v2_73:ru-9", EndpointURL: "https://ru-9.cloud.api.selcloud.ru/share/v2"}
	rule := share
	rule.Kind = "access_rule"
	rule.ID = "rule-1"
	out, e := v.FileStorage().DenyAccess(context.Background(), rule, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("missing parent %+v %v", out, e)
	}
	rule.Parent = &share
	out, e = v.FileStorage().DenyAccess(context.Background(), rule, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "rule-1" || out.Receipt.ResourceRefs[0].Parent == nil || calls != 1 {
		t.Fatalf("deny %+v %v", out, e)
	}
}
