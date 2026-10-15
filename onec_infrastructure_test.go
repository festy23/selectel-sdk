package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/onec"
	"github.com/festy23/selectel-sdk/pagination"
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

func TestOneCClusterListingPreservesBoundAndOpaqueContinuation(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
	endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if r.URL.Path != "/v2/clusters" || r.URL.Query().Get("cursor.limit") != "1" || r.URL.Query().Get("cursor.cursor") != "" {
			t.Fatalf("list wire %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"clusters":[{"id":"cluster-1","projectId":"test-project","spec":{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}}],"cursor":{"next":"50","total":"9007199254740993"}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	regional, _ := p.Location("spb", "ru-9")
	out, err := regional.OneC().ListClusters(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || calls != 1 || out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.NextCursor == nil || out.NextCursor.WireCursor != "50" || len(out.Envelopes) != 1 || *out.Envelopes[0].Cursor.Total != "9007199254740993" {
		t.Fatalf("list %+v, %v", out, err)
	}
}

func TestOneCClusterListingDoesNotInventTerminalSentinels(t *testing.T) {
	for _, payload := range []string{`{"clusters":[]}`, `{"clusters":[],"cursor":{"next":"0","total":"0"}}`} {
		t.Run(payload, func(t *testing.T) {
			scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
			endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
			calls := 0
			c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/identity/v3/auth/tokens" {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				calls++
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			p, _ := c.Project("test-project")
			regional, _ := p.Location("spb", "ru-9")
			out, err := regional.OneC().ListClusters(context.Background(), pagination.Options{MaxPages: 1})
			if err == nil || calls != 1 || out.Complete || len(out.Envelopes) != 1 {
				t.Fatalf("invented end: %+v %v calls %d", out, err, calls)
			}
		})
	}
}

func TestOneCClusterDeletionPreservesExactAcceptedReference(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
	endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		if r.Method != "DELETE" || r.URL.Path != "/v2/clusters/cluster-1" {
			t.Fatalf("delete wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"cluster":{"id":"cluster-1","projectId":"test-project","spec":{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}},"state":{"status":"CLUSTER_STATUS_DELETING"}}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	regional, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: "cluster", ID: "cluster-1", EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
	out, err := regional.OneC().DeleteCluster(context.Background(), ref, types.CallOptions{})
	if err != nil || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref || out.Data.Cluster == nil || out.Data.Cluster.Ref == nil || *out.Data.Cluster.Ref != ref || *out.Data.Cluster.State.Status != "CLUSTER_STATUS_DELETING" {
		t.Fatalf("delete result %+v %v calls%d", out, err, calls)
	}
}

func TestOneCClusterDeletionWaitUsesSourceStateAndReceiptGoal(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
	endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
	writes, reads := 0, 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), PollInterval: time.Millisecond, EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.URL.Path != "/v2/clusters/cluster-1" {
			t.Fatalf("unexpected wait path %s", r.URL)
		}
		state := "CLUSTER_STATUS_DELETING"
		switch r.Method {
		case "DELETE":
			writes++
		case "GET":
			reads++
			if reads > 1 {
				state = "CLUSTER_STATUS_DELETED"
			}
		default:
			t.Fatalf("unexpected method%s", r.Method)
		}
		payload := `{"cluster":{"id":"cluster-1","projectId":"test-project","spec":{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}},"state":{"status":"` + state + `"}}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	regional, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: "cluster", ID: "cluster-1", EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
	accepted, err := regional.OneC().DeleteCluster(context.Background(), ref, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.Resume(*accepted.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Wait(context.Background(), "cluster.active")
	if types.ErrorCode(err) != types.CapabilityUnavailable || writes != 1 || reads != 0 {
		t.Fatalf("invalid receipt goal: %v writes%d reads%d", err, writes, reads)
	}
	waited, err := h.Wait(context.Background(), "cluster.deleted")
	if err != nil || !waited.Complete || waited.Receipt == nil || waited.Receipt.CallID != accepted.Receipt.CallID || waited.Last == nil || waited.Last.ProviderState != "CLUSTER_STATUS_DELETED" || reads != 2 || writes != 1 {
		t.Fatalf("wait %+v %v writes%d reads%d", waited, err, writes, reads)
	}
}

func TestOneCClusterObservationPreservesUnknownAndMissingStates(t *testing.T) {
	for _, tc := range []struct{ stateJSON, raw string }{{``, ""}, {`,"state":null`, ""}, {`,"state":{"status":"FUTURE_PROVIDER_STATE"}`, "FUTURE_PROVIDER_STATE"}, {`,"state":{"status":"CLUSTER_STATUS_UNSPECIFIED"}`, "CLUSTER_STATUS_UNSPECIFIED"}} {
		t.Run(tc.raw+tc.stateJSON, func(t *testing.T) {
			scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
			endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
			calls := 0
			c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/identity/v3/auth/tokens" {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				calls++
				payload := `{"cluster":{"id":"cluster-1","projectId":"test-project","spec":{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}` + tc.stateJSON + `}}`
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			ref := types.ResourceRef{Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: "cluster", ID: "cluster-1", EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
			h, err := c.Resource(ref)
			if err != nil || calls != 0 {
				t.Fatalf("handle creation %v calls%d", err, calls)
			}
			status, err := h.Status(context.Background())
			if err != nil || status.ProviderState != tc.raw || status.Classification != "unknown" {
				t.Fatalf("observation %+v %v", status, err)
			}
			waited, err := h.Wait(context.Background(), "cluster.active")
			if types.ErrorCode(err) != types.StateUnknown || waited.Complete || waited.Receipt != nil {
				t.Fatalf("unknown wait %+v %v", waited, err)
			}
		})
	}
}

func TestOneCClusterPatchSendsRawSpecWithExplicitFalseAndEmptyAllowList(t *testing.T) {
	scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
	endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		calls++
		raw, _ := io.ReadAll(r.Body)
		var body map[string]json.RawMessage
		if r.Method != "PATCH" || r.URL.Path != "/v2/clusters/cluster-1" || json.Unmarshal(raw, &body) != nil || body["spec"] != nil || string(body["enableStorageAutoScaling"]) != "false" || !strings.Contains(string(body["config"]), `"webClientIpAllowList":[]`) {
			t.Fatalf("patch wire %s %s %s", r.Method, r.URL, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"cluster":{"id":"cluster-1","projectId":"test-project","spec":` + string(raw) + `}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	regional, _ := p.Location("spb", "ru-9")
	ref := types.ResourceRef{Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: "cluster", ID: "cluster-1", EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
	var spec onec.ClusterSpec
	if json.Unmarshal([]byte(`{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"},"enableStorageAutoScaling":false,"config":{"webClientIpAllowList":[]}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.OneC().PatchCluster(context.Background(), ref, spec, types.CallOptions{})
	if err != nil || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Data.Cluster == nil || out.Data.Cluster.Spec.EnableStorageAutoScaling == nil || *out.Data.Cluster.Spec.EnableStorageAutoScaling || out.Data.Cluster.Spec.Config.WebClientIPAllowList == nil {
		t.Fatalf("patch result %+v %v calls%d", out, err, calls)
	}
}

const onecClusterFixture = `{"cluster":{"id":"cluster-1","projectId":"test-project","spec":{"name":"test","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}},"state":{"status":"CLUSTER_STATUS_ACTIVE"}}}`

func onecRegionalFixture(t *testing.T, wire transportFunc) (*sdk.Client, *onec.Client, types.ResourceRef) {
	t.Helper()
	scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
	endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return wire(r)
	})})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := c.Project("test-project")
	regional, _ := p.Location("spb", "ru-9")
	return c, regional.OneC(), types.ResourceRef{Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: "cluster", ID: "cluster-1", EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
}

func TestOneCClusterRestartHasNoInventedRequestBody(t *testing.T) {
	calls := 0
	_, regional, ref := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v2/clusters/cluster-1/restart" || len(raw) != 0 {
			t.Fatalf("restart wire %s %s %s", r.Method, r.URL, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecClusterFixture))}, nil
	}))
	out, err := regional.RestartCluster(context.Background(), ref, types.CallOptions{})
	if err != nil || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("restart result %+v %v calls%d", out, err, calls)
	}
}

