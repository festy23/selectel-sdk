package selectelsdk_test

import (
	"context"
	"encoding/json"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/onec"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOneCExplicitProjectDiscovery(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if r.Method != "GET" || r.URL.String() != "https://api.selectel.ru/ones/paas/v2/service/endpoints" || r.Header.Get("X-Auth-Token") == "" {
			t.Fatalf("unexpected discovery request %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"endpoints":[{"region":"ru-9","endpoints":[{"protocol":"https","endpoint":"https://api.ru-9.1c.selcloud.ru","type":"api"}]}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	out, err := p.OneC().Endpoints(context.Background(), types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(out.Data.Endpoints) != 1 || *out.Data.Endpoints[0].Region != "ru-9" || *out.Data.Endpoints[0].Endpoints[0].Type != "api" || out.Metadata.Scope != p.Scope() {
		t.Fatalf("discovery: %+v", out)
	}
}

func TestOneCRegionalClusterUsesExactOverrideAndRef(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
	endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.String() != "https://synthetic.onec.example/v2/clusters/cluster-1" {
			t.Fatalf("unexpected URL %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"cluster":{"id":"cluster-1","projectId":"test-project","spec":{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}},"state":{"status":"CLUSTER_STATUS_ACTIVE"}}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	regional, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: "cluster", ID: "cluster-1", EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
	out, err := regional.OneC().GetCluster(context.Background(), ref, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data.Cluster == nil || out.Data.Cluster.Ref == nil || *out.Data.Cluster.Ref != ref || *out.Data.Cluster.State.Status != "CLUSTER_STATUS_ACTIVE" {
		t.Fatalf("cluster %+v", out)
	}
}

func TestOneCRegionalEndpointRequiredBeforeIO(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	regional, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Scope: regional.Scope(), Service: "cloud_1c", APIGeneration: "v2", Kind: "cluster", ID: "cluster-1", EndpointID: "guessed", EndpointURL: "https://api.ru-3.1c.selcloud.ru/v2"}
	_, err = regional.OneC().GetCluster(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(err) != types.EndpointUnavailable || calls != 0 {
		t.Fatalf("endpoint gate: %v, calls %d", err, calls)
	}
}

func TestOneCClusterCreationAcceptedIdentitySurvivesBadFields(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
	endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v2/clusters" || !strings.Contains(string(body), `"enableStorageAutoScaling":false`) {
			t.Fatalf("create wire %s %s %s", r.Method, r.URL, body)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"cluster":{"id":"created-1","projectId":"test-project","spec":null,"createdAt":"bad-time"}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	regional, _ := p.Location("spb", "ru-9")
	var spec onec.CreateClusterRequest
	if json.Unmarshal([]byte(`{"spec":{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"},"enableStorageAutoScaling":false}}`), &spec) != nil {
		t.Fatal("bad fixture")
	}
	out, err := regional.OneC().CreateCluster(context.Background(), spec, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "created-1" || out.Receipt.ResourceRefs[0].Scope != scope {
		t.Fatalf("accepted identity result %+v, err %v", out, err)
	}
}

func TestOneCClusterMutationLocalFailuresHaveRevisionBoundReceipts(t *testing.T) {
	for _, mode := range []string{"missing-endpoint", "invalid-spec"} {
		t.Run(mode, func(t *testing.T) {
			calls, lookups := 0, 0
			c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
				lookups++
				return source().Lookup(ctx, r)
			}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
			if err != nil {
				t.Fatal(err)
			}
			p, _ := c.Project("test-project")
			regional, _ := p.Location("spb", "ru-9")
			var spec onec.CreateClusterRequest
			if mode == "missing-endpoint" {
				if json.Unmarshal([]byte(`{"spec":{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}}`), &spec) != nil {
					t.Fatal("fixture")
				}
			}
			out, err := regional.OneC().CreateCluster(context.Background(), spec, types.CallOptions{})
			want := types.InvalidArgument
			if mode == "missing-endpoint" {
				want = types.EndpointUnavailable
			}
			if types.ErrorCode(err) != want || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || out.Receipt.Attempts != 0 || lookups != 0 || calls != 0 {
				t.Fatalf("local result %+v, %v, lookups %d calls %d", out, err, lookups, calls)
			}
		})
	}
}

func TestOneCClusterUnknownCreationNeverReplays(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
	endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), ReadMaxAttempts: 5, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		return nil, io.ErrUnexpectedEOF
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	regional, _ := p.Location("spb", "ru-9")
	var spec onec.CreateClusterRequest
	if json.Unmarshal([]byte(`{"spec":{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.OneC().CreateCluster(context.Background(), spec, types.CallOptions{})
	if types.ErrorCode(err) != types.UnknownOutcome || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || out.Receipt.Attempts != 1 || len(out.Receipt.ResourceRefs) != 0 || calls != 1 {
		t.Fatalf("unknown result %+v, %v calls %d", out, err, calls)
	}
}
