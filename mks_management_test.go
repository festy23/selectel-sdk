package selectelsdk_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/mks"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

func mksClient(t *testing.T, wire transportFunc) *sdk.Client {
	t.Helper()
	transport := transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Header.Get("X-Auth-Token") == "" {
			t.Error("MKS IAM token missing")
		}
		return wire(r)
	})
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transport, PollInterval: time.Millisecond, WaitTimeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func mksRef(kind, id string, parent *types.ResourceRef) types.ResourceRef {
	return types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}, Service: "mks", APIGeneration: "v2", Kind: kind, ID: id, Parent: parent, EndpointID: "selectel-mks.v2:ru-9", EndpointURL: "https://ru-8.mks.selcloud.ru/v2"}
}
func detailedClusterJSON(status string) string {
	var v map[string]json.RawMessage
	_ = json.Unmarshal([]byte(clusterJSON), &v)
	v["status"], _ = json.Marshal(status)
	v["additional_software"] = json.RawMessage(`{"synthetic_private":"synthetic-mks-private-value"}`)
	v["maintenance_last_start"] = json.RawMessage(`"2025-01-01T00:00:00Z"`)
	v["maintenance_window_start"] = json.RawMessage(`"03:00:00"`)
	v["maintenance_window_end"] = json.RawMessage(`"04:00:00"`)
	v["pki_tree_updated_at"] = json.RawMessage(`"2025-01-01T00:00:00Z"`)
	raw, _ := json.Marshal(v)
	return string(raw)
}

const mksNodeJSON = `{"id":"node-id","created_at":"2025-01-01T03:00:00+03:00","hostname":"worker","ip":"10.0.0.1","nodegroup_id":"group-id","provider_server_id":"server-id"}`

func nodegroupJSON(list bool) string {
	v := `{"id":"group-id","cluster_id":"cluster-id","created_at":"2025-01-01T00:00:00Z","status":"ACTIVE","nodegroup_type":"STANDARD","enable_autoscale":false,"install_nvidia_device_plugin":false,"preemptible":false,"labels":{},"taints":[],"segment":"ru-9a","nodes":[` + mksNodeJSON + `]`
	if list {
		return v + `,"available_additional_info":{"user_data":true}}`
	}
	return v + `,"user_data":"c3ludGhldGljLXByaXZhdGUtdXNlcmRhdGE="}`
}

const mksTaskJSON = `{"id":"task-id","cluster_id":"cluster-id","started_at":"2025-01-01T00:00:00Z","status":"DONE","type":"CREATE_CLUSTER","error_details":{"code":105,"name":"synthetic-private-task-error","details":"synthetic-private-task-details"}}`
const mksIntegrationJSON = `{"status":"ACTIVE","registries":[{"id":"registry-id","name":"registry"}]}`

func clusterSpec(t *testing.T) mks.ClusterCreateSpec {
	t.Helper()
	var software types.ProtectedJSON
	if json.Unmarshal([]byte(`{"synthetic_private":"synthetic-mks-private-value"}`), &software) != nil {
		t.Fatal("fixture invalid")
	}
	return mks.ClusterCreateSpec{Name: managementPtr("cluster"), NetworkID: managementPtr("network-id"), SubnetID: managementPtr("subnet-id"), KubeVersion: managementPtr("1.32.0"), PrivateKubeAPI: managementPtr(false), Pool: managementPtr("ru-9"), NetworkType: managementPtr("STANDARD"), MaintenanceWindowStart: managementPtr(""), CloudSubnetCidr: managementPtr(""), Basic: managementPtr(false), AdditionalSoftware: &software}
}