func TestOneCClusterConfigChangeUsesDocumentedWrapperAndValues(t *testing.T) {
	_, regional, ref := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v2/clusters/cluster-1/config" || string(raw) != `{"config":{"debugMode":false,"webClientIpAllowList":[],"webServerEnabled":false}}` {
			t.Fatalf("config wire %s %s %s", r.Method, r.URL, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecClusterFixture))}, nil
	}))
	var spec onec.ChangeClusterConfigBody
	if json.Unmarshal([]byte(`{"config":{"webServerEnabled":false,"debugMode":false,"webClientIpAllowList":[]}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.ChangeClusterConfig(context.Background(), ref, spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("config result %+v %v", out, err)
	}
}

func TestOneCSourceIntegerStringsAreValidatedBeforeMutationIO(t *testing.T) {
	calls := 0
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecClusterFixture))}, nil
	}))
	var spec onec.CreateClusterRequest
	if json.Unmarshal([]byte(`{"spec":{"name":"test","platform":{},"flavor":{"vCpu":"2.5","ramMb":"4096","diskGb":"20"}}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.CreateCluster(context.Background(), spec, types.CallOptions{})
	if types.ErrorCode(err) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || calls != 0 {
		t.Fatalf("source integer gate %+v %v calls%d", out, err, calls)
	}
}

func TestOneCClusterResizePreservesSourceIntegerStrings(t *testing.T) {
	_, regional, ref := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v2/clusters/cluster-1/resize" || string(raw) != `{"flavor":{"diskGb":"9007199254740993","ramMb":"4096","vCpu":"2"}}` {
			t.Fatalf("resize wire %s %s %s", r.Method, r.URL, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecClusterFixture))}, nil
	}))
	var spec onec.ResizeClusterBody
	if json.Unmarshal([]byte(`{"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"9007199254740993"}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.ResizeCluster(context.Background(), ref, spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("resize result %+v %v", out, err)
	}
}

func TestOneCClusterUpgradeSendsExplicitPlatformVersion(t *testing.T) {
	_, regional, ref := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v2/clusters/cluster-1/upgrade" || string(raw) != `{"platform":{"version":"8.3.synthetic"}}` {
			t.Fatalf("upgrade wire %s %s %s", r.Method, r.URL, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecClusterFixture))}, nil
	}))
	var spec onec.UpgradeClusterBody
	if json.Unmarshal([]byte(`{"platform":{"version":"8.3.synthetic"}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.UpgradeCluster(context.Background(), ref, spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("upgrade result %+v %v", out, err)
	}
}

func TestOneCClusterFlavorDiscoveryBoundsAndResumesFiniteMetadata(t *testing.T) {
	calls := 0
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/v2/cluster-flavors" || r.URL.RawQuery != "includeDeprecated=false" {
			t.Fatalf("flavor wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"flavors":[{"id":"flavor-1","vCpu":"2","ramMb":"4096","diskGb":"20"},{"id":"flavor-2","vCpu":"4","ramMb":"8192","diskGb":"40"}]}`))}, nil
	}))
	deprecated := false
	first, err := regional.ListClusterFlavors(context.Background(), &deprecated, pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || first.Complete || len(first.Items) != 1 || *first.Items[0].ID != "flavor-1" || first.NextCursor == nil || first.NextCursor.Skip != 1 || calls != 1 {
		t.Fatalf("first flavor page %+v %v", first, err)
	}
	second, err := regional.ListClusterFlavors(context.Background(), &deprecated, pagination.Options{MaxItems: 1, Cursor: first.NextCursor})
	if err != nil || !second.Complete || len(second.Items) != 1 || *second.Items[0].ID != "flavor-2" || second.NextCursor != nil || calls != 2 {
		t.Fatalf("second flavor page %+v %v", second, err)
	}
}

func TestOneCClusterPlatformDiscoveryPreservesDeprecatedMetadata(t *testing.T) {
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/cluster-platforms" || r.URL.RawQuery != "includeDeprecated=true" {
			t.Fatalf("platform wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"platforms":[{"version":"8.3.synthetic-new","deprecated":false},{"version":"8.3.synthetic-old","deprecated":true}]}`))}, nil
	}))
	deprecated := true
	out, err := regional.ListClusterPlatforms(context.Background(), &deprecated, pagination.Options{})
	if err != nil || !out.Complete || len(out.Items) != 2 || *out.Items[0].Version != "8.3.synthetic-new" || *out.Items[0].Deprecated || !*out.Items[1].Deprecated {
		t.Fatalf("platforms %+v %v", out, err)
	}
}

func TestOneCClusterFlavorConstraintsUseTypedExactLimits(t *testing.T) {
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/cluster-flavors/constraints" || r.URL.RawQuery != "" {
			t.Fatalf("constraints wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"constraints":{"vCpu":{"min":"2","max":"128"},"diskGb":{"min":"20","max":"9007199254740993"}}}`))}, nil
	}))
	out, err := regional.GetClusterFlavorConstraints(context.Background(), types.CallOptions{})
	if err != nil || out.Data.Constraints == nil || out.Data.Constraints.DiskGb == nil || *out.Data.Constraints.DiskGb.Max != "9007199254740993" {
		t.Fatalf("constraints %+v %v", out, err)
	}
}

func TestOneCRegionalDiscoveryReturnsAllRegionMetadataWithoutAdoption(t *testing.T) {
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/service/regions" {
			t.Fatalf("regions wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"regions":[{"regionName":"Synthetic SPB","regionCode":"ru-9","regionPool":"ru-9"},{"regionName":"Synthetic NSK","regionCode":"ru-8","regionPool":"ru-8"}]}`))}, nil
	}))
	out, err := regional.Regions(context.Background(), pagination.Options{})
	if err != nil || !out.Complete || len(out.Items) != 2 || *out.Items[0].RegionCode != "ru-9" || *out.Items[1].RegionPool != "ru-8" {
		t.Fatalf("regions %+v %v", out, err)
	}
}

func TestOneCServiceConfigIsMetadataAndDoesNotSelectDefaultRegion(t *testing.T) {
	calls := 0
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/v2/service/config" {
			t.Fatalf("service config wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"config":{"defaultRegion":"ru-8","regions":[{"regionCode":"ru-8","regionPool":"ru-8"}],"endpoints":[{"region":"ru-8","endpoints":[{"protocol":"https","endpoint":"https://unselected.synthetic.example","type":"api"}]}]}}`))}, nil
	}))
	out, err := regional.GetServiceConfig(context.Background(), types.CallOptions{})
	if err != nil || calls != 1 || out.Data.Config == nil || *out.Data.Config.DefaultRegion != "ru-8" || len(out.Data.Config.Endpoints) != 1 || out.Metadata.Scope.Pool != "ru-9" {
		t.Fatalf("service config %+v %v calls%d", out, err, calls)
	}
}

const onecDatastoreFixture = `{"datastore":{"id":"store-1","projectId":"test-project","spec":{"name":"store","flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"},"nodeCount":"1","enableStorageAutoScaling":false},"state":{"status":"DATASTORE_STATUS_ACTIVE"}}}`

func TestOneCDatastoreReadHasIndependentScopedIdentity(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/datastores/store-1" {
			t.Fatalf("datastore wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecDatastoreFixture))}, nil
	}))
	ref := cluster
	ref.Kind = "datastore"
	ref.ID = "store-1"
	out, err := regional.GetDatastore(context.Background(), ref, types.CallOptions{})
	if err != nil || out.Data.Datastore == nil || out.Data.Datastore.Ref == nil || *out.Data.Datastore.Ref != ref || *out.Data.Datastore.Spec.NodeCount != "1" || *out.Data.Datastore.Spec.EnableStorageAutoScaling || *out.Data.Datastore.State.Status != "DATASTORE_STATUS_ACTIVE" {
		t.Fatalf("datastore %+v %v", out, err)
	}
}

