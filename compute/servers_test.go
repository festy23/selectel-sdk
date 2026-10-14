package compute_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/compute"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servers"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func client(t *testing.T, wire transport, identities ...string) (*sdk.Client, *compute.Servers) {
	t.Helper()
	identity := "source"
	if len(identities) > 0 {
		identity = identities[0]
	}
	source := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: identity, Revision: "revision", Scheme: credentials.IAMPassword, Binding: types.Scope{AccountID: "account"}, Audience: "selectel", Username: types.Protect([]byte("user")), Password: types.Protect([]byte("synthetic-private-password"))}, nil
	})
	wrapped := transport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/identity/v3/auth/tokens" {
			raw, _ := json.Marshal(map[string]any{"token": map[string]any{"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), "project": map[string]any{"id": "project", "domain": map[string]string{"name": "account"}}, "catalog": []any{map[string]any{"type": "compute", "endpoints": []any{map[string]string{"id": "compute-endpoint", "interface": "public", "region": "pool", "url": "https://nova.test/v2.1/project"}}}}}})
			r := response(201, string(raw))
			r.Header.Set("X-Subject-Token", "synthetic-token")
			return r, nil
		}
		if req.URL.Host != "nova.test" || req.Header.Get("X-Auth-Token") == "" || req.Header.Get("OpenStack-API-Version") != "compute 2.1" {
			t.Error("endpoint/auth/microversion lost")
		}
		return wire(req)
	})
	c, e := sdk.New(sdk.Config{AccountID: "account", Credentials: source, Transport: wrapped, PollInterval: time.Millisecond, WaitTimeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Project("project")
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Location("region", "pool")
	if e != nil {
		t.Fatal(e)
	}
	return c, r.Compute().Servers
}
func request() compute.CreateRequest {
	return compute.CreateRequest{Options: servers.CreateOpts{Name: "vm", FlavorRef: "flavor", ImageRef: "image", Networks: []servers.Network{{UUID: "network"}}}}
}
func serverRef() types.ResourceRef {
	return types.ResourceRef{Scope: types.Scope{AccountID: "account", ProjectID: "project", Region: "region", Pool: "pool"}, Service: "compute", APIGeneration: "v2_1", Kind: "server", ID: "server-id", EndpointID: "compute-endpoint", EndpointURL: "https://nova.test/v2.1/project"}
}
func TestCreateWaitDeleteAndResume(t *testing.T) {
	reads := 0
	deleted := false
	mutations := 0
	c, s := client(t, func(r *http.Request) (*http.Response, error) {
		switch r.Method {
		case "POST":
			mutations++
			var body map[string]map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["server"]["name"] != "vm" {
				t.Error("native body missing")
			}
			return response(202, `{"server":{"id":"server-id","adminPass":"synthetic-private-result"}}`), nil
		case "GET":
			if deleted {
				return response(404, ""), nil
			}
			reads++
			state := "BUILD"
			if reads > 1 {
				state = "ACTIVE"
			}
			return response(200, `{"server":{"id":"server-id","tenant_id":"project","status":"`+state+`"}}`), nil
		case "DELETE":
			mutations++
			deleted = true
			return response(204, ""), nil
		}
		t.Fatal("unexpected request")
		return nil, nil
	})
	r, e := s.CreateAndWait(context.Background(), request())
	if e != nil || !r.Wait.Complete || r.Create.Receipt.Outcome != types.Accepted || reads != 2 {
		t.Fatal("create/wait failed", e)
	}
	if r.Create.Data.Server.AdminPass != "" || r.Create.Data.AdminPassword == nil || string(r.Create.Data.AdminPassword.Reveal()) != "synthetic-private-result" {
		t.Fatal("password not protected")
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "synthetic-private-result") || strings.Contains(fmt.Sprintf("%+v", r), "synthetic-private-result") {
		t.Fatal("password disclosed")
	}

	generic, e := c.Resource(r.Create.Data.Ref)
	if e != nil {
		t.Fatal("generic compute Resource dispatch failed", e)
	}
	observation, e := generic.Status(context.Background())
	if e != nil || observation.ProviderState != "ACTIVE" {
		t.Fatal("generic compute observation failed", e)
	}

	deletedResult, e := s.Delete(context.Background(), r.Create.Data.Ref)
	if e != nil {
		t.Fatal(e)
	}
	h, e := c.Resume(*deletedResult.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	wait, e := h.Wait(context.Background(), "server.deleted")
	if e != nil || !wait.Complete || mutations != 2 {
		t.Fatal("delete replayed or absent not confirmed")
	}
}
func TestUnknownCreateNeverResubmitsAndAcceptedMalformedBody(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		outcome types.Outcome
		code    types.Code
	}{{0, "", types.Unknown, types.UnknownOutcome}, {202, `{"server":{}}`, types.Accepted, types.ProtocolError}} {
		t.Run(string(tc.code), func(t *testing.T) {
			n := 0
			_, s := client(t, func(*http.Request) (*http.Response, error) {
				n++
				if tc.status == 0 {
					return nil, io.EOF
				}
				return response(tc.status, tc.body), nil
			})
			r, e := s.Create(context.Background(), request())
			if n != 1 || r.Receipt == nil || r.Receipt.Outcome != tc.outcome || types.ErrorCode(e) != tc.code || len(r.Receipt.ResourceRefs) != 0 {
				t.Fatal("mutation outcome lost/replayed", e)
			}
		})
	}
}
func TestUnknownDeleteRecoveryIsIndeterminate(t *testing.T) {
	writes := 0
	c, s := client(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			writes++
			return nil, io.EOF
		}
		return response(200, `{"server":{"id":"server-id","status":"ACTIVE"}}`), nil
	})
	r, e := s.Delete(context.Background(), serverRef())
	if types.ErrorCode(e) != types.UnknownOutcome {
		t.Fatal(e)
	}
	h, e := c.Resume(*r.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || recovered.ProofStatus != "indeterminate" || recovered.Receipt.Outcome != types.Unknown || writes != 1 {
		t.Fatal("resource observation invented mutation proof")
	}
}
func TestBoundedListCursorAndScopeChecks(t *testing.T) {
	n := 0
	_, s := client(t, func(r *http.Request) (*http.Response, error) {
		n++
		marker := r.URL.Query().Get("marker")
		if marker == "" {
			return response(200, `{"servers":[{"id":"one","tenant_id":"project","status":"ACTIVE"}],"servers_links":[{"rel":"next","href":"https://nova.test/v2.1/project/servers/detail?marker=one&limit=1"}]}`), nil
		}
		if marker != "one" {
			t.Error("marker changed")
		}
		return response(200, `{"servers":[{"id":"two","tenant_id":"project","status":"BUILD"}],"servers_links":[]}`), nil
	})
	first, e := s.Collect(context.Background(), servers.ListOpts{}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || first.Complete || first.NextCursor == nil || n != 1 {
		t.Fatal("truncated collection advertised complete")
	}
	second, e := s.Collect(context.Background(), servers.ListOpts{}, pagination.Options{Cursor: first.NextCursor})
	if e != nil || !second.Complete || second.Items[0].ID != "two" || n != 2 {
		t.Fatal("resume failed", e)
	}
	foreign := *first.NextCursor
	foreign.Scope.ProjectID = "another"
	_, e = s.Collect(context.Background(), servers.ListOpts{}, pagination.Options{Cursor: &foreign})
	if types.ErrorCode(e) != types.ScopeMismatch || n != 2 {
		t.Fatal("foreign cursor reached resource API")
	}
}
func TestNextLinkDoesNotRedirectCredentials(t *testing.T) {
	_, s := client(t, func(r *http.Request) (*http.Response, error) {
		return response(200, `{"servers":[],"servers_links":[{"rel":"next","href":"https://other.test/servers/detail?marker=one"}]}`), nil
	})
	_, e := s.ListPage(context.Background(), servers.ListOpts{}, pagination.Options{})
	if types.ErrorCode(e) != types.ProtocolError {
		t.Fatal("foreign next link accepted")
	}
}
func TestCreateValidationPreventsIO(t *testing.T) {
	n := 0
	_, s := client(t, func(*http.Request) (*http.Response, error) { n++; return nil, errors.New("must not send") })
	bad := request()
	bad.Options.Networks = nil
	r, e := s.Create(context.Background(), bad)
	if types.ErrorCode(e) != types.InvalidArgument || n != 0 || r.Receipt == nil || r.Receipt.Outcome != types.NotSent {
		t.Fatal("invalid create sent")
	}
	bad = request()
	bad.Options.Max = 2
	_, e = s.Create(context.Background(), bad)
	if types.ErrorCode(e) != types.CapabilityUnavailable || n != 0 {
		t.Fatal("multi-create sent")
	}
}

func TestProtectedCreateFieldsPreservePresenceAndEncodeRawBytesOnce(t *testing.T) {
	_, s := client(t, func(r *http.Request) (*http.Response, error) {
		var body map[string]map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("bad JSON")
		}
		password, present := body["server"]["adminPass"]
		if !present || password != "" {
			t.Fatal("explicit empty password omitted")
		}
		if body["server"]["user_data"] != base64.StdEncoding.EncodeToString([]byte("YmFzZQ==")) {
			t.Fatal("raw user data incorrectly detected as preencoded")
		}
		return response(202, `{"server":{"id":"server-id"}}`), nil
	})
	r := request()
	password := types.Protect([]byte{})
	data := types.Protect([]byte("YmFzZQ=="))
	r.AdminPassword = &password
	r.UserData = &data
	if _, e := s.Create(context.Background(), r); e != nil {
		t.Fatal(e)
	}
}

func TestResumeAllowsNewCredentialSourceForSameScope(t *testing.T) {
	_, s := client(t, func(*http.Request) (*http.Response, error) {
		return response(202, `{"server":{"id":"server-id"}}`), nil
	})
	created, e := s.Create(context.Background(), request())
	if e != nil {
		t.Fatal(e)
	}
	c, _ := client(t, func(*http.Request) (*http.Response, error) {
		return response(200, `{"server":{"id":"server-id","status":"ACTIVE"}}`), nil
	}, "new-explicit-source")
	h, e := c.Resume(*created.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	observation, e := h.Status(context.Background())
	if e != nil || observation.Classification != "succeeded" {
		t.Fatal("new valid source rejected", e)
	}
}