func TestAllMksV2WireBindings(t *testing.T) {
	cluster := mksRef("cluster", "cluster-id", nil)
	group := mksRef("nodegroup", "group-id", &cluster)
	node := mksRef("node", "node-id", &group)
	task := mksRef("task", "task-id", &cluster)
	spec := clusterSpec(t)
	zero, _ := types.ParseInteger("0")
	encoded := base64.StdEncoding.EncodeToString([]byte("synthetic-private-userdata"))
	protected := types.Protect([]byte(encoded))
	groupSpec := mks.NodegroupCreateSpec{Segment: managementPtr("ru-9a"), Count: &zero, EnableAutoscale: managementPtr(false), Labels: map[string]string{}, Taints: []mks.NodegroupTaint{}, UserData: &protected}
	type testCase struct {
		name, method, path, body, response string
		status                             int
		run                                func(mks.V2Client) error
	}
	tests := []testCase{
		{"clusters.list", "GET", "/clusters", "", `{"clusters":[` + clusterJSON + `]}`, 200, func(v mks.V2Client) error {
			_, e := v.Clusters.ListPage(context.Background(), mks.ListOptions{}, pagination.Options{})
			return e
		}},
		{"clusters.create", "POST", "/clusters", "", `{"cluster":` + detailedClusterJSON("PENDING_CREATE") + `}`, 201, func(v mks.V2Client) error {
			r, e := v.Clusters.Create(context.Background(), spec)
			if e == nil && (r.Receipt.Outcome != types.Accepted || r.Data.Ref.ID != "cluster-id") {
				t.Error("cluster create receipt lost")
			}
			return e
		}},
		{"clusters.get", "GET", "/clusters/cluster-id", "", `{"cluster":` + detailedClusterJSON("ACTIVE") + `}`, 200, func(v mks.V2Client) error { _, e := v.Clusters.Get(context.Background(), cluster); return e }},
		{"clusters.replace", "PUT", "/clusters/cluster-id", `{"cluster":{"enable_autorepair":false,"maintenance_window_start":""}}`, `{"cluster":` + detailedClusterJSON("PENDING_UPDATE") + `}`, 200, func(v mks.V2Client) error {
			_, e := v.Clusters.Replace(context.Background(), cluster, mks.ClusterUpdateSpec{EnableAutorepair: managementPtr(false), MaintenanceWindowStart: managementPtr("")})
			return e
		}},
		{"clusters.update", "PATCH", "/clusters/cluster-id", `{"cluster":{"enable_autorepair":false}}`, `{"cluster":` + detailedClusterJSON("PENDING_UPDATE") + `}`, 200, func(v mks.V2Client) error {
			r, e := v.Clusters.Update(context.Background(), cluster, mks.ClusterUpdateSpec{EnableAutorepair: managementPtr(false)})
			if e == nil && len(r.Receipt.ResourceRefs) != 1 {
				t.Error("duplicate request/response refs")
			}
			return e
		}},
		{"clusters.delete", "DELETE", "/clusters/cluster-id", "", "", 204, func(v mks.V2Client) error { _, e := v.Clusters.Delete(context.Background(), cluster); return e }},
		{"clusters.kubeconfig", "GET", "/clusters/cluster-id/kubeconfig", "", "apiVersion: v1\nusers:\n- token: synthetic-private-kubeconfig\n", 200, func(v mks.V2Client) error {
			r, e := v.Clusters.Kubeconfig(context.Background(), cluster)
			if e == nil {
				b, _ := json.Marshal(r)
				if strings.Contains(string(b), "synthetic-private") || strings.Contains(fmt.Sprintf("%+v", r), "synthetic-private") {
					t.Error("kubeconfig disclosed")
				}
				if !strings.Contains(string(r.Data.Reveal()), "synthetic-private-kubeconfig") {
					t.Error("explicit kubeconfig read lost data")
				}
			}
			return e
		}},
		{"clusters.rotate", "POST", "/clusters/cluster-id/rotate-certs", "", "", 204, func(v mks.V2Client) error {
			_, e := v.Clusters.RotateCertificates(context.Background(), cluster)
			return e
		}},
		{"clusters.upgrade-patch", "POST", "/clusters/cluster-id/upgrade-patch-version", "", `{"cluster":` + detailedClusterJSON("PENDING_UPGRADE_PATCH_VERSION") + `}`, 200, func(v mks.V2Client) error {
			_, e := v.Clusters.UpgradePatchVersion(context.Background(), cluster)
			return e
		}},
		{"clusters.upgrade-minor", "POST", "/clusters/cluster-id/upgrade-minor-version", "", `{"cluster":` + detailedClusterJSON("PENDING_UPGRADE_MINOR_VERSION") + `}`, 200, func(v mks.V2Client) error {
			_, e := v.Clusters.UpgradeMinorVersion(context.Background(), cluster)
			return e
		}},
		{"nodegroups.list", "GET", "/clusters/cluster-id/nodegroups", "", `{"nodegroups":[` + nodegroupJSON(true) + `]}`, 200, func(v mks.V2Client) error {
			r, e := v.Nodegroups.ListPage(context.Background(), cluster, pagination.Options{})
			if e == nil && (r.Items[0].Ref.Parent.ID != cluster.ID || r.Items[0].Nodes[0].Ref.Parent.ID != group.ID || r.Items[0].Nodes[0].CreatedAt.Location() != time.UTC) {
				t.Error("nodegroup hierarchy or timestamp lost")
			}
			return e
		}},
		{"nodegroups.create", "POST", "/clusters/cluster-id/nodegroups", `{"nodegroups":[{"segment":"ru-9a","count":0,"enable_autoscale":false,"labels":{},"taints":[],"user_data":"` + encoded + `"}]}`, "", 204, func(v mks.V2Client) error {
			r, e := v.Nodegroups.Create(context.Background(), cluster, []mks.NodegroupCreateSpec{groupSpec})
			if e == nil && (r.Receipt.Outcome != types.Accepted || len(r.Receipt.ResourceRefs) != 1 || r.Receipt.ResourceRefs[0].Kind != "cluster" || r.Receipt.ProviderOperationRef != nil) {
				t.Error("created nodegroup/task ID invented")
			}
			return e
		}},
		{"nodegroups.get", "GET", "/clusters/cluster-id/nodegroups/group-id", "", `{"nodegroup":` + nodegroupJSON(false) + `}`, 200, func(v mks.V2Client) error {
			r, e := v.Nodegroups.Get(context.Background(), group)
			if e == nil {
				b, _ := json.Marshal(r)
				if strings.Contains(string(b), "c3ludGhldGlj") {
					t.Error("userdata disclosed")
				}
			}
			return e
		}},
		{"nodegroups.replace", "PUT", "/clusters/cluster-id/nodegroups/group-id", `{"nodegroup":{"enable_autoscale":false,"labels":{},"taints":[]}}`, "", 204, func(v mks.V2Client) error {
			_, e := v.Nodegroups.Replace(context.Background(), group, mks.NodegroupUpdateSpec{EnableAutoscale: managementPtr(false), Labels: map[string]string{}, Taints: []mks.NodegroupTaint{}})
			return e
		}},
		{"nodegroups.delete", "DELETE", "/clusters/cluster-id/nodegroups/group-id", "", "", 204, func(v mks.V2Client) error { _, e := v.Nodegroups.Delete(context.Background(), group); return e }},
		{"nodegroups.resize", "POST", "/clusters/cluster-id/nodegroups/group-id/resize", `{"nodegroup":{"desired":0}}`, "", 204, func(v mks.V2Client) error {
			_, e := v.Nodegroups.Resize(context.Background(), group, mks.NodegroupResizeSpec{Desired: &zero})
			return e
		}},
		{"nodes.get", "GET", "/clusters/cluster-id/nodegroups/group-id/nodes/node-id", "", `{"node":` + mksNodeJSON + `}`, 200, func(v mks.V2Client) error { _, e := v.Nodes.Get(context.Background(), node); return e }},
		{"nodes.delete", "DELETE", "/clusters/cluster-id/nodegroups/group-id/nodes/node-id", "", "", 204, func(v mks.V2Client) error { _, e := v.Nodes.Delete(context.Background(), node); return e }},
		{"nodes.reinstall", "POST", "/clusters/cluster-id/nodegroups/group-id/nodes/node-id/reinstall", "", "", 204, func(v mks.V2Client) error { _, e := v.Nodes.Reinstall(context.Background(), node); return e }},
		{"tasks.list", "GET", "/clusters/cluster-id/tasks", "", `{"count":1,"tasks":[` + mksTaskJSON + `]}`, 200, func(v mks.V2Client) error {
			_, e := v.Tasks.ListPage(context.Background(), cluster, mks.TaskListOptions{WithErrorDetails: managementPtr(false)}, pagination.Options{})
			return e
		}},
		{"tasks.get", "GET", "/clusters/cluster-id/tasks/task-id", "", `{"task":` + mksTaskJSON + `}`, 200, func(v mks.V2Client) error {
			r, e := v.Tasks.Get(context.Background(), task, mks.TaskOptions{WithErrorDetails: managementPtr(false)})
			if e == nil {
				b, _ := json.Marshal(r)
				if strings.Contains(string(b), "synthetic-private-task") || strings.Contains(fmt.Sprintf("%+v", r), "synthetic-private-task") {
					t.Error("task error text disclosed")
				}
				if string(r.Data.ErrorDetails.Details.Reveal()) != "synthetic-private-task-details" {
					t.Error("task details lost")
				}
			}
			return e
		}},
		{"registries.create", "POST", "/clusters/cluster-id/registries", `{"registries":[{"id":"registry-id"}]}`, mksIntegrationJSON, 201, func(v mks.V2Client) error {
			_, e := v.Registries.Create(context.Background(), cluster, []mks.RegistryIntegrationCreateSpec{{ID: managementPtr("registry-id")}})
			return e
		}},
		{"registries.get", "GET", "/clusters/cluster-id/registries", "", mksIntegrationJSON, 200, func(v mks.V2Client) error { _, e := v.Registries.Get(context.Background(), cluster); return e }},
		{"registries.replace", "PUT", "/clusters/cluster-id/registries", `{"registries":[]}`, mksIntegrationJSON, 200, func(v mks.V2Client) error {
			_, e := v.Registries.Replace(context.Background(), cluster, []mks.RegistryIntegrationUpdateSpec{})
			return e
		}},
		{"registries.delete-all", "DELETE", "/clusters/cluster-id/registries", "", "", 204, func(v mks.V2Client) error { _, e := v.Registries.DeleteAll(context.Background(), cluster); return e }},
		{"registries.delete", "DELETE", "/clusters/cluster-id/registries/registry-id", "", "", 204, func(v mks.V2Client) error {
			_, e := v.Registries.Delete(context.Background(), cluster, "registry-id")
			return e
		}},
		{"configuration.admission", "GET", "/admission-controllers", "", `{"admission_controllers":[{"kube_version_minor":"1.32","names":["AlwaysPullImages"]}]}`, 200, func(v mks.V2Client) error {
			_, e := v.Configuration.AdmissionControllers(context.Background())
			return e
		}},
		{"configuration.versions", "GET", "/kubeversions", "", `{"kube_versions":[{"version":"1.32.0","is_default":false}]}`, 200, func(v mks.V2Client) error { _, e := v.Configuration.KubernetesVersions(context.Background()); return e }},
		{"configuration.features", "GET", "/feature-gates", "", `{"feature_gates":[{"kube_version_minor":"1.32","names":["Feature"]}]}`, 200, func(v mks.V2Client) error { _, e := v.Configuration.FeatureGates(context.Background()); return e }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := mksClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "ru-8.mks.selcloud.ru" || r.URL.Path != "/v2"+tc.path || r.Method != tc.method {
					t.Error("MKS binding path/method/alias mismatch")
				}
				body, _ := io.ReadAll(r.Body)
				if tc.body != "" {
					var got, want any
					if json.Unmarshal(body, &got) != nil || json.Unmarshal([]byte(tc.body), &want) != nil || fmt.Sprint(got) != fmt.Sprint(want) {
						t.Error("MKS body mapping differs")
					}
				}
				if tc.name == "clusters.create" {
					var v map[string]map[string]json.RawMessage
					_ = json.Unmarshal(body, &v)
					if !strings.Contains(string(v["cluster"]["additional_software"]), "synthetic-mks-private-value") || string(v["cluster"]["basic"]) != "false" {
						t.Error("protected request or explicit false was changed")
					}
					if _, ok := v["cluster"]["enable_autorepair"]; ok {
						t.Error("provider default inserted")
					}
				}
				if strings.HasPrefix(tc.name, "tasks.") && r.URL.Query().Get("with_error_details") != "false" {
					t.Error("explicit task false lost")
				}
				return inventoryReply(tc.status, tc.response), nil
			})
			if e := tc.run(regionalView(t, c, "ru-9").MKS().V2); e != nil || calls != 1 {
				t.Fatal("MKS method failed", e, calls)
			}
		})
	}
}