func TestOneCDatastoreCreationDoesNotInferClusterParentOrDefaults(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v2/datastores" || string(raw) != `{"spec":{"enableStorageAutoScaling":false,"flavor":{"diskGb":"20","ramMb":"4096","vCpu":"2"},"name":"store"}}` {
			t.Fatalf("create datastore wire %s %s %s", r.Method, r.URL, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecDatastoreFixture))}, nil
	}))
	var spec onec.CreateDatastoreRequest
	if json.Unmarshal([]byte(`{"spec":{"name":"store","flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"},"enableStorageAutoScaling":false}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.CreateDatastore(context.Background(), spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].Kind != "datastore" || out.Receipt.ResourceRefs[0].ID != "store-1" || out.Receipt.ResourceRefs[0].Parent != nil || out.Receipt.ResourceRefs[0].Scope != cluster.Scope {
		t.Fatalf("create datastore %+v %v", out, err)
	}
}

func TestOneCDatastoreDeletionWaitKeepsIndependentIdentity(t *testing.T) {
	writes, reads := 0, 0
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/datastores/store-1" {
			t.Fatalf("datastore delete wire %s", r.URL)
		}
		state := "DATASTORE_STATUS_DELETING"
		switch r.Method {
		case "DELETE":
			writes++
		case "GET":
			reads++
			state = "DATASTORE_STATUS_DELETED"
		default:
			t.Fatalf("unexpected method%s", r.Method)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Replace(onecDatastoreFixture, "DATASTORE_STATUS_ACTIVE", state, 1)))}, nil
	}))
	ref := cluster
	ref.Kind = "datastore"
	ref.ID = "store-1"
	out, err := regional.DeleteDatastore(context.Background(), ref, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("delete datastore %+v", out)
	}
	h, err := c.Resume(*out.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Wait(context.Background(), "datastore.active")
	if types.ErrorCode(err) != types.CapabilityUnavailable || writes != 1 || reads != 0 {
		t.Fatalf("foreign goal %v writes%d reads%d", err, writes, reads)
	}
	waited, err := h.Wait(context.Background(), "datastore.deleted")
	if err != nil || !waited.Complete || waited.Last == nil || waited.Last.Ref != ref || waited.Last.ProviderState != "DATASTORE_STATUS_DELETED" || writes != 1 || reads != 1 {
		t.Fatalf("datastore wait %+v %v writes%d reads%d", waited, err, writes, reads)
	}
}

func TestOneCReceiptCannotObserveDifferentResourceKindInSameService(t *testing.T) {
	calls := 0
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecDatastoreFixture))}, nil
	}))
	ref := cluster
	ref.Kind = "datastore"
	ref.ID = "store-1"
	accepted, err := regional.DeleteDatastore(context.Background(), ref, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	forged := *accepted.Receipt
	forged.ResourceRefs = append([]types.ResourceRef{}, forged.ResourceRefs...)
	forged.ResourceRefs[0].Kind = "cluster"
	_, err = c.Resume(forged)
	if types.ErrorCode(err) != types.CapabilityUnavailable || calls != 1 {
		t.Fatalf("cross-kind receipt %v calls%d", err, calls)
	}
}

func TestOneCDatastorePatchUsesRawSpecAndExactNodeCount(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PATCH" || r.URL.Path != "/v2/datastores/store-1" || string(raw) != `{"enableStorageAutoScaling":false,"flavor":{"diskGb":"20","ramMb":"4096","vCpu":"2"},"name":"store","nodeCount":"2"}` {
			t.Fatalf("datastore patch wire %s %s %s", r.Method, r.URL, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecDatastoreFixture))}, nil
	}))
	ref := cluster
	ref.Kind = "datastore"
	ref.ID = "store-1"
	var spec onec.DatastoreSpec
	if json.Unmarshal([]byte(`{"name":"store","flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"},"enableStorageAutoScaling":false,"nodeCount":"2"}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.PatchDatastore(context.Background(), ref, spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("patch datastore %+v %v", out, err)
	}
}

func TestOneCDatastoreResizeUsesOwnSourceFlavorShape(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/v2/datastores/store-1/resize" || string(raw) != `{"flavor":{"diskGb":"40","ramMb":"8192","vCpu":"4"}}` {
			t.Fatalf("datastore resize wire %s %s %s", r.Method, r.URL, raw)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecDatastoreFixture))}, nil
	}))
	ref := cluster
	ref.Kind = "datastore"
	ref.ID = "store-1"
	var spec onec.ResizeDatastoreBody
	if json.Unmarshal([]byte(`{"flavor":{"vCpu":"4","ramMb":"8192","diskGb":"40"}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.ResizeDatastore(context.Background(), ref, spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("resize datastore %+v %v", out, err)
	}
}

func TestOneCDatastoreFlavorDiscoveryPreservesAbsentFilter(t *testing.T) {
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/datastore-flavors" || r.URL.RawQuery != "" {
			t.Fatalf("datastore flavors wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"flavors":[{"id":"store-flavor","vCpu":"2","ramMb":"4096","diskGb":"20","deprecated":false}]}`))}, nil
	}))
	out, err := regional.ListDatastoreFlavors(context.Background(), nil, pagination.Options{})
	if err != nil || !out.Complete || len(out.Items) != 1 || *out.Items[0].ID != "store-flavor" || *out.Items[0].Deprecated {
		t.Fatalf("datastore flavors %+v %v", out, err)
	}
}

func TestOneCDatastoreFlavorConstraintsPreserveOptionalBounds(t *testing.T) {
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/datastore-flavors/constraints" {
			t.Fatalf("datastore constraints wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"constraints":{"vCpu":{"min":"2","max":"128"},"ramMb":{"min":"4096"}}}`))}, nil
	}))
	out, err := regional.GetDatastoreFlavorConstraints(context.Background(), types.CallOptions{})
	if err != nil || out.Data.Constraints == nil || out.Data.Constraints.RAMMb == nil || *out.Data.Constraints.RAMMb.Min != "4096" || out.Data.Constraints.RAMMb.Max != nil {
		t.Fatalf("datastore constraints %+v %v", out, err)
	}
}

func TestOneCDatastoreTypeDiscoveryPreservesEngineAndVersion(t *testing.T) {
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/datastore-types" || r.URL.RawQuery != "" {
			t.Fatalf("datastore types wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"types":[{"id":"pg1c","engine":"postgresql","version":"14-1C"}]}`))}, nil
	}))
	out, err := regional.ListDatastoreTypes(context.Background(), pagination.Options{})
	if err != nil || !out.Complete || len(out.Items) != 1 || *out.Items[0].ID != "pg1c" || *out.Items[0].Engine != "postgresql" || *out.Items[0].Version != "14-1C" {
		t.Fatalf("datastore types %+v %v", out, err)
	}
}

