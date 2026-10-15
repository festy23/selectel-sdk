package letsencrypt_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/letsencrypt"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type wireTransport func(*http.Request) (*http.Response, error)

func (f wireTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestLEListMapsFullSourceMetadataAndScope(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	calls := 0
	root, err := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Service != "certificates" || r.Scope != scope || r.Mode != types.IAMProject {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: wireTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return &http.Response{StatusCode: 200, Header: http.Header{"X-Subject-Token": {"synthetic-token"}}, Body: io.NopCloser(strings.NewReader(`{"token":{"expires_at":"2099-01-01T00:00:00Z","user":{"id":"u"},"project":{"id":"p","domain":{"name":"a"}},"catalog":[]}}`))}, nil
		}
		if r.Method != "GET" || r.URL.String() != "https://api.selectel.ru/certs/le/" || r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal(r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"items":[{"id":"le-1","project_id":"p","name":"domain","domains":["*.example.com","example.com"],"status":"active","version":9007199254740993,"created_at":"2022-02-02 22:22:22.222","updated_at":"2022-02-02 22:22:22.222","expire_at":"2023-10-02T22:22:22.222Z","knox_cert_id":"knox-1","error_description":"sensitive-provider-detail"}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	project, _ := root.Project("p")
	it, err := project.LetsEncrypt().List(pagination.Options{MaxItems: 5, MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	out, err := it.Collect(context.Background())
	if err != nil || !out.Complete || len(out.Items) != 1 {
		t.Fatal(out, err)
	}
	v := out.Items[0]
	if v.Ref.Scope != scope || v.Ref.Kind != "lets_encrypt_certificate" || v.Ref.ID != "le-1" || v.Version.String() != "9007199254740993" || *v.CreatedAt != "2022-02-02 22:22:22.222" || *v.Status != "active" || len(v.Domains) != 2 || calls != 2 {
		t.Fatal(v, calls)
	}
}

func leClient(t *testing.T, wire wireTransport, configure ...func(*sdk.Config)) (*sdk.Client, *letsencrypt.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	cfg := sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: wireTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return &http.Response{StatusCode: 200, Header: http.Header{"X-Subject-Token": {"synthetic-token"}}, Body: io.NopCloser(strings.NewReader(`{"token":{"expires_at":"2099-01-01T00:00:00Z","user":{"id":"u"},"project":{"id":"p","domain":{"name":"a"}},"catalog":[]}}`))}, nil
		}
		return wire(r)
	})}
	for _, f := range configure {
		f(&cfg)
	}
	root, err := sdk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	project, _ := root.Project("p")
	return root, project.LetsEncrypt()
}
func TestLEIssuePreservesDNSV2PresenceAndAcceptedIdentity(t *testing.T) {
	for _, variant := range []string{"omitted", "false", "true", "malformed"} {
		t.Run(variant, func(t *testing.T) {
			calls := 0
			_, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/certs/le/issue" {
					t.Fatal(r.Method, r.URL)
				}
				want := ""
				if variant == "false" {
					want = "dnsv2=false"
				}
				if variant == "true" || variant == "malformed" {
					want = "dnsv2=true"
				}
				if r.URL.RawQuery != want {
					t.Fatal(r.URL)
				}
				raw, _ := io.ReadAll(r.Body)
				if string(raw) != `{"domains":["*.example.com","example.com"],"name":"domain"}` {
					t.Fatal(string(raw))
				}
				body := `{"id":"issued","project_id":"p","status":"creating"}`
				if variant == "malformed" {
					body = `{"id":"issued","project_id":"p","version":"bad"}`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			options := letsencrypt.IssueOptions{}
			value := variant != "false"
			if variant != "omitted" {
				options.DNSV2 = &value
			}
			out, err := c.Issue(context.Background(), letsencrypt.IssueRequest{Domains: []string{"*.example.com", "example.com"}, Name: "domain"}, options, types.CallOptions{})
			if (variant == "malformed" && types.ErrorCode(err) != types.ProtocolError) || (variant != "malformed" && err != nil) || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "issued" || calls != 1 {
				t.Fatal(out, err, calls)
			}
		})
	}
}
func TestLEDeleteReceiptsUnknownNeverReplayAndScopeGuard(t *testing.T) {
	for _, status := range []int{204, 400, 403, 404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			_, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "DELETE" || r.URL.String() != "https://api.selectel.ru/certs/le/known" {
					t.Fatal(r.Method, r.URL)
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error_text":"sensitive-provider-detail"}`))}, nil
			}))
			ref, err := c.Ref("known")
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Delete(context.Background(), ref, types.CallOptions{})
			want := types.Rejected
			if status == 204 {
				want = types.Accepted
			}
			if status == 500 {
				want = types.Unknown
			}
			if out.Receipt == nil || out.Receipt.Outcome != want || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "known" || calls != 1 || (status == 204 && err != nil) || (status == 500 && types.ErrorCode(err) != types.UnknownOutcome) {
				t.Fatal(out, err, calls)
			}
			ref.Scope.ProjectID = "other"
			out, err = c.Delete(context.Background(), ref, types.CallOptions{})
			if types.ErrorCode(err) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 1 {
				t.Fatal(out, err, calls)
			}
		})
	}
}
func TestLEResourceResumeObservesOnlyExactIDAndWaitsSourceStates(t *testing.T) {
	mutations, reads := 0, 0
	root, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			mutations++
			return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		reads++
		body := `{"items":[{"id":"unrelated","status":"active"},{"id":"known","status":"issuing"}]}`
		if reads > 1 {
			body = `{"items":[{"id":"known","status":"active"}]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}), func(cfg *sdk.Config) { cfg.PollInterval = time.Millisecond })
	ref, err := c.Ref("known")
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(err) != types.UnknownOutcome {
		t.Fatal(err)
	}
	h, err := root.Resume(*out.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := h.Recover(context.Background())
	if types.ErrorCode(err) != types.UnknownOutcome || recovered.ProofStatus != "indeterminate" || recovered.Observation == nil || recovered.Observation.Ref.ID != "known" || recovered.Observation.ProviderState != "issuing" || mutations != 1 || reads != 1 {
		t.Fatal(recovered, err, mutations, reads)
	}
	h, err = root.Resource(ref)
	if err != nil {
		t.Fatal(err)
	}
	wait, err := h.Wait(context.Background(), "active")
	if err != nil || !wait.Complete || wait.Last.ProviderState != "active" || mutations != 1 {
		t.Fatal(wait, err, mutations)
	}
}
func TestLEKnownNoRefReceiptRecoversWithoutIO(t *testing.T) {
	calls := 0
	root, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}))
	out, err := c.Issue(context.Background(), letsencrypt.IssueRequest{Name: "domain", Domains: []string{"example.com"}}, letsencrypt.IssueOptions{}, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted {
		t.Fatal(out, err)
	}
	h, err := root.Resume(*out.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := h.Recover(context.Background())
	if err != nil || recovery.ProofStatus != "conclusive" || recovery.Observation != nil || calls != 1 {
		t.Fatal(recovery, err, calls)
	}
	if _, err = h.Status(context.Background()); types.ErrorCode(err) != types.RecoveryUnavailable {
		t.Fatal(err)
	}
}
func TestLEListMalformedPrefixAndProtectedErrorOutput(t *testing.T) {
	_, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"items":[{"id":"good","error_description":"sensitive-provider-detail"},{"id":""}]}`))}, nil
	}))
	it, err := c.List(pagination.Options{MaxItems: 5, MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	out, err := it.Collect(context.Background())
	if types.ErrorCode(err) != types.ProtocolError || out.Complete || len(out.Items) != 1 || out.Items[0].Ref.ID != "good" {
		t.Fatal(out, err)
	}
	v := out.Items[0]
	for _, value := range []any{v, v.ErrorDescription, *v.ErrorDescription} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			text := fmt.Sprintf(format, value)
			if strings.Contains(text, "sensitive-provider-detail") || strings.Contains(text, "115 101 110 115") || strings.Contains(text, "0x73, 0x65, 0x6e") {
				t.Fatal(text)
			}
		}
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "sensitive-provider-detail") {
			t.Fatal(string(raw))
		}
	}
}
func TestLEStatusCannotInferAbsenceFromIncompleteList(t *testing.T) {
	root, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"items":[{"id":"other"},{"id":"known","status":"active"}]}`))}, nil
	}), func(cfg *sdk.Config) { cfg.MaxItems = 1 })
	ref, _ := c.Ref("known")
	h, err := root.Resource(ref)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := h.Status(context.Background())
	if types.ErrorCode(err) != types.Incomplete || observation.Classification == "absent" {
		t.Fatal(observation, err)
	}
}
func TestLEAuth404NeverEstablishesResourceAbsence(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	calls := 0
	root, err := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: wireTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	project, _ := root.Project("p")
	ref, _ := project.LetsEncrypt().Ref("known")
	cap, _ := root.Catalog().Describe("certificates.le_v1.certificates.delete")
	receipt := types.Receipt{SchemaVersion: 1, CallID: "synthetic-call", OperationID: cap.OperationID, ContractRevision: cap.ContractRevision, Scope: scope, Outcome: types.Unknown, Recovery: "resource", ResourceRefs: []types.ResourceRef{ref}, Endpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}}
	h, err := root.Resume(receipt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := h.Recover(context.Background())
	if err == nil || result.Observation != nil || calls != 1 {
		t.Fatal(result, err, calls)
	}
}

func TestLEListCursorBindsResolvedEndpointURL(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	newClient := func(endpoint string, calls *int) *letsencrypt.Client {
		_, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != endpoint+"/" {
				t.Fatal(r.URL)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"items":[{"id":"a"},{"id":"b"}]}`))}, nil
		}), func(cfg *sdk.Config) {
			cfg.EndpointOverrides = map[types.EndpointKey]types.EndpointBinding{
				{Service: "certificates", APIGeneration: "le_v1", Scope: scope}: {ID: "same-id", URL: endpoint, Scope: scope, APIGeneration: "le_v1", Audience: "selectel"},
			}
			transport := cfg.Transport
			cfg.Transport = wireTransport(func(r *http.Request) (*http.Response, error) { *calls++; return transport.RoundTrip(r) })
		})
		return c
	}
	firstCalls := 0
	first := newClient("https://first.invalid/prefix", &firstCalls)
	it, err := first.List(pagination.Options{MaxItems: 1, MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	page, err := it.Collect(context.Background())
	if types.ErrorCode(err) != types.Incomplete || page.NextCursor == nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	foreignCalls := 0
	foreign := newClient("https://second.invalid/prefix", &foreignCalls)
	if _, err = foreign.List(pagination.Options{Cursor: page.NextCursor}); types.ErrorCode(err) != types.ScopeMismatch || foreignCalls != 0 {
		t.Fatalf("foreign endpoint accepted: err=%v HTTP=%d", err, foreignCalls)
	}
	originalCalls := 0
	original := newClient("https://first.invalid/prefix", &originalCalls)
	resumed, err := original.List(pagination.Options{MaxItems: 5, MaxPages: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	rest, err := resumed.Collect(context.Background())
	if err != nil || !rest.Complete || len(rest.Items) != 1 || rest.Items[0].Ref.ID != "b" || originalCalls == 0 {
		t.Fatal(rest, err, originalCalls)
	}
}
func TestLEIssueInvalidIdentityNeverAdoptsAndUnknownCannotResume(t *testing.T) {
	for _, body := range []string{`{"id":""}`, `{"id":"."}`, `{"id":"issued","project_id":"other"}`, `{"name":"same-name"}`} {
		t.Run(body, func(t *testing.T) {
			root, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			out, err := c.Issue(context.Background(), letsencrypt.IssueRequest{Name: "same-name", Domains: []string{"example.com"}}, letsencrypt.IssueOptions{}, types.CallOptions{})
			if err == nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 0 {
				t.Fatal(out, err)
			}
			receipt := *out.Receipt
			receipt.Outcome = types.Unknown
			if _, err = root.Resume(receipt); types.ErrorCode(err) != types.RecoveryUnavailable {
				t.Fatal(err)
			}
		})
	}
}
func TestLEExactRefGuardsRunBeforeCredentialsAndHTTP(t *testing.T) {
	lookups, calls := 0, 0
	root, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) { calls++; return nil, fmt.Errorf("unexpected HTTP") }), func(cfg *sdk.Config) {
		cfg.Credentials = credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
			lookups++
			return credentials.Material{}, fmt.Errorf("unexpected credentials")
		})
	})
	ref, _ := c.Ref("known")
	for _, change := range []func(*types.ResourceRef){func(r *types.ResourceRef) { r.Scope.ProjectID = "other" }, func(r *types.ResourceRef) { r.APIGeneration = "user_v1" }, func(r *types.ResourceRef) { r.Kind = "user_certificate" }, func(r *types.ResourceRef) { r.Service = "registry" }, func(r *types.ResourceRef) { r.Parent = &ref }} {
		bad := ref
		change(&bad)
		out, err := c.Delete(context.Background(), bad, types.CallOptions{})
		if err == nil || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || lookups != 0 || calls != 0 {
			t.Fatal(out, err, lookups, calls)
		}
	}
	foreign := ref
	foreign.EndpointURL = "https://other.invalid/le"
	h, err := root.Resource(foreign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Status(context.Background()); types.ErrorCode(err) != types.ScopeMismatch || lookups != 0 || calls != 0 {
		t.Fatal(err, lookups, calls)
	}
}
func TestLEWaitPollsPendingAndHonorsBudget(t *testing.T) {
	calls := 0
	root, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		state := "creating"
		if calls == 2 {
			state = "validating"
		}
		if calls == 3 {
			state = "active"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"items":[{"id":"known","status":"` + state + `"}]}`))}, nil
	}), func(cfg *sdk.Config) { cfg.PollInterval = time.Millisecond })
	ref, _ := c.Ref("known")
	h, err := root.Resource(ref)
	if err != nil {
		t.Fatal(err)
	}
	out, err := h.Wait(context.Background(), "active")
	if err != nil || !out.Complete || calls != 3 {
		t.Fatal(out, err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = h.Wait(ctx, "active"); types.ErrorCode(err) != types.Canceled || calls != 3 {
		t.Fatal(err, calls)
	}
}
func TestLEDeleteReceiptDoesNotOfferIssueGoal(t *testing.T) {
	root, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "DELETE" {
			t.Fatal("unexpected observation", r.Method)
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	ref, _ := c.Ref("known")
	result, err := c.Delete(context.Background(), ref, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h, err := root.Resume(*result.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Wait(context.Background(), "active"); types.ErrorCode(err) != types.CapabilityUnavailable {
		t.Fatal(err)
	}
}
func TestLEWaitUsesDocumentedFailureUnknownAndDeletedStates(t *testing.T) {
	for _, state := range []string{"error", "invalid", "future-provider-state", "deleted", "absent"} {
		t.Run(state, func(t *testing.T) {
			root, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
				body := `{"items":[{"id":"known","status":"` + state + `"}]}`
				if state == "absent" {
					body = `{"items":[]}`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			ref, _ := c.Ref("known")
			h, err := root.Resource(ref)
			if err != nil {
				t.Fatal(err)
			}
			goal := "active"
			want := types.OperationFailed
			if state == "future-provider-state" {
				want = types.StateUnknown
			}
			if state == "deleted" || state == "absent" {
				goal = "deleted"
				want = ""
			}
			out, err := h.Wait(context.Background(), goal)
			if types.ErrorCode(err) != want || (want == "" && !out.Complete) {
				t.Fatal(out, err)
			}
		})
	}
}
func TestLEIssueLeavesUnspecifiedStringAndArrayConstraintsToProvider(t *testing.T) {
	calls := 0
	_, c := leClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"domains":[],"name":""}` {
			t.Fatal(string(body))
		}
		return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}))
	out, err := c.Issue(context.Background(), letsencrypt.IssueRequest{Name: "", Domains: []string{}}, letsencrypt.IssueOptions{}, types.CallOptions{})
	if err == nil || out.Receipt == nil || out.Receipt.Outcome != types.Rejected || calls != 1 {
		t.Fatal(out, err, calls)
	}
}