func TestMksCreateWaitResumeAndUnknownStates(t *testing.T) {
	for _, state := range []string{"ACTIVE", "ERROR", "UNRECOGNIZED_FUTURE_STATE"} {
		t.Run(state, func(t *testing.T) {
			gets := 0
			posts := 0
			c := mksClient(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == "POST" {
					posts++
					return inventoryReply(201, `{"cluster":`+detailedClusterJSON("PENDING_CREATE")+`}`), nil
				}
				gets++
				s := "PENDING_CREATE"
				if gets > 1 {
					s = state
				}
				return inventoryReply(200, `{"cluster":`+detailedClusterJSON(s)+`}`), nil
			})
			result, e := regionalView(t, c, "ru-9").MKS().V2.Clusters.CreateAndWait(context.Background(), clusterSpec(t))
			want := types.Code("")
			if state == "ERROR" {
				want = types.OperationFailed
			}
			if state == "UNRECOGNIZED_FUTURE_STATE" {
				want = types.StateUnknown
			}
			if types.ErrorCode(e) != want || posts != 1 || gets != 2 || result.Create.Receipt.Outcome != types.Accepted {
				t.Fatal("cluster wait semantics broken", e)
			}
			h, e := c.Resume(*result.Create.Receipt)
			if e != nil || h.Resource().Kind != "cluster" {
				t.Fatal("accepted MKS receipt cannot resume", e)
			}
		})
	}
}