func TestOneCDatastoreListingPreservesPartialCursorAndScopedRefs(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/datastores" || r.URL.Query().Get("cursor.limit") != "1" {
			t.Fatalf("datastores list wire %s %s", r.Method, r.URL)
		}
		payload := `{"datastores":[{"id":"store-1","projectId":"test-project","spec":{"name":"store","flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}}],"cursor":{"next":"50","total":"2"}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	out, err := regional.ListDatastores(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.Scope != cluster.Scope || out.Items[0].Ref.Parent != nil || out.Items[0].Ref.Kind != "datastore" || out.NextCursor == nil || out.NextCursor.WireCursor != "50" || len(out.Envelopes) != 1 || *out.Envelopes[0].Cursor.Total != "2" {
		t.Fatalf("datastores partial %+v %v", out, err)
	}
}

func TestOneCDatastoreMaintenancePreservesParentAndProtectedDescription(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/datastores/store-1/maintenance" || r.URL.Query().Get("cursor.limit") != "1" {
			t.Fatalf("datastore maintenance wire %s %s", r.Method, r.URL)
		}
		payload := `{"events":[{"id":"maintenance-1","projectId":"test-project","spec":{"datastoreId":"store-1","description":"synthetic-maintenance-private","tasks":["DATASTORE_MNT_TASK_STORAGE_AUTO_SCALING"],"scheduledFor":"2026-10-15T14:00:00+03:00"}}],"cursor":{"next":"20","total":"5"}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	ref := cluster
	ref.Kind = "datastore"
	ref.ID = "store-1"
	out, err := regional.ListDatastoreMaintenanceEvents(context.Background(), ref, pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.Parent == nil || *out.Items[0].Ref.Parent != ref || out.Items[0].Spec == nil || string(out.Items[0].Spec.Description.Reveal()) != "synthetic-maintenance-private" || out.Items[0].Spec.ScheduledFor.Location() != time.UTC {
		t.Fatalf("maintenance %+v %v", out, err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-maintenance-private") || strings.Contains(fmt.Sprintf("%+v %#v", out, out), "synthetic-maintenance-private") {
		t.Fatal("maintenance description exposed")
	}
}

func TestOneCRefEndpointsAreRejectedBeforeCredentialOrHTTPIO(t *testing.T) {
	var clusterSpec onec.ClusterSpec
	var datastoreSpec onec.DatastoreSpec
	var config onec.ChangeClusterConfigBody
	var resize onec.ResizeClusterBody
	var upgrade onec.UpgradeClusterBody
	var datastoreResize onec.ResizeDatastoreBody
	fixtures := []struct {
		raw    string
		target any
	}{
		{`{"name":"cluster","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}`, &clusterSpec},
		{`{"name":"store","flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}`, &datastoreSpec},
		{`{"config":{}}`, &config}, {`{"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}`, &resize},
		{`{"platform":{"version":"8.3.synthetic"}}`, &upgrade}, {`{"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}`, &datastoreResize},
	}
	for _, f := range fixtures {
		if json.Unmarshal([]byte(f.raw), f.target) != nil {
			t.Fatal("fixture")
		}
	}
	tests := []struct {
		name, kind string
		mutation   bool
		call       func(*onec.Client, types.ResourceRef) (*types.Receipt, error)
	}{
		{"cluster-get", "cluster", false, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			_, e := c.GetCluster(context.Background(), r, types.CallOptions{})
			return nil, e
		}},
		{"cluster-delete", "cluster", true, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			o, e := c.DeleteCluster(context.Background(), r, types.CallOptions{})
			return o.Receipt, e
		}},
		{"cluster-patch", "cluster", true, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			o, e := c.PatchCluster(context.Background(), r, clusterSpec, types.CallOptions{})
			return o.Receipt, e
		}},
		{"cluster-config", "cluster", true, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			o, e := c.ChangeClusterConfig(context.Background(), r, config, types.CallOptions{})
			return o.Receipt, e
		}},
		{"cluster-restart", "cluster", true, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			o, e := c.RestartCluster(context.Background(), r, types.CallOptions{})
			return o.Receipt, e
		}},
		{"cluster-resize", "cluster", true, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			o, e := c.ResizeCluster(context.Background(), r, resize, types.CallOptions{})
			return o.Receipt, e
		}},
		{"cluster-upgrade", "cluster", true, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			o, e := c.UpgradeCluster(context.Background(), r, upgrade, types.CallOptions{})
			return o.Receipt, e
		}},
		{"datastore-get", "datastore", false, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			_, e := c.GetDatastore(context.Background(), r, types.CallOptions{})
			return nil, e
		}},
		{"datastore-delete", "datastore", true, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			o, e := c.DeleteDatastore(context.Background(), r, types.CallOptions{})
			return o.Receipt, e
		}},
		{"datastore-patch", "datastore", true, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			o, e := c.PatchDatastore(context.Background(), r, datastoreSpec, types.CallOptions{})
			return o.Receipt, e
		}},
		{"datastore-resize", "datastore", true, func(c *onec.Client, r types.ResourceRef) (*types.Receipt, error) {
			o, e := c.ResizeDatastore(context.Background(), r, datastoreResize, types.CallOptions{})
			return o.Receipt, e
		}},
	}
	for _, tc := range tests {
		for _, dimension := range []string{"id", "url"} {
			t.Run(tc.name+"/"+dimension, func(t *testing.T) {
				scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
				endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
				lookups, calls := 0, 0
				c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
					lookups++
					return source().Lookup(ctx, r)
				}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
				if e != nil {
					t.Fatal(e)
				}
				p, _ := c.Project("test-project")
				regional, _ := p.Location("spb", "ru-9")
				ref := types.ResourceRef{Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: tc.kind, ID: "exact-id", EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
				if dimension == "id" {
					ref.EndpointID = "foreign-endpoint"
				} else {
					ref.EndpointURL += "/foreign"
				}
				receipt, e := tc.call(regional.OneC(), ref)
				if types.ErrorCode(e) != types.EndpointUnavailable || lookups != 0 || calls != 0 {
					t.Fatalf("endpoint gate %v lookups%d calls%d", e, lookups, calls)
				}
				if tc.mutation && (receipt == nil || receipt.Outcome != types.NotSent || receipt.ContractRevision == "" || receipt.Attempts != 0) {
					t.Fatalf("endpoint receipt %+v", receipt)
				}
			})
		}
	}
}

func TestOneCDatastoreHealthTelemetryNumbersStayProtected(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		payload := strings.Replace(onecDatastoreFixture, `"status":"DATASTORE_STATUS_ACTIVE"`, `"status":"DATASTORE_STATUS_ACTIVE","health":{"issues":[{"metricThresholdBreach":{"metric":{"value":987654321.125},"threshold":123456789.625}}]}`, 1)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	ref := cluster
	ref.Kind = "datastore"
	ref.ID = "store-1"
	out, err := regional.GetDatastore(context.Background(), ref, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	breach := out.Data.Datastore.State.Health.Issues[0].MetricThresholdBreach
	if string(breach.Metric.Value.Reveal()) != "987654321.125" || string(breach.Threshold.Reveal()) != "123456789.625" {
		t.Fatal("protected number changed source precision")
	}
	raw, _ := json.Marshal(out)
	diagnostic := fmt.Sprintf("%+v %#v %+v %#v %x", out, out, *breach.Metric.Value, *breach.Threshold, *breach.Metric.Value)
	for _, marker := range []string{"987654321.125", "123456789.625"} {
		if strings.Contains(diagnostic, fmt.Sprint([]byte(marker))) || strings.Contains(diagnostic, fmt.Sprintf("%x", []byte(marker))) {
			t.Fatal("encoded telemetry leaked")
		}
		if strings.Contains(string(raw), marker) || strings.Contains(diagnostic, marker) {
			t.Fatal("health telemetry leaked")
		}
	}
}

func TestOneCNetworkSharingProjectIsTargetMetadataNotCallerScope(t *testing.T) {
	calls := 0
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/v2/cluster-proxies/network-share/project" {
			t.Fatalf("network sharing wire %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"projectId":"provider-service-project"}`))}, nil
	}))
	out, err := regional.GetProjectIDForNetworkSharing(context.Background(), types.CallOptions{})
	if err != nil || calls != 1 || out.Data.ProjectID == nil || *out.Data.ProjectID != "provider-service-project" || out.Metadata.Scope != cluster.Scope {
		t.Fatalf("sharing target %+v %v", out, err)
	}
}

const onecProxyFixture = `{"clusterProxy":{"id":"proxy-1","projectId":"test-project","spec":{"clusterId":"cluster-1","publicAccess":false,"networkId":"network-1","subnetId":"subnet-1"},"state":{"status":"CLUSTER_PROXY_STATUS_ACTIVE"},"cluster":{"id":"cluster-1","projectId":"test-project","spec":{"name":"cluster","platform":{},"flavor":{"vCpu":"2","ramMb":"4096","diskGb":"20"}}}}}`

func TestOneCProxyGetOnlyKeepsCallerProvidedClusterParent(t *testing.T) {
	for _, provided := range []bool{false, true} {
		t.Run(fmt.Sprint(provided), func(t *testing.T) {
			_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.URL.Path != "/v2/cluster-proxies/proxy-1" {
					t.Fatalf("proxy wire %s %s", r.Method, r.URL)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecProxyFixture))}, nil
			}))
			ref := cluster
			ref.Kind = "cluster_proxy"
			ref.ID = "proxy-1"
			if provided {
				ref.Parent = &cluster
			}
			out, err := regional.GetClusterProxy(context.Background(), ref, types.CallOptions{})
			if err != nil || out.Data.ClusterProxy == nil || out.Data.ClusterProxy.Ref == nil {
				t.Fatalf("proxy %+v %v", out, err)
			}
			got := out.Data.ClusterProxy.Ref
			if provided {
				if got.Parent == nil || *got.Parent != cluster || out.Data.ClusterProxy.Cluster.Ref == nil || *out.Data.ClusterProxy.Cluster.Ref != cluster {
					t.Fatal("caller parent lost")
				}
			} else if got.Parent != nil || out.Data.ClusterProxy.Cluster.Ref != nil {
				t.Fatal("parent inferred from provider metadata")
			}
		})
	}
}

