package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/audit"
	"github.com/festy23/selectel-sdk/databases"
	"github.com/festy23/selectel-sdk/inventory"
	"github.com/festy23/selectel-sdk/mks"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/projects"
	"github.com/festy23/selectel-sdk/registry"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func inventoryReply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func inventoryClient(t *testing.T, wire transportFunc, timeout time.Duration) *sdk.Client {
	t.Helper()
	rt := transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Header.Get("X-Auth-Token") == "" {
			t.Error("IAM authorization missing")
		}
		return wire(r)
	})
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: rt, CollectionTimeout: timeout})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func projectView(t *testing.T, c *sdk.Client) sdk.ProjectClient {
	t.Helper()
	p, e := c.Project("test-project")
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func regionalView(t *testing.T, c *sdk.Client, pool string) sdk.RegionalClient {
	t.Helper()
	r, e := projectView(t, c).Location("", pool)
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestProjectsPreservePresenceAndIgnoreInjectedSDKRef(t *testing.T) {
	n := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		n++
		if r.URL.Host != "api.selectel.ru" || r.URL.Path != "/vpc/resell/v2/projects" {
			t.Error("Resell generation duplicated")
		}
		if n == 1 && r.URL.Query().Has("all") {
			t.Error("default all was sent")
		}
		if n == 2 && r.URL.Query().Get("all") != "false" {
			t.Error("explicit false lost")
		}
		return inventoryReply(200, `{"projects":[{"id":"one","name":"name","enabled":false},{"name":"without-id","ref":{"scope":"injected-invalid-value"}}]}`), nil
	}, 0)
	first, e := c.Account().Projects().Collect(context.Background(), projects.ListOptions{}, pagination.Options{})
	if e != nil || !first.Complete || first.Items[0].Enabled == nil || *first.Items[0].Enabled || first.Items[1].Enabled != nil || first.Items[1].Ref != nil {
		t.Fatal("presence or ref boundary broken", e)
	}
	f := false
	if _, e = c.Account().Projects().Collect(context.Background(), projects.ListOptions{All: &f}, pagination.Options{}); e != nil {
		t.Fatal(e)
	}
}
func TestDatabaseFiltersScopeProtectedObjectsAndRawProviderTime(t *testing.T) {
	name := "db"
	enabled := "false"
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "ru-1.dbaas.selcloud.ru" || r.URL.Path != "/v1/datastores" || r.URL.Query().Get("project_id") != "test-project" || r.URL.Query().Get("enabled") != "false" || r.URL.Query().Get("name") != "db" {
			t.Error("database request lost mapping")
		}
		return inventoryReply(200, `{"datastores":[{"id":"db-id","project_id":"test-project","created_at":"2025-01-01T12:00:00","enabled":false,"disk_used":900719925474099312345,"config":{"credential":"synthetic-private-config","integer":900719925474099312345},"instances":[],"firewall":[],"security_groups":[]}]}`), nil
	}, 0)
	r, e := regionalView(t, c, "ru-1").Databases().V1.Datastores.Collect(context.Background(), databases.ListOptions{Name: &name, Enabled: &enabled}, pagination.Options{})
	if e != nil || r.Items[0].DiskUsed.String() != "900719925474099312345" || *r.Items[0].CreatedAt != "2025-01-01T12:00:00" || r.Items[0].Instances == nil {
		t.Fatal("database data changed", e)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "synthetic-private-config") || strings.Contains(fmt.Sprintf("%+v", r), "synthetic-private-config") {
		t.Fatal("config disclosed")
	}
	if !strings.Contains(string(r.Items[0].Config.Reveal()), "900719925474099312345") {
		t.Fatal("explicit protected JSON read lost precision")
	}
}

