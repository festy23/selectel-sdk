package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/mkslegacy"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMKSLegacyListRetainsV1WireScopeAndProtectedOptions(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if r.URL.String() != "https://ru-9.mks.selcloud.ru/v1/clusters" || r.Header.Get("X-Auth-Token") == "" {
			t.Fatalf("legacy request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"clusters":[{"id":"legacy-cluster","cni_type":"future-cni","project_id":"test-project","region":"ru-9","status":"future-state","kubernetes_options":{"oidc":{"ca_certs":"synthetic-private-ca"}}}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	out, e := v.MKSLegacy().ListClusters(context.Background(), pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.APIGeneration != "v1" || out.Items[0].Ref.Parent != nil || out.Items[0].Ref.Scope != v.Scope() || out.Items[0].Status == nil || *out.Items[0].Status != "future-state" || calls != 1 {
		t.Fatalf("legacy %+v %v", out, e)
	}
	safe, _ := json.Marshal(out)
	if strings.Contains(string(safe)+fmt.Sprintf("%+v %#v", out, out), "synthetic-private-ca") {
		t.Fatal("options leaked")
	}
}

func TestMKSLegacyGetRejectsV2RefBeforeIOAndKeepsExactV1Identity(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/v1/clusters/legacy-cluster" {
			t.Fatalf("path %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"cluster":{"id":"legacy-cluster","project_id":"test-project","region":"ru-9","cni_type":"CALICO","status":"ACTIVE"}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Service: "mks", APIGeneration: "v2", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v2:ru-9", EndpointURL: "https://ru-8.mks.selcloud.ru/v2"}
	_, e = v.MKSLegacy().GetCluster(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 0 {
		t.Fatalf("generation %v calls%d", e, calls)
	}
	ref.APIGeneration = "v1"
	ref.EndpointID = "selectel-mks.v1:ru-9"
	ref.EndpointURL = "https://ru-9.mks.selcloud.ru/v1"
	out, e := v.MKSLegacy().GetCluster(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Data.Ref == nil || *out.Data.Ref != ref || calls != 2 {
		t.Fatalf("get %+v %v", out, e)
	}
}

func TestMKSLegacyKubeVersionsKeepFalseAndLocalPartialBounds(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/v1/kubeversions" {
			t.Fatalf("path %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"kube_versions":[{"version":"1.30.1","is_default":false},{"version":"1.31.1","is_default":true}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	out, e := v.MKSLegacy().ListKubeVersions(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || out.Complete || out.NextCursor == nil || len(out.Items) != 1 || out.Items[0].IsDefault == nil || *out.Items[0].IsDefault {
		t.Fatalf("versions %+v %v", out, e)
	}
}

func TestMKSLegacyAdmissionControllersKeepNativeCapitalizedFields(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"admission_controllers":[{"KubeVersionMinor":"1.30","Names":["synthetic-controller"]}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	out, e := v.MKSLegacy().ListAdmissionControllers(context.Background(), pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].KubeVersionMinor == nil || *out.Items[0].KubeVersionMinor != "1.30" {
		t.Fatalf("admission %+v %v", out, e)
	}
}

func TestMKSLegacyFeatureGatesRemainSeparateConfigurationEndpoint(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/v1/feature-gates" {
			t.Fatalf("path %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"feature_gates":[{"KubeVersionMinor":"1.30","Names":["synthetic-feature"]}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	out, e := v.MKSLegacy().ListFeatureGates(context.Background(), pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || len(out.Items[0].Names) != 1 || out.Items[0].Names[0] != "synthetic-feature" {
		t.Fatalf("gates %+v %v", out, e)
	}
}

func TestMKSLegacyDeleteKeepsRevisionBoundNotSentAndUnknownReceipt(t *testing.T) {
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
	ref := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	wrong := ref
	wrong.APIGeneration = "v2"
	out, e := v.MKSLegacy().DeleteCluster(context.Background(), wrong, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || out.Receipt.Attempts != 0 || writes != 0 {
		t.Fatalf("not sent %+v %v", out, e)
	}
	out, e = v.MKSLegacy().DeleteCluster(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref || writes != 1 {
		t.Fatalf("unknown %+v %v", out, e)
	}
}

func TestMKSLegacyCreatePreservesAcceptedIdentityBeforeMalformedFields(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v1/clusters" || !strings.Contains(string(raw), `"zonal":false`) || !strings.Contains(string(raw), `"user_data":"dGVzdA=="`) {
			t.Fatalf("create %s %s", r.URL, raw)
		}
		return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"cluster":{"id":"created-v1-cluster","project_id":"test-project","region":"ru-9","cni_type":17}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	name, version, pool, zone := "synthetic-cluster", "1.30.1", "ru-9", "ru-9a"
	count, _ := types.ParseInteger("1")
	data := types.Protect([]byte("dGVzdA=="))
	zonal := false
	input := mkslegacy.ClusterCreateSpec{Name: &name, KubeVersion: &version, Region: &pool, Zonal: &zonal, Nodegroups: []mkslegacy.NodegroupCreate{{Count: &count, AvailabilityZone: &zone, UserData: &data}}}
	out, e := v.MKSLegacy().CreateCluster(context.Background(), input, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "created-v1-cluster" || out.Receipt.ResourceRefs[0].APIGeneration != "v1" || out.Receipt.ResourceRefs[0].Parent != nil {
		t.Fatalf("create %+v %v", out, e)
	}
	safe, _ := json.Marshal(input)
	if strings.Contains(string(safe)+fmt.Sprintf("%+v %#v", input, input), "dGVzdA==") {
		t.Fatal("user-data leaked")
	}
}

func TestMKSLegacyDeleteAcknowledgementIsBodylessAcceptance(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method != "DELETE" || r.URL.Path != "/v1/clusters/legacy-cluster" {
			t.Fatalf("delete %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	out, e := v.MKSLegacy().DeleteCluster(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("delete %+v %v", out, e)
	}
}

func TestMKSLegacyCreateRejectsScalarForSourceObjectBeforeIO(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	var software types.ProtectedJSON
	_ = json.Unmarshal([]byte(`"synthetic-not-an-object"`), &software)
	name, version, pool := "synthetic-cluster", "1.30.1", "ru-9"
	out, e := v.MKSLegacy().CreateCluster(context.Background(), mkslegacy.ClusterCreateSpec{Name: &name, KubeVersion: &version, Region: &pool, Nodegroups: []mkslegacy.NodegroupCreate{}, AdditionalSoftware: &software}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("object %+v %v calls%d", out, e, calls)
	}
}

func TestMKSLegacyUpdatePreservesExplicitFalseAndExactCallerIdentity(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || string(raw) != `{"cluster":{"enable_autorepair":false}}` {
			t.Fatalf("update %s %s", r.Method, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"cluster":{"id":"legacy-cluster","project_id":"test-project","region":"ru-9","cni_type":"CALICO"}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	enabled := false
	out, e := v.MKSLegacy().UpdateCluster(context.Background(), ref, mkslegacy.ClusterUpdateSpec{EnableAutorepair: &enabled}, types.CallOptions{})
	if e != nil || out.Data.Ref == nil || *out.Data.Ref != ref || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 {
		t.Fatalf("update %+v %v", out, e)
	}
}

func TestMKSLegacyUpgradeMinorUsesNativeActionAndAcceptedExactRef(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v1/clusters/legacy-cluster/upgrade-minor-version" || len(raw) != 0 {
			t.Fatalf("upgrade %s %s %s", r.Method, r.URL, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"cluster":{"id":"legacy-cluster","project_id":"test-project","region":"ru-9","cni_type":"CALICO","status":"PENDING_UPGRADE_MINOR_VERSION"}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	out, e := v.MKSLegacy().UpgradeMinorVersion(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Data.Status == nil || *out.Data.Status != "PENDING_UPGRADE_MINOR_VERSION" || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Receipt.ProviderOperationRef != nil {
		t.Fatalf("upgrade %+v %v", out, e)
	}
}

func TestMKSLegacyUpgradePatchRejectsForeignAcceptedClusterIdentity(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method != "POST" || r.URL.Path != "/v1/clusters/legacy-cluster/upgrade-patch-version" {
			t.Fatalf("patch %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"cluster":{"id":"foreign-cluster","project_id":"test-project","region":"ru-9","cni_type":"CALICO"}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	out, e := v.MKSLegacy().UpgradePatchVersion(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref || out.Data.Ref != nil {
		t.Fatalf("patch %+v %v", out, e)
	}
}

func TestMKSLegacyRotateCertificatesIsAcceptedWithoutInventedTask(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method != "POST" || r.URL.Path != "/v1/clusters/legacy-cluster/rotate-certs" {
			t.Fatalf("rotate %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	out, e := v.MKSLegacy().RotateCertificates(context.Background(), ref, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Receipt.ProviderOperationRef != nil || len(out.Receipt.ResourceRefs) != 1 {
		t.Fatalf("rotate %+v %v", out, e)
	}
}

func TestMKSLegacyNodegroupListKeepsExplicitClusterAndNativeNodeParents(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/v1/clusters/legacy-cluster/nodegroups" {
			t.Fatalf("nodegroups %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"nodegroups":[{"id":"group-1","cluster_id":"legacy-cluster","nodes":[{"id":"node-1","nodegroup_id":"group-1"}]}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	out, e := v.MKSLegacy().ListNodegroups(context.Background(), ref, pagination.Options{})
	if e != nil || !out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.Parent == nil || *out.Items[0].Ref.Parent != ref || len(out.Items[0].Nodes) != 1 || out.Items[0].Nodes[0].Ref == nil || out.Items[0].Nodes[0].Ref.Parent == nil || out.Items[0].Nodes[0].Ref.Parent.ID != "group-1" {
		t.Fatalf("nodegroups %+v %v", out, e)
	}
}

func TestMKSLegacyNodegroupGetChecksAvailableAncestorIdentity(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/v1/clusters/legacy-cluster/nodegroups/group-1" {
			t.Fatalf("group %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"nodegroup":{"id":"group-1","cluster_id":"foreign-cluster","nodes":[]}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	cluster := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	group := cluster
	group.ID = "group-1"
	group.Kind = "nodegroup"
	group.Parent = &cluster
	out, e := v.MKSLegacy().GetNodegroup(context.Background(), group, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Data.Ref != nil {
		t.Fatalf("group %+v %v", out, e)
	}
}

func TestMKSLegacyNodegroupCreateAcknowledgementNeverInfersNewGroup(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v1/clusters/legacy-cluster/nodegroups" || !strings.Contains(string(raw), `"count":1`) || !strings.Contains(string(raw), `"enable_autoscale":false`) {
			t.Fatalf("create group %s %s", r.URL, raw)
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	count, _ := types.ParseInteger("1")
	zone := "ru-9a"
	enabled := false
	out, e := v.MKSLegacy().CreateNodegroup(context.Background(), ref, mkslegacy.NodegroupCreate{Count: &count, AvailabilityZone: &zone, EnableAutoscale: &enabled}, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref || out.Receipt.ProviderOperationRef != nil {
		t.Fatalf("create group %+v %v", out, e)
	}
}

func TestMKSLegacyNodegroupUpdateRetainsExactParentOnAcceptedMalformedBody(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || r.URL.Path != "/v1/clusters/legacy-cluster/nodegroups/group-1" || !strings.Contains(string(raw), `"enable_autoscale":false`) {
			t.Fatalf("update group %s %s", r.URL, raw)
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"unexpected":"synthetic-private-body"}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	cluster := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	group := cluster
	group.ID = "group-1"
	group.Kind = "nodegroup"
	group.Parent = &cluster
	enabled := false
	out, e := v.MKSLegacy().UpdateNodegroup(context.Background(), group, mkslegacy.NodegroupUpdate{EnableAutoscale: &enabled}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].Parent == nil || out.Receipt.ResourceRefs[0].Parent.ID != "legacy-cluster" {
		t.Fatalf("update group %+v %v", out, e)
	}
}

func TestMKSLegacyNodegroupDeleteRejectsForeignAncestorsBeforeIO(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	cluster := types.ResourceRef{Service: "foreign-service", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	group := cluster
	group.Service = "mks"
	group.ID = "group-1"
	group.Kind = "nodegroup"
	group.Parent = &cluster
	out, e := v.MKSLegacy().DeleteNodegroup(context.Background(), group, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || calls != 0 {
		t.Fatalf("ancestor %+v %v calls%d", out, e, calls)
	}
	cluster.Service = "mks"
	cluster.Parent = &cluster
	_, e = v.MKSLegacy().DeleteNodegroup(context.Background(), group, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 0 {
		t.Fatalf("cycle %v calls%d", e, calls)
	}
}

func TestMKSLegacyNodegroupResizePreservesExactDesiredAmount(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v1/clusters/legacy-cluster/nodegroups/group-1/resize" || string(raw) != `{"nodegroup":{"desired":9007199254740993}}` {
			t.Fatalf("resize %s %s", r.URL, raw)
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	cluster := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	group := cluster
	group.ID = "group-1"
	group.Kind = "nodegroup"
	group.Parent = &cluster
	desired, _ := types.ParseInteger("9007199254740993")
	out, e := v.MKSLegacy().ResizeNodegroup(context.Background(), group, desired, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 {
		t.Fatalf("resize %+v %v", out, e)
	}
}

func TestMKSLegacyNodeGetUsesV1PathWithoutNodesSegment(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/v1/clusters/legacy-cluster/nodegroups/group-1/node-1" {
			t.Fatalf("node %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"node":{"id":"node-1","nodegroup_id":"group-1","os_server_id":"unadopted-os-server"}}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	cluster := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	group := cluster
	group.ID = "group-1"
	group.Kind = "nodegroup"
	group.Parent = &cluster
	node := group
	node.ID = "node-1"
	node.Kind = "node"
	node.Parent = &group
	out, e := v.MKSLegacy().GetNode(context.Background(), node, types.CallOptions{})
	if e != nil || out.Data.Ref == nil || out.Data.Ref.ID != "node-1" || out.Data.Ref.Parent == nil || out.Data.Ref.Parent.ID != "group-1" || out.Data.Ref.Parent.Parent == nil || out.Data.Ref.Parent.Parent.ID != "legacy-cluster" {
		t.Fatalf("node %+v %v", out, e)
	}
}

func TestMKSLegacyNodeDeleteUnknownKeepsClonedExactHierarchyWithoutReplay(t *testing.T) {
	writes := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		writes++
		if r.Method != "DELETE" || r.URL.Path != "/v1/clusters/legacy-cluster/nodegroups/group-1/node-1" {
			t.Fatalf("delete node %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"message":"synthetic-secret-provider-error"}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	cluster := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	group := cluster
	group.ID = "group-1"
	group.Kind = "nodegroup"
	group.Parent = &cluster
	node := group
	node.ID = "node-1"
	node.Kind = "node"
	node.Parent = &group
	out, e := v.MKSLegacy().DeleteNode(context.Background(), node, types.CallOptions{})
	cluster.ID = "caller-mutated-cluster"
	if types.ErrorCode(e) != types.UnknownOutcome || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].Parent.Parent.ID != "legacy-cluster" || writes != 1 || strings.Contains(fmt.Sprint(e), "synthetic-secret-provider-error") {
		t.Fatalf("delete node %+v %v writes%d", out, e, writes)
	}
}

func TestMKSLegacyNodeReinstallAcknowledgementKeepsOnlyExactNode(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Method != "POST" || r.URL.Path != "/v1/clusters/legacy-cluster/nodegroups/group-1/node-1/reinstall" {
			t.Fatalf("reinstall %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("test-project")
	v, _ := p.Location("spb", "ru-9")
	cluster := types.ResourceRef{Service: "mks", APIGeneration: "v1", Kind: "cluster", ID: "legacy-cluster", Scope: v.Scope(), EndpointID: "selectel-mks.v1:ru-9", EndpointURL: "https://ru-9.mks.selcloud.ru/v1"}
	group := cluster
	group.ID = "group-1"
	group.Kind = "nodegroup"
	group.Parent = &cluster
	node := group
	node.ID = "node-1"
	node.Kind = "node"
	node.Parent = &group
	out, e := v.MKSLegacy().ReinstallNode(context.Background(), node, types.CallOptions{})
	if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "node-1" || out.Receipt.ProviderOperationRef != nil {
		t.Fatalf("reinstall %+v %v", out, e)
	}
}