func TestOneCProxyAncestorsAreValidatedBeforeIO(t *testing.T) {
	for _, tc := range []struct {
		name string
		want types.Code
	}{{"cycle", types.InvalidArgument}, {"service", types.ScopeMismatch}, {"generation", types.ScopeMismatch}, {"scope", types.ScopeMismatch}, {"endpoint", types.EndpointUnavailable}} {
		t.Run(tc.name, func(t *testing.T) {
			scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
			endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
			lookups, calls := 0, 0
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
				lookups++
				return source().Lookup(ctx, r)
			}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
			if e != nil {
				t.Fatal(e)
			}
			p, _ := c.Project("test-project")
			regional, _ := p.Location("spb", "ru-9")
			parent := types.ResourceRef{Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: "cluster", ID: "cluster-1", EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
			ref := parent
			ref.Kind = "cluster_proxy"
			ref.ID = "proxy-1"
			ref.Parent = &parent
			switch tc.name {
			case "cycle":
				ref.Parent = &ref
			case "service":
				parent.Service = "compute"
			case "generation":
				parent.APIGeneration = "v1"
			case "scope":
				parent.Scope.ProjectID = "foreign-project"
			case "endpoint":
				parent.EndpointURL += "/foreign"
			}
			_, e = regional.OneC().GetClusterProxy(context.Background(), ref, types.CallOptions{})
			if types.ErrorCode(e) != tc.want || lookups != 0 || calls != 0 {
				t.Fatalf("ancestor gate %v lookups%d calls%d", e, lookups, calls)
			}
		})
	}
}

func TestOneCProxyCreationRetainsExactParentAndResumesOnlyCreatedProxy(t *testing.T) {
	writes, reads := 0, 0
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		switch r.Method {
		case "POST":
			writes++
			raw, _ := io.ReadAll(r.Body)
			if r.URL.Path != "/v2/cluster-proxies" || string(raw) != `{"spec":{"clusterId":"cluster-1","networkId":"network-1","publicAccess":false,"subnetId":"subnet-1"}}` {
				t.Fatalf("create proxy wire %s %s", r.URL, raw)
			}
		case "GET":
			reads++
			if r.URL.Path != "/v2/cluster-proxies/proxy-1" {
				t.Fatalf("proxy recovery read %s", r.URL)
			}
		default:
			t.Fatalf("unexpected method%s", r.Method)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecProxyFixture))}, nil
	}))
	var spec onec.CreateClusterProxyRequest
	if json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1","publicAccess":false,"networkId":"network-1","subnetId":"subnet-1"}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	accepted, err := regional.CreateClusterProxy(context.Background(), cluster, spec, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Receipt == nil || accepted.Receipt.Outcome != types.Accepted || len(accepted.Receipt.ResourceRefs) != 2 || accepted.Data.ClusterProxy == nil || accepted.Data.ClusterProxy.Ref == nil || accepted.Data.ClusterProxy.Ref.Parent == nil || *accepted.Data.ClusterProxy.Ref.Parent != cluster {
		t.Fatalf("created proxy %+v", accepted)
	}
	h, err := c.Resume(*accepted.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if h.Resource().Kind != "cluster_proxy" || h.Resource().ID != "proxy-1" || writes != 1 || reads != 0 {
		t.Fatal("wrong recovery target or implicit I/O")
	}
	waited, err := h.Wait(context.Background(), "cluster_proxy.active")
	if err != nil || !waited.Complete || writes != 1 || reads != 1 || waited.Receipt == nil || waited.Receipt.CallID != accepted.Receipt.CallID {
		t.Fatalf("proxy wait %+v %v writes%d reads%d", waited, err, writes, reads)
	}
}

func TestOneCProxyHandleRejectsUnrelatedExtraReceiptReferences(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecProxyFixture))}, nil
	}))
	var spec onec.CreateClusterProxyRequest
	if json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1"}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	accepted, err := regional.CreateClusterProxy(context.Background(), cluster, spec, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	receipt := *accepted.Receipt
	receipt.ResourceRefs = append([]types.ResourceRef{}, receipt.ResourceRefs...)
	extra := cluster
	extra.Kind = "datastore"
	extra.ID = "foreign-store"
	receipt.ResourceRefs = append(receipt.ResourceRefs, extra)
	_, err = regional.ClusterProxyHandle(*accepted.Data.ClusterProxy.Ref, &receipt)
	if types.ErrorCode(err) != types.ScopeMismatch {
		t.Fatalf("unrelated receipt reference accepted: %v", err)
	}
}

func TestOneCUnknownProxyCreationKeepsParentWithoutAdoptingProxy(t *testing.T) {
	calls := 0
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF }))
	var spec onec.CreateClusterProxyRequest
	if json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1"}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.CreateClusterProxy(context.Background(), cluster, spec, types.CallOptions{})
	if types.ErrorCode(err) != types.UnknownOutcome || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || out.Receipt.Attempts != 1 || out.Receipt.Recovery != "unavailable" || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != cluster || calls != 1 {
		t.Fatalf("unknown proxy %+v %v calls%d", out, err, calls)
	}
	_, err = c.Resume(*out.Receipt)
	if types.ErrorCode(err) != types.RecoveryUnavailable || calls != 1 {
		t.Fatalf("parent substituted for unknown proxy: %v calls%d", err, calls)
	}
}

func TestOneCAcceptedProxyIdentitySurvivesIndependentDecodeFailure(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(*http.Request) (*http.Response, error) {
		payload := `{"clusterProxy":{"id":"proxy-1","projectId":"test-project","createdAt":"bad-time"}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	var spec onec.CreateClusterProxyRequest
	if json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1"}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.CreateClusterProxy(context.Background(), cluster, spec, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.Recovery != "resource" {
		t.Fatalf("accepted proxy %+v %v", out, err)
	}
	found := false
	for _, ref := range out.Receipt.ResourceRefs {
		if ref.Kind == "cluster_proxy" && ref.ID == "proxy-1" && ref.Parent != nil && *ref.Parent == cluster {
			found = true
		}
	}
	if !found {
		t.Fatal("trustworthy accepted proxy ID lost")
	}
}

func TestOneCProxyForeignIdentitiesNeverBecomeReceiptReferences(t *testing.T) {
	for _, tc := range []struct{ name, old, new string }{
		{"project", `"projectId":"test-project"`, `"projectId":"foreign-project"`},
		{"spec-parent", `"clusterId":"cluster-1"`, `"clusterId":"foreign-cluster"`},
		{"embedded-parent", `"id":"cluster-1"`, `"id":"foreign-cluster"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Replace(onecProxyFixture, tc.old, tc.new, 1)))}, nil
			}))
			var spec onec.CreateClusterProxyRequest
			if json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1"}}`), &spec) != nil {
				t.Fatal("fixture")
			}
			out, err := regional.CreateClusterProxy(context.Background(), cluster, spec, types.CallOptions{})
			if types.ErrorCode(err) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != cluster || out.Receipt.Recovery != "unavailable" {
				t.Fatalf("foreign proxy identity %+v %v", out, err)
			}
		})
	}
}