func TestMksUnknownMutationAndGroupCreationDoNotAdoptIDs(t *testing.T) {
	cluster := mksRef("cluster", "cluster-id", nil)
	calls := 0
	c := mksClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "DELETE" {
			return nil, io.ErrUnexpectedEOF
		}
		return inventoryReply(404, `{}`), nil
	})
	v, e := regionalView(t, c, "ru-9").MKS().V2.Clusters.Delete(context.Background(), cluster)
	if types.ErrorCode(e) != types.UnknownOutcome || v.Receipt.Outcome != types.Unknown || calls != 1 {
		t.Fatal("MKS lost mutation retried", e)
	}
	h, e := c.Resume(*v.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovery, e := h.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || recovery.ProofStatus != "indeterminate" || recovery.Receipt.Outcome != types.Unknown || recovery.Observation.Classification != "absent" {
		t.Fatal("absence treated as mutation proof", e)
	}
	c = mksClient(t, func(r *http.Request) (*http.Response, error) { return inventoryReply(204, ""), nil })
	created, e := regionalView(t, c, "ru-9").MKS().V2.Nodegroups.Create(context.Background(), cluster, []mks.NodegroupCreateSpec{})
	if e != nil {
		t.Fatal(e)
	}
	h, e = c.Resume(*created.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.Wait(context.Background(), "cluster.active"); types.ErrorCode(e) != types.CapabilityUnavailable {
		t.Fatal("parent cluster readiness substituted for new nodegroup completion", e)
	}
}

func TestMksRejectsForeignIdentityBeforeRetainingRefs(t *testing.T) {
	for _, body := range []string{`{"cluster":{"id":"known","project_id":"foreign","name":123}}`, `{"cluster":{"id":"known","pool":"ru-8","name":123}}`} {
		c := mksClient(t, func(*http.Request) (*http.Response, error) { return inventoryReply(201, body), nil })
		v, e := regionalView(t, c, "ru-9").MKS().V2.Clusters.Create(context.Background(), clusterSpec(t))
		if types.ErrorCode(e) != types.ScopeMismatch || v.Receipt.Outcome != types.Accepted || len(v.Receipt.ResourceRefs) != 0 {
			t.Fatal("foreign provider identity retained", e)
		}
	}
	c := mksClient(t, func(*http.Request) (*http.Response, error) {
		return inventoryReply(201, `{"cluster":{"id":"known","project_id":"test-project","pool":"ru-9","name":123}}`), nil
	})
	v, e := regionalView(t, c, "ru-9").MKS().V2.Clusters.Create(context.Background(), clusterSpec(t))
	if types.ErrorCode(e) != types.ProtocolError || v.Receipt.Outcome != types.Accepted || len(v.Receipt.ResourceRefs) != 1 || v.Receipt.ResourceRefs[0].ID != "known" {
		t.Fatal("accepted malformed response lost trustworthy ID", e)
	}
}

func TestMksPreflightParentAndInputValidation(t *testing.T) {
	cluster := mksRef("cluster", "cluster-id", nil)
	group := mksRef("nodegroup", "group-id", &cluster)
	for _, kind := range []string{"pool", "missing", "parent", "base64", "nested_required", "enum"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			c := mksClient(t, func(*http.Request) (*http.Response, error) { calls++; return inventoryReply(204, ""), nil })
			v := regionalView(t, c, "ru-9").MKS().V2
			var receipt *types.Receipt
			var e error
			switch kind {
			case "pool":
				s := clusterSpec(t)
				s.Pool = managementPtr("ru-1")
				r, err := v.Clusters.Create(context.Background(), s)
				receipt = r.Receipt
				e = err
			case "missing":
				r, err := v.Clusters.Create(context.Background(), mks.ClusterCreateSpec{})
				receipt = r.Receipt
				e = err
			case "parent":
				bad := group
				bad.Parent = managementPtr(cluster)
				bad.Parent.EndpointURL = "https://other.invalid/v2"
				r, err := v.Nodegroups.Delete(context.Background(), bad)
				receipt = r.Receipt
				e = err
			case "base64":
				zero, _ := types.ParseInteger("0")
				userdata := types.Protect([]byte("not-base64"))
				r, err := v.Nodegroups.Create(context.Background(), cluster, []mks.NodegroupCreateSpec{{Segment: managementPtr("ru-9a"), Count: &zero, UserData: &userdata}})
				receipt = r.Receipt
				e = err
			case "nested_required":
				r, err := v.Nodegroups.Replace(context.Background(), group, mks.NodegroupUpdateSpec{Taints: []mks.NodegroupTaint{{Effect: managementPtr("NoSchedule")}}})
				receipt = r.Receipt
				e = err
			case "enum":
				r, err := v.Nodegroups.Replace(context.Background(), group, mks.NodegroupUpdateSpec{Taints: []mks.NodegroupTaint{{Effect: managementPtr("UNKNOWN"), Key: managementPtr("key"), Value: managementPtr("")}}})
				receipt = r.Receipt
				e = err
			}
			if e == nil || receipt == nil || receipt.Outcome != types.NotSent || calls != 0 {
				t.Fatal("invalid MKS mutation sent", e)
			}
		})
	}
}

