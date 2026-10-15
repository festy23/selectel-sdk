package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
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