func TestOneCProxyDeletionReceiptAllowsOnlyDeletedGoal(t *testing.T) {
	writes, reads := 0, 0
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/cluster-proxies/proxy-1" {
			t.Fatalf("delete proxy path %s", r.URL)
		}
		state := "CLUSTER_PROXY_STATUS_DELETING"
		switch r.Method {
		case "DELETE":
			writes++
		case "GET":
			reads++
			state = "CLUSTER_PROXY_STATUS_DELETED"
		default:
			t.Fatalf("unexpected method%s", r.Method)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(strings.Replace(onecProxyFixture, "CLUSTER_PROXY_STATUS_ACTIVE", state, 1)))}, nil
	}))
	ref := cluster
	ref.Kind = "cluster_proxy"
	ref.ID = "proxy-1"
	ref.Parent = &cluster
	accepted, err := regional.DeleteClusterProxy(context.Background(), ref, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Receipt == nil || accepted.Receipt.Outcome != types.Accepted || len(accepted.Receipt.ResourceRefs) != 1 {
		t.Fatalf("delete proxy %+v", accepted)
	}
	h, err := c.Resume(*accepted.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Wait(context.Background(), "cluster_proxy.active")
	if types.ErrorCode(err) != types.CapabilityUnavailable || reads != 0 || writes != 1 {
		t.Fatalf("wrong proxy deletion goal %v", err)
	}
	waited, err := h.Wait(context.Background(), "cluster_proxy.deleted")
	if err != nil || !waited.Complete || waited.Last == nil || waited.Last.ProviderState != "CLUSTER_PROXY_STATUS_DELETED" || reads != 1 || writes != 1 {
		t.Fatalf("proxy deleted %+v %v", waited, err)
	}
}

func TestOneCProxyListingUsesExplicitClusterFilterAsParent(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/cluster-proxies" || r.URL.Query().Get("clusterId") != "cluster-1" || r.URL.Query().Get("cursor.limit") != "1" {
			t.Fatalf("proxy list wire %s %s", r.Method, r.URL)
		}
		payload := `{"clusterProxies":[{"id":"proxy-1","projectId":"test-project","spec":{"clusterId":"cluster-1","publicAccess":false}}],"cursor":{"next":"50","total":"2"}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	out, err := regional.ListClusterProxies(context.Background(), &cluster, pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.Parent == nil || *out.Items[0].Ref.Parent != cluster || out.NextCursor == nil || out.NextCursor.WireCursor != "50" {
		t.Fatalf("proxy list %+v %v", out, err)
	}
}

func TestOneCProxyCreationValidatesSourceConditionsAndParentBeforeIO(t *testing.T) {
	for _, tc := range []struct {
		name string
		want types.Code
	}{{"public-network", types.InvalidArgument}, {"cluster-id", types.ScopeMismatch}, {"parent-service", types.ScopeMismatch}, {"parent-generation", types.ScopeMismatch}, {"parent-endpoint", types.EndpointUnavailable}} {
		t.Run(tc.name, func(t *testing.T) {
			scope := types.Scope{AccountID: "test-account", ProjectID: "test-project", Region: "spb", Pool: "ru-9"}
			endpoint := types.EndpointBinding{ID: "synthetic-onec-ru9", URL: "https://synthetic.onec.example/v2", Scope: scope, APIGeneration: "v2", Audience: "selectel"}
			lookups, calls := 0, 0
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
				lookups++
				return source().Lookup(ctx, r)
			}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "cloud_1c", APIGeneration: "v2", Scope: scope}: endpoint}, Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
			if e != nil {
				t.Fatal(e)
			}
			p, _ := c.Project("test-project")
			regional, _ := p.Location("spb", "ru-9")
			parent := types.ResourceRef{Scope: scope, Service: "cloud_1c", APIGeneration: "v2", Kind: "cluster", ID: "cluster-1", EndpointID: endpoint.ID, EndpointURL: endpoint.URL}
			var spec onec.CreateClusterProxyRequest
			if json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1"}}`), &spec) != nil {
				t.Fatal("fixture")
			}
			switch tc.name {
			case "public-network":
				v := true
				spec.Spec.PublicAccess = &v
			case "cluster-id":
				v := "other-cluster"
				spec.Spec.ClusterID = &v
			case "parent-service":
				parent.Service = "compute"
			case "parent-generation":
				parent.APIGeneration = "v1"
			case "parent-endpoint":
				parent.EndpointURL += "/foreign"
			}
			out, e := regional.OneC().CreateClusterProxy(context.Background(), parent, spec, types.CallOptions{})
			if types.ErrorCode(e) != tc.want || lookups != 0 || calls != 0 || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || out.Receipt.Attempts != 0 {
				t.Fatalf("proxy local gate %+v %v lookups%d calls%d", out, e, lookups, calls)
			}
		})
	}
}

func TestOneCProxyHandleDoesNotAcceptUndocumentedProviderOperationRef(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecProxyFixture))}, nil
	}))
	var spec onec.CreateClusterProxyRequest
	if json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1"}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.CreateClusterProxy(context.Background(), cluster, spec, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	receipt := *out.Receipt
	undeclared := cluster
	undeclared.Kind = "provider_task"
	undeclared.ID = "undeclared-task"
	receipt.ProviderOperationRef = &undeclared
	_, err = regional.ClusterProxyHandle(*out.Data.ClusterProxy.Ref, &receipt)
	if types.ErrorCode(err) != types.CapabilityUnavailable {
		t.Fatalf("undeclared operation accepted: %v", err)
	}
}

func TestOneCDomainListingKeepsExplicitParentAndProtectedTLSWireStrings(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/clusters/cluster-1/domains" || r.URL.Query().Get("cursor.limit") != "1" {
			t.Fatalf("domains wire %s %s", r.Method, r.URL)
		}
		payload := `{"clusterDomains":[{"id":"domain-1","spec":{"clusterId":"cluster-1","fqdn":"synthetic.example","autoIssuedCert":false,"zoneId":"caller-zone-metadata","customCert":{"certificate":"synthetic-cert-wire","key":"synthetic-key-wire"}},"state":{"status":"CLUSTER_DOMAIN_STATUS_ACTIVE","certExpiresAt":"2026-10-20T12:00:00+03:00"}}],"cursor":{"next":"10","total":"2"}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	out, err := regional.ListClusterDomains(context.Background(), cluster, pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.Items[0].Ref == nil || out.Items[0].Ref.Parent == nil || *out.Items[0].Ref.Parent != cluster || string(out.Items[0].Spec.CustomCert.Certificate.Reveal()) != "synthetic-cert-wire" || string(out.Items[0].Spec.CustomCert.Key.Reveal()) != "synthetic-key-wire" || out.Items[0].State.CertExpiresAt.Location() != time.UTC {
		t.Fatalf("domain list %+v %v", out, err)
	}
	raw, _ := json.Marshal(out)
	diagnostic := fmt.Sprintf("%+v %#v %+v %#v %x", out, out, *out.Items[0].Spec.CustomCert.Key, *out.Items[0].Spec.CustomCert.Certificate, *out.Items[0].Spec.CustomCert.Key)
	for _, marker := range []string{"synthetic-cert-wire", "synthetic-key-wire"} {
		if strings.Contains(string(raw), marker) || strings.Contains(diagnostic, marker) || strings.Contains(diagnostic, fmt.Sprint([]byte(marker))) || strings.Contains(diagnostic, fmt.Sprintf("%x", []byte(marker))) {
			t.Fatal("TLS material leaked")
		}
	}
}

func TestOneCDomainRegistrationRevealsTLSOnlyAtExplicitWireBoundary(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		var wire struct {
			Spec struct {
				ClusterID      string                            `json:"clusterId"`
				AutoIssuedCert *bool                             `json:"autoIssuedCert"`
				CustomCert     struct{ Certificate, Key string } `json:"customCert"`
			} `json:"spec"`
		}
		if r.Method != "POST" || r.URL.Path != "/v2/clusters/cluster-1/domains" || json.Unmarshal(raw, &wire) != nil || wire.Spec.ClusterID != "cluster-1" || wire.Spec.AutoIssuedCert == nil || *wire.Spec.AutoIssuedCert || wire.Spec.CustomCert.Certificate != "synthetic-cert-wire" || wire.Spec.CustomCert.Key != "synthetic-key-wire" {
			t.Fatalf("domain registration wire %s %s", r.Method, r.URL)
		}
		payload := strings.TrimSuffix(onecClusterFixture, "}") + `,"domain":{"id":"domain-1","spec":{"clusterId":"cluster-1","fqdn":"synthetic.example","autoIssuedCert":false,"customCert":{"certificate":"synthetic-cert-wire","key":"synthetic-key-wire"}},"state":{"status":"CLUSTER_DOMAIN_STATUS_CREATING"}}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	var spec onec.RegisterClusterDomainBody
	if json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1","fqdn":"synthetic.example","autoIssuedCert":false,"customCert":{"certificate":"synthetic-cert-wire","key":"synthetic-key-wire"}}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	safe, _ := json.Marshal(spec)
	if strings.Contains(string(safe), "synthetic-key-wire") {
		t.Fatal("request diagnostics exposed TLS")
	}
	out, err := regional.RegisterClusterDomain(context.Background(), cluster, spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Data.Domain == nil || out.Data.Domain.Ref == nil || out.Data.Domain.Ref.Parent == nil || *out.Data.Domain.Ref.Parent != cluster || out.Data.Cluster == nil || out.Data.Cluster.Ref == nil || *out.Data.Cluster.Ref != cluster {
		t.Fatalf("domain registration %+v %v", out, err)
	}
}