func TestMksTaskPaginationAndExactHierarchy(t *testing.T) {
	cluster := mksRef("cluster", "cluster-id", nil)
	calls := 0
	c := mksClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("offset") != fmt.Sprint(calls-1) {
			t.Error("task offset did not advance")
		}
		if calls == 1 {
			return inventoryReply(200, `{"count":2,"tasks":[`+mksTaskJSON+`]}`), nil
		}
		return inventoryReply(200, `{"count":2,"tasks":[`+strings.Replace(mksTaskJSON, `"task-id"`, `"second-task"`, 1)+`]}`), nil
	})
	result, e := regionalView(t, c, "ru-9").MKS().V2.Tasks.Collect(context.Background(), cluster, mks.TaskListOptions{}, pagination.Options{PageSize: 1})
	if e != nil || !result.Complete || len(result.Items) != 2 || result.ProviderPages[0].TotalCount.String() != "2" || result.Items[0].Ref.Parent.ID != cluster.ID {
		t.Fatal("task pagination lost provider metadata", e)
	}
	group := mksRef("nodegroup", "group-id", &cluster)
	c = mksClient(t, func(*http.Request) (*http.Response, error) {
		return inventoryReply(200, `{"nodegroup":`+strings.Replace(nodegroupJSON(false), `"cluster-id"`, `"foreign-cluster"`, 1)+`}`), nil
	})
	r, e := regionalView(t, c, "ru-9").MKS().V2.Nodegroups.Get(context.Background(), group)
	if types.ErrorCode(e) != types.ScopeMismatch || r.Data.Ref != nil {
		t.Fatal("foreign parent nodegroup accepted", e)
	}
}