const clusterJSON = `{"id":"cluster-id","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","name":"cluster","project_id":"test-project","status":"ACTIVE","basic":false,"enable_autorepair":true,"enable_patch_version_auto_upgrade":false,"private_kube_api":true,"kube_api_ip":"10.0.0.1","subnet_id":"subnet","kube_version":"1.32","network_id":"network","network_type":"STANDARD","pool":"ru-9","cni_type":"CALICO","cluster_type":"HIGH_AVAILABILITY","kubernetes_options":{"admission_controllers":[],"feature_gates":[],"x509_ca_certificates":"","audit_logs":{"enabled":false,"secret_name":"mks-audit-logs"},"oidc":{"client_id":"","enabled":false,"groups_claim":"","issuer_url":"","provider_name":"","username_claim":"","ca_certs":""}}}`

func TestKubernetesUsesDocumentedPoolAliasAndRejectsForeignScope(t *testing.T) {
	foreign := false
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "ru-8.mks.selcloud.ru" || r.URL.Path != "/v2/clusters" {
			t.Error("documented ru-9 alias lost")
		}
		body := clusterJSON
		if foreign {
			body = strings.Replace(body, `"test-project"`, `"foreign-project"`, 1)
		}
		return inventoryReply(200, `{"clusters":[`+body+`]}`), nil
	}, 0)
	clusters := regionalView(t, c, "ru-9").MKS().V2.Clusters
	r, e := clusters.Collect(context.Background(), mks.ListOptions{}, pagination.Options{})
	if e != nil || r.Items[0].Ref.Scope.Pool != "ru-9" || r.Items[0].Basic == nil || *r.Items[0].Basic {
		t.Fatal("cluster decode failed", e)
	}
	foreign = true
	r, e = clusters.Collect(context.Background(), mks.ListOptions{}, pagination.Options{})
	if types.ErrorCode(e) != types.ScopeMismatch || len(r.Items) != 0 {
		t.Fatal("foreign cluster returned")
	}
}
func TestRegistryOffsetPaginationAndImmutableFilters(t *testing.T) {
	search := "original"
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api/v2/registries" || r.URL.Query().Get("search") != "original" {
			t.Error("registry mapping or filters changed")
		}
		offset := r.URL.Query().Get("offset")
		if offset == "0" {
			return inventoryReply(200, `{"registries":[{"id":"one","name":"registry","createdAt":"2025-01-01T03:00:00+03:00","status":"ACTIVE","size":900719925474099312345}],"totalCount":2}`), nil
		}
		if offset != "1" {
			t.Error("offset not based on returned count")
		}
		return inventoryReply(200, `{"registries":[{"id":"two","name":"registry","createdAt":"2025-01-01T00:00:00Z","status":"ACTIVE","size":0}],"totalCount":2}`), nil
	}, 0)
	it, e := projectView(t, c).Registry().V2.Registries.Iterate(registry.ListOptions{Search: &search}, pagination.Options{})
	if e != nil {
		t.Fatal(e)
	}
	search = "changed"
	r, e := it.Collect(context.Background())
	if e != nil || !r.Complete || calls != 2 || r.Items[0].Size.String() != "900719925474099312345" || r.Items[0].CreatedAt.Location() != time.UTC || len(r.ProviderPages) != 2 || r.ProviderPages[0].TotalCount.String() != "2" {
		t.Fatal("registry pagination failed", e)
	}
}
func TestInventoryContinuesAfterPermissionFailureAndExcludesPayloads(t *testing.T) {
	paths := []string{}
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/vpc/resell/v2/projects":
			return inventoryReply(200, `{"projects":[{"id":"project"}]}`), nil
		case "/api/v2/registries":
			return inventoryReply(403, `{"message":"synthetic-private-error"}`), nil
		case "/v1/datastores":
			return inventoryReply(200, `{"datastores":[{"id":"db","project_id":"test-project","config":{"password":"synthetic-private-config"},"connection":{"host":"endpoint"}}]}`), nil
		}
		t.Fatal("unexpected path")
		return nil, nil
	}, 0)
	scope := regionalView(t, c, "ru-1").Scope()
	result, e := c.Inventory().Scan(context.Background(), inventory.Plan{Branches: []inventory.Branch{{Scope: c.Scope(), OperationID: "projects.v2.projects.list"}, {Scope: projectView(t, c).Scope(), OperationID: "registry.v2.registries.list"}, {Scope: scope, OperationID: "databases.v1.datastores.list"}}})
	if types.ErrorCode(e) != types.Incomplete || result.Complete || len(paths) != 3 || len(result.Branches) != 3 || len(result.Coverage) != 3 || !result.Branches[2].Complete || result.Branches[1].Error.Code != types.PermissionDenied {
		t.Fatal("scan skipped branches or hid failure", e)
	}
	if result.Branches[2].Items.Datastores[0].Config != nil || result.Branches[2].Items.Datastores[0].Connection != nil {
		t.Fatal("inventory contains protected payloads")
	}
}
func TestInvalidInventoryPlanPerformsNoIO(t *testing.T) {
	n := 0
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) { n++; return nil, io.EOF }, 0)
	account := inventory.Branch{Scope: c.Scope(), OperationID: "projects.v2.projects.list"}
	badCompute := inventory.Branch{Scope: c.Scope(), OperationID: "compute.v2_1.servers.list"}
	for _, branches := range [][]inventory.Branch{{account, account}, {account, badCompute}} {
		_, e := c.Inventory().Scan(context.Background(), inventory.Plan{Branches: branches})
		if types.ErrorCode(e) != types.InvalidArgument || n != 0 {
			t.Fatal("invalid plan reached network", e)
		}
	}
}
func TestInventoryCancellationKeepsLaterBranchesNotStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) { calls++; cancel(); return nil, context.Canceled }, 0)
	p := projectView(t, c).Scope()
	r, e := c.Inventory().Scan(ctx, inventory.Plan{Branches: []inventory.Branch{{Scope: c.Scope(), OperationID: "projects.v2.projects.list"}, {Scope: p, OperationID: "registry.v2.registries.list"}}})
	if types.ErrorCode(e) != types.Incomplete || calls != 1 || r.Branches[1].StopReason != "not_started" || r.Branches[1].Error.Code != types.Canceled {
		t.Fatal("canceled scan continued IO", e)
	}
}
func TestAuditUsesSafePOSTReadPaginationAndProtectsPayload(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.selectel.ru" || r.URL.Path != "/audit-logs/v1/logs" || r.Method != "POST" {
			t.Error("audit endpoint mapping wrong")
		}
		var b map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&b)
		if len(b["event_saved_time_from"]) == 0 || len(b["event_saved_time_to"]) == 0 {
			t.Error("audit time bounds missing")
		}
		if calls == 1 {
			return inventoryReply(200, `{"data":[],"pagination":{"next_cursor":"synthetic-private-cursor"}}`), nil
		}
		if r.URL.Query().Get("cursor") != "synthetic-private-cursor" {
			t.Error("audit cursor changed")
		}
		return inventoryReply(200, `{"data":[{"event_id":"event","resource":{"account_id":"test-account","old_values":{"password":"synthetic-private-value"}},"request":{"parameters":"token=synthetic-private-query"}}],"pagination":{"prev_cursor":"synthetic-private-previous","count":1}}`), nil
	}, 0)
	r, e := c.Account().Audit().Query(context.Background(), audit.QueryRequest{Start: time.Unix(1, 0), End: time.Unix(2, 0)}, pagination.Options{})
	if e != nil || !r.Complete || len(r.Items) != 1 || calls != 2 || len(r.ProviderPages) != 1 || r.ProviderPages[0].ReportedCount.String() != "1" || r.ProviderPages[0].PreviousToken == nil {
		t.Fatal("audit pagination failed", e)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("audit payload leaked")
	}
	if string(r.Items[0].Request.Parameters.Reveal()) != "token=synthetic-private-query" {
		t.Fatal("explicit protected audit value lost")
	}
}
func TestLocationPoolDerivationAndConflictAreOffline(t *testing.T) {
	n := 0
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) { n++; return nil, io.EOF }, 0)
	p := projectView(t, c)
	r, e := p.Location("", "ru-1")
	if e != nil || r.Scope().Region != "spb" || n != 0 {
		t.Fatal("pool registry unavailable")
	}
	if _, e = p.Location("msk", "ru-1"); types.ErrorCode(e) != types.ScopeMismatch || n != 0 {
		t.Fatal("conflicting location accepted")
	}
}