func TestOneCDomainStatusScansOnlyForExactAcknowledgedID(t *testing.T) {
	writes, reads := 0, 0
	var readDeadline time.Time
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/clusters/cluster-1/domains" {
			t.Fatalf("invented domain Get endpoint %s", r.URL)
		}
		var payload string
		if r.Method == "POST" {
			writes++
			payload = strings.TrimSuffix(onecClusterFixture, "}") + `,"domain":{"id":"domain-1","spec":{"clusterId":"cluster-1","fqdn":"synthetic.example","autoIssuedCert":true},"state":{"status":"CLUSTER_DOMAIN_STATUS_CREATING"}}}`
		} else if r.Method == "GET" {
			reads++
			deadline, ok := r.Context().Deadline()
			if !ok {
				t.Fatal("unbounded domain status")
			}
			if reads == 1 {
				readDeadline = deadline
			} else if !deadline.Equal(readDeadline) {
				t.Fatal("status budget reset between native pages")
			}
			id, next := "other-domain", "50"
			if reads == 2 {
				if r.URL.Query().Get("cursor.cursor") != "50" {
					t.Fatalf("domain continuation %s", r.URL)
				}
				id, next = "domain-1", "100"
			}
			payload = `{"clusterDomains":[{"id":"` + id + `","spec":{"clusterId":"cluster-1","fqdn":"synthetic.example"},"state":{"status":"CLUSTER_DOMAIN_STATUS_ACTIVE"}}],"cursor":{"next":"` + next + `","total":"2"}}`
		} else {
			t.Fatalf("unexpected method%s", r.Method)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	var spec onec.RegisterClusterDomainBody
	if json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1","fqdn":"synthetic.example","autoIssuedCert":true}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	accepted, err := regional.RegisterClusterDomain(context.Background(), cluster, spec, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.Resume(*accepted.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	waited, err := h.Wait(context.Background(), "cluster_domain.active")
	if err != nil || !waited.Complete || waited.Last == nil || waited.Last.Ref.ID != "domain-1" || writes != 1 || reads != 2 {
		t.Fatalf("exact domain status %+v %v writes%d reads%d", waited, err, writes, reads)
	}
}

func TestOneCPartialDomainListNeverProvesDeletionOrAdoptsSameName(t *testing.T) {
	c, _, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/clusters/cluster-1/domains" {
			t.Fatalf("domain lookup %s", r.URL)
		}
		payload := `{"clusterDomains":[{"id":"other-domain","spec":{"clusterId":"cluster-1","fqdn":"synthetic.example"},"state":{"status":"CLUSTER_DOMAIN_STATUS_DELETED"}}]}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	ref := cluster
	ref.Kind = "cluster_domain"
	ref.ID = "domain-1"
	ref.Parent = &cluster
	h, err := c.Resource(ref)
	if err != nil {
		t.Fatal(err)
	}
	out, err := h.Wait(context.Background(), "cluster_domain.deleted")
	if types.ErrorCode(err) != types.Incomplete || out.Complete || out.Last != nil {
		t.Fatalf("partial list deletion %+v %v", out, err)
	}
}

func TestOneCDomainPatchUsesExactChildAndRawProtectedSpec(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		var wire map[string]json.RawMessage
		if r.Method != "PATCH" || r.URL.Path != "/v2/clusters/cluster-1/domains/domain-1" || json.Unmarshal(raw, &wire) != nil || wire["spec"] != nil || string(wire["clusterId"]) != `"cluster-1"` || !strings.Contains(string(wire["customCert"]), `"key":"synthetic-renewed-key"`) {
			t.Fatalf("domain patch wire %s %s", r.Method, r.URL)
		}
		payload := strings.TrimSuffix(onecClusterFixture, "}") + `,"domain":{"id":"domain-1","spec":` + string(raw) + `}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	ref := cluster
	ref.Kind = "cluster_domain"
	ref.ID = "domain-1"
	ref.Parent = &cluster
	var spec onec.ClusterDomainSpec
	if json.Unmarshal([]byte(`{"clusterId":"cluster-1","fqdn":"synthetic.example","autoIssuedCert":false,"customCert":{"certificate":"synthetic-renewed-cert","key":"synthetic-renewed-key"}}`), &spec) != nil {
		t.Fatal("fixture")
	}
	out, err := regional.PatchClusterDomain(context.Background(), ref, spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Data.Domain == nil || out.Data.Domain.Ref == nil || out.Data.Domain.Ref.Parent == nil || *out.Data.Domain.Ref.Parent != cluster {
		t.Fatalf("patched domain %+v %v", out, err)
	}
}

func TestOneCDomainDeletionDoesNotUseParentClusterAsCompletionProof(t *testing.T) {
	writes, reads := 0, 0
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			writes++
			if r.URL.Path != "/v2/clusters/cluster-1/domains/domain-1" {
				t.Fatalf("domain delete path %s", r.URL)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecClusterFixture))}, nil
		}
		reads++
		if r.Method != "GET" || r.URL.Path != "/v2/clusters/cluster-1/domains" {
			t.Fatalf("domain delete observation %s %s", r.Method, r.URL)
		}
		payload := `{"clusterDomains":[{"id":"domain-1","spec":{"clusterId":"cluster-1","fqdn":"synthetic.example"},"state":{"status":"CLUSTER_DOMAIN_STATUS_DELETED"}}]}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	ref := cluster
	ref.Kind = "cluster_domain"
	ref.ID = "domain-1"
	ref.Parent = &cluster
	out, err := regional.DeleteClusterDomain(context.Background(), ref, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Data.Cluster == nil || out.Data.Cluster.Ref == nil || *out.Data.Cluster.Ref != cluster {
		t.Fatalf("domain deletion %+v", out)
	}
	h, err := c.Resume(*out.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Wait(context.Background(), "cluster_domain.active")
	if types.ErrorCode(err) != types.CapabilityUnavailable || reads != 0 {
		t.Fatalf("wrong delete goal %v reads%d", err, reads)
	}
	waited, err := h.Wait(context.Background(), "cluster_domain.deleted")
	if err != nil || !waited.Complete || waited.Last == nil || waited.Last.Ref.ID != "domain-1" || reads != 1 || writes != 1 {
		t.Fatalf("domain deletion wait %+v %v", waited, err)
	}
}

func TestOneCDomainTransferRetainsBothLocationsAndRecoversOnlyAcknowledgedTarget(t *testing.T) {
	reads := 0
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		payload := strings.TrimSuffix(onecClusterFixture, "}") + `,"domain":{"id":"domain-1","spec":{"clusterId":"cluster-2","fqdn":"synthetic.example"}}}`
		if r.Method == "POST" {
			raw, _ := io.ReadAll(r.Body)
			if r.URL.Path != "/v2/clusters/cluster-1/domains/domain-1/transfer" || string(raw) != `{"targetClusterId":"cluster-2"}` {
				t.Fatalf("transfer wire %s %s", r.URL, raw)
			}
		} else {
			reads++
			if r.Method != "GET" || r.URL.Path != "/v2/clusters/cluster-2/domains" {
				t.Fatalf("transfer observation %s", r.URL)
			}
			payload = `{"clusterDomains":[{"id":"domain-1","spec":{"clusterId":"cluster-2","fqdn":"synthetic.example"},"state":{"status":"CLUSTER_DOMAIN_STATUS_ACTIVE"}}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	// The supplied source and target are explicit references in the same scope.
	target := cluster
	target.ID = "cluster-2"
	ref := cluster
	ref.Kind = "cluster_domain"
	ref.ID = "domain-1"
	ref.Parent = &cluster
	var spec onec.TransferClusterDomainBody
	if err := json.Unmarshal([]byte(`{"targetClusterId":"cluster-2"}`), &spec); err != nil {
		t.Fatal(err)
	}
	out, err := regional.TransferClusterDomain(context.Background(), ref, target, spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 3 || out.Data.Domain.Ref.Parent.ID != "cluster-2" || out.Data.Cluster.Ref.ID != "cluster-1" {
		t.Fatalf("transfer %+v %v", out, err)
	}
	h, err := c.Resume(*out.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	waited, err := h.Wait(context.Background(), "cluster_domain.active")
	if err != nil || !waited.Complete || reads != 1 {
		t.Fatalf("target wait %+v %v", waited, err)
	}
}

func TestOneCDomainTransferRequiresAcknowledgedTargetBeforeRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		lost          bool
		code          types.Code
		refs          int
		recoverable   bool
	}{
		{name: "lost", lost: true, code: types.UnknownOutcome, refs: 2},
		{name: "old owner only", payload: `{"domain":{"id":"domain-1","spec":{"clusterId":"cluster-1","fqdn":"synthetic.example"}}}`, refs: 2},
		{name: "foreign owner", payload: `{"domain":{"id":"domain-1","spec":{"clusterId":"foreign","fqdn":"synthetic.example"}}}`, code: types.ScopeMismatch, refs: 2},
		{name: "bad unrelated date", payload: `{"domain":{"id":"domain-1","spec":{"clusterId":"cluster-2","fqdn":"synthetic.example"},"createdAt":"not-a-date"}}`, code: types.ProtocolError, refs: 3, recoverable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
				writes++
				if r.Method != "POST" {
					t.Fatal("unrequested observation")
				}
				if tc.lost {
					return nil, io.ErrUnexpectedEOF
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.payload))}, nil
			}))
			target := cluster
			target.ID = "cluster-2"
			ref := cluster
			ref.Kind = "cluster_domain"
			ref.ID = "domain-1"
			ref.Parent = &cluster
			var spec onec.TransferClusterDomainBody
			_ = json.Unmarshal([]byte(`{"targetClusterId":"cluster-2"}`), &spec)
			out, err := regional.TransferClusterDomain(context.Background(), ref, target, spec, types.CallOptions{})
			if types.ErrorCode(err) != tc.code || out.Receipt == nil || len(out.Receipt.ResourceRefs) != tc.refs || writes != 1 {
				t.Fatalf("transfer %+v %v writes%d", out, err, writes)
			}
			_, err = c.Resume(*out.Receipt)
			if tc.recoverable {
				if err != nil {
					t.Fatal(err)
				}
			} else if types.ErrorCode(err) != types.RecoveryUnavailable {
				t.Fatalf("invented transfer recovery %v", err)
			}
		})
	}
}

func TestOneCVolumeSupportedRegionsIsBoundedFiniteSourceCollection(t *testing.T) {
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/clusters/volumes/supported-regions" {
			t.Fatalf("volume regions %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"regions":["ru-1","ru-2"]}`))}, nil
	}))
	out, err := regional.ListVolumeSupportedRegions(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.Items[0] != "ru-1" {
		t.Fatalf("regions %+v %v", out, err)
	}
}

func TestOneCVolumeVerificationForwardsProtectedCredentialsWithoutMounting(t *testing.T) {
	calls := 0
	_, regional, _ := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		var wire struct {
			Spec struct {
				ClusterID string `json:"clusterId"`
				S3        struct {
					AccessKeyID     string `json:"accessKeyId"`
					SecretAccessKey string `json:"secretAccessKey"`
				} `json:"s3"`
			} `json:"spec"`
		}
		if r.Method != "POST" || r.URL.Path != "/v2/clusters/volumes/verify" || json.Unmarshal(raw, &wire) != nil || wire.Spec.ClusterID != "cluster-1" || wire.Spec.S3.SecretAccessKey != "synthetic-volume-secret" || wire.Spec.S3.AccessKeyID != "synthetic-volume-access" {
			t.Fatalf("verify wire %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}))
	var spec onec.VerifyClusterVolumeSpecRequest
	_ = json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1","type":"CLUSTER_VOLUME_TYPE_S3","s3":{"bucketName":"synthetic-bucket","accessKeyId":"synthetic-volume-access","secretAccessKey":"synthetic-volume-secret"}}}`), &spec)
	safe, _ := json.Marshal(spec)
	if strings.Contains(string(safe), "synthetic-volume-secret") || strings.Contains(fmt.Sprintf("%+v %#v", spec, spec), "synthetic-volume-secret") {
		t.Fatal("volume credentials leaked")
	}
	out, err := regional.VerifyVolumeSpec(context.Background(), spec, types.CallOptions{})
	if err != nil || out.Receipt != nil || calls != 1 {
		t.Fatalf("verification %+v %v", out, err)
	}
}

func TestOneCVolumeListPreservesExactSizeProtectedFieldsAndPartialCursor(t *testing.T) {
	_, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/v2/clusters/cluster-1/volumes" {
			t.Fatalf("volume list %s", r.URL)
		}
		payload := `{"volumes":[{"id":"volume-1","spec":{"clusterId":"cluster-1","size":"18446744073709551615","s3":{"bucketName":"synthetic-bucket","secretAccessKey":"synthetic-volume-secret"}},"state":{"status":"CLUSTER_VOLUME_STATUS_ACTIVE","reason":"synthetic-provider-detail","mountPath":"synthetic-private-path"}}],"cursor":{"next":"50","total":"1"}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	out, err := regional.ListClusterVolumes(context.Background(), cluster, pagination.Options{MaxPages: 1})
	if types.ErrorCode(err) != types.Incomplete || out.Complete || len(out.Items) != 1 || out.Items[0].Ref.Parent.ID != "cluster-1" || *out.Items[0].Spec.Size != "18446744073709551615" {
		t.Fatalf("volume list %+v %v", out, err)
	}
	safe, _ := json.Marshal(out.Items[0])
	if strings.Contains(string(safe), "synthetic-volume-secret") || strings.Contains(string(safe), "synthetic-provider-detail") || strings.Contains(string(safe), "synthetic-private-path") {
		t.Fatal("volume output leaked")
	}
}

func TestOneCVolumeMountPreservesAcceptedChildAndExactRecovery(t *testing.T) {
	writes, reads := 0, 0
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v2/clusters/cluster-1/volumes" {
			t.Fatalf("volume path %s", r.URL)
		}
		payload := `{"volume":{"id":"volume-1","spec":{"clusterId":"cluster-1","s3":{"bucketName":"synthetic-bucket","secretAccessKey":"synthetic-volume-secret"}},"state":{"status":"CLUSTER_VOLUME_STATUS_MOUNTING"}}}`
		if r.Method == "POST" {
			writes++
			raw, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(raw), `"secretAccessKey":"synthetic-volume-secret"`) {
				t.Fatal("mount lost explicit credential")
			}
		} else {
			reads++
			payload = `{"volumes":[{"id":"volume-1","spec":{"clusterId":"cluster-1"},"state":{"status":"CLUSTER_VOLUME_STATUS_ACTIVE"}}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}))
	var spec onec.MountClusterVolumeBody
	_ = json.Unmarshal([]byte(`{"spec":{"clusterId":"cluster-1","s3":{"bucketName":"synthetic-bucket","secretAccessKey":"synthetic-volume-secret"}}}`), &spec)
	out, err := regional.MountClusterVolume(context.Background(), cluster, spec, types.CallOptions{})
	if err != nil || out.Receipt == nil || len(out.Receipt.ResourceRefs) != 2 || out.Data.Volume.Ref.Parent.ID != "cluster-1" {
		t.Fatalf("mount %+v %v", out, err)
	}
	h, err := c.Resume(*out.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	waited, err := h.Wait(context.Background(), "cluster_volume.active")
	if err != nil || !waited.Complete || writes != 1 || reads != 1 {
		t.Fatalf("volume ready %+v %v", waited, err)
	}
}

func TestOneCVolumeUnmountWaitsForExactExplicitDeletedState(t *testing.T) {
	reads := 0
	c, regional, cluster := onecRegionalFixture(t, transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			if r.URL.Path != "/v2/clusters/cluster-1/volumes/volume-1" {
				t.Fatalf("unmount path %s", r.URL)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(onecClusterFixture))}, nil
		}
		reads++
		if r.Method != "GET" || r.URL.Path != "/v2/clusters/cluster-1/volumes" {
			t.Fatalf("unmount observation %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"volumes":[{"id":"volume-1","spec":{"clusterId":"cluster-1"},"state":{"status":"CLUSTER_VOLUME_STATUS_DELETED"}}]}`))}, nil
	}))
	ref := cluster
	ref.Kind = "cluster_volume"
	ref.ID = "volume-1"
	ref.Parent = &cluster
	if _, err := c.Resource(ref); err != nil {
		t.Fatal(err)
	}
	out, err := regional.UnmountClusterVolume(context.Background(), ref, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted {
		t.Fatalf("unmount %+v %v", out, err)
	}
	h, err := c.Resume(*out.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Wait(context.Background(), "cluster_volume.active")
	if types.ErrorCode(err) != types.CapabilityUnavailable || reads != 0 {
		t.Fatalf("unmount wrong goal %v", err)
	}
	waited, err := h.Wait(context.Background(), "cluster_volume.deleted")
	if err != nil || !waited.Complete || reads != 1 {
		t.Fatalf("unmount wait %+v %v", waited, err)
	}
}
