package registry_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/registry"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type managementTransport func(*http.Request) (*http.Response, error)

func (f managementTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func managementResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func managementClient(t *testing.T, wire managementTransport, overrides ...map[types.EndpointKey]types.EndpointBinding) (*sdk.Client, *registry.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	var selected map[types.EndpointKey]types.EndpointBinding
	if len(overrides) > 0 {
		selected = overrides[0]
	}
	c, e := sdk.New(sdk.Config{EndpointOverrides: selected, AccountID: "a", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Scope != scope || r.Service != "registry" || r.Mode != types.IAMProject {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: managementTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			raw, _ := json.Marshal(map[string]any{"token": map[string]any{"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "user": map[string]string{"id": "subject"}, "catalog": []any{}, "project": map[string]any{"id": "p", "domain": map[string]string{"name": "a"}}}})
			v := managementResponse(200, string(raw))
			v.Header.Set("X-Subject-Token", "synthetic-token")
			return v, nil
		}
		if r.URL.Host != "cr.selcloud.ru" || r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal("wrong registry auth/endpoint", r.URL)
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("p")
	return c, p.Registry()
}

func TestLegacyRegistryTokenCreateProtectsOutputAndPreservesFalse(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/v1/token" || r.URL.Query().Get("ttl") != "1y" || r.URL.Query().Get("docker-config") != "false" {
			t.Fatal(r.Method, r.URL)
		}
		return managementResponse(201, `{"token":"synthetic-legacy-secret","expireAt":9007199254740993,"expireIn":31536000}`), nil
	})
	ttl, dc := "1y", false
	v, e := c.V1.Tokens().Create(context.Background(), registry.LegacyTokenCreateOptions{TTL: &ttl, DockerConfig: &dc}, types.CallOptions{})
	if e != nil || v.Receipt == nil || v.Receipt.Outcome != types.Accepted || len(v.Receipt.ResourceRefs) != 0 || v.Data.Token == nil || string(v.Data.Token.Reveal()) != "synthetic-legacy-secret" || v.Data.ExpireAt.String() != "9007199254740993" {
		t.Fatal(v, e)
	}
	encoded, _ := json.Marshal(v)
	for _, safe := range []string{string(encoded), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprintf("%+v", *v.Data.Token), fmt.Sprintf("%#v", v.Data.Token)} {
		if strings.Contains(safe, "synthetic-legacy-secret") || strings.Contains(safe, "115 121 110 116 104") || strings.Contains(safe, "0x73, 0x79, 0x6e, 0x74, 0x68") {
			t.Fatal("secret disclosure")
		}
	}
	ttl = "bad"
	bad, e := c.V1.Tokens().Create(context.Background(), registry.LegacyTokenCreateOptions{TTL: &ttl}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || bad.Receipt == nil || bad.Receipt.Outcome != types.NotSent || calls != 1 {
		t.Fatal(bad, e, calls)
	}
}

func TestLegacyRegistryTokenLifecycleKeepsCredentialOutOfRefsAndReceipts(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.EscapedPath() != "/api/v1/token/legacy%2Fsynthetic%3F%23" && r.URL.EscapedPath() != "/api/v1/token/legacy%2Fsynthetic%3F%23/refresh" {
			t.Fatal(r.URL)
		}
		switch r.Method {
		case "GET":
			return managementResponse(200, `{"expireAt":9007199254740993,"expireIn":0}`), nil
		case "POST":
			if !strings.HasSuffix(r.URL.Path, "/refresh") {
				t.Fatal(r.URL)
			}
			return managementResponse(200, `{"expireAt":9007199254740994,"expireIn":12}`), nil
		case "DELETE":
			return managementResponse(204, ``), nil
		}
		t.Fatal(r.Method)
		return nil, nil
	})
	ctx := context.Background()
	secret := types.Protect([]byte("legacy/synthetic?#"))
	info, e := c.V1.Tokens().Get(ctx, secret, types.CallOptions{})
	if e != nil || info.Data.ExpireAt.String() != "9007199254740993" || info.Receipt != nil {
		t.Fatal(info, e)
	}
	refreshed, e := c.V1.Tokens().Refresh(ctx, secret, types.CallOptions{})
	if e != nil || refreshed.Data.ExpireIn.String() != "12" || refreshed.Data.Token != nil || refreshed.Receipt == nil || refreshed.Receipt.Outcome != types.Accepted {
		t.Fatal(refreshed, e)
	}
	revoked, e := c.V1.Tokens().Revoke(ctx, secret, types.CallOptions{})
	if e != nil || revoked.Receipt == nil || revoked.Receipt.Outcome != types.Accepted || len(revoked.Receipt.ResourceRefs) != 0 || calls != 3 {
		t.Fatal(revoked, e, calls)
	}
	for _, v := range []any{info, refreshed, revoked} {
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), "legacy/synthetic") || strings.Contains(fmt.Sprintf("%#v", v), "legacy/synthetic") {
			t.Fatal("credential persisted")
		}
	}
	bad, e := c.V1.Tokens().Revoke(ctx, types.Protect(nil), types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || bad.Receipt == nil || bad.Receipt.Outcome != types.NotSent || calls != 3 {
		t.Fatal(bad, e, calls)
	}
}

func TestLegacyRegistryTokenDockerConfigIsExplicitProtectedJSON(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api/v1/token" || r.URL.Query().Get("docker-config") != "true" || r.URL.Query().Get("ttl") != "12h" {
			t.Fatal(r.URL)
		}
		return managementResponse(201, `{"auths":{"cr.selcloud.ru":{"auth":"synthetic-base64-credential"}}}`), nil
	})
	ttl := "12h"
	v, e := c.V1.Tokens().CreateDockerConfig(context.Background(), &ttl, types.CallOptions{})
	if e != nil || v.Receipt == nil || v.Receipt.Outcome != types.Accepted || !strings.Contains(string(v.Data.Reveal()), "synthetic-base64-credential") {
		t.Fatal(v, e)
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "synthetic-base64-credential") || strings.Contains(fmt.Sprintf("%#v", v), "synthetic-base64-credential") || calls != 1 {
		t.Fatal("unprotected Docker config")
	}
}

func TestRegistryV2TokenCreateGetHaveExactIDAndProtectedSecret(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v2/tokens":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]json.RawMessage
			if json.Unmarshal(raw, &body) != nil || string(body["scope"]) != `{"allRegistries":false,"modeRW":false,"registryIds":[]}` || string(body["expiration"]) != `{"isSet":false}` || r.URL.Query().Get("docker-config") != "false" || len(body) != 3 {
				t.Fatal(string(raw), r.URL)
			}
			return managementResponse(201, `{"id":"tok","name":"name","createdAt":"2025-01-02T03:00:00+03:00","expiration":{"isSet":false},"scope":{"allRegistries":false,"modeRW":false,"registryIds":[]},"status":"active","token":"synthetic-v2-secret"}`), nil
		case "GET /api/v2/tokens/tok":
			return managementResponse(200, `{"id":"tok","name":"name","createdAt":"2025-01-02T00:00:00Z","expiration":{"isSet":false},"scope":{"allRegistries":false,"modeRW":false},"status":"active"}`), nil
		}
		t.Fatal(r.Method, r.URL)
		return nil, nil
	})
	no := false
	ids := []string{}
	name := "name"
	in := registry.TokenCreateRequest{Name: &name, Scope: &registry.TokenScope{AllRegistries: &no, ModeRW: &no, RegistryIDs: &ids}, Expiration: &registry.TokenExpiration{IsSet: &no}, DockerConfig: &no}
	ctx := context.Background()
	created, e := c.V2.Tokens().Create(ctx, in, types.CallOptions{})
	if e != nil || created.Receipt == nil || created.Receipt.Outcome != types.Accepted || len(created.Receipt.ResourceRefs) != 1 || created.Data.Ref == nil || created.Data.Ref.ID != "tok" || created.Data.Ref.Kind != "token" || created.Data.Ref.APIGeneration != "v2" || string(created.Data.Token.Reveal()) != "synthetic-v2-secret" || created.Data.CreatedAt.Format(time.RFC3339) != "2025-01-02T00:00:00Z" {
		t.Fatal(created, e)
	}
	got, e := c.V2.Tokens().Get(ctx, *created.Data.Ref, types.CallOptions{})
	if e != nil || got.Data.Token != nil || got.Data.Ref.ID != "tok" || got.Receipt != nil || calls != 2 {
		t.Fatal(got, e, calls)
	}
	raw, _ := json.Marshal(created)
	if strings.Contains(string(raw), "synthetic-v2-secret") || strings.Contains(fmt.Sprintf("%#v", created), "synthetic-v2-secret") {
		t.Fatal("secret disclosure")
	}
	bad := *created.Data.Ref
	bad.APIGeneration = "v1"
	_, e = c.V2.Tokens().Get(ctx, bad, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestRegistryV2RefreshRegenerateRevokeAndDeleteAreDistinct(t *testing.T) {
	calls := 0
	fixture := `{"id":"tok","name":"name","createdAt":"2025-01-02T00:00:00Z","expiration":{"isSet":false},"scope":{"allRegistries":true,"modeRW":false},"status":"active"}`
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v2/tokens":
			return managementResponse(201, fixture), nil
		case "POST /api/v2/tokens/tok/refresh":
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != `{"expiration":{"isSet":false}}` {
				t.Fatal(string(raw))
			}
			return managementResponse(200, fixture), nil
		case "POST /api/v2/tokens/tok/regenerate":
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != `{"expiration":{"isSet":false}}` {
				t.Fatal(string(raw))
			}
			return managementResponse(200, strings.TrimSuffix(fixture, "}")+`,"token":"replacement-secret"}`), nil
		case "PATCH /api/v2/tokens/tok":
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != `{"name":""}` {
				t.Fatal(string(raw))
			}
			return managementResponse(200, fixture), nil
		case "POST /api/v2/tokens/tok/revoke", "DELETE /api/v2/tokens/tok":
			return managementResponse(204, ``), nil
		}
		t.Fatal(r.Method, r.URL)
		return nil, nil
	})
	ctx := context.Background()
	no, yes := false, true
	v, e := c.V2.Tokens().Create(ctx, registry.TokenCreateRequest{Expiration: &registry.TokenExpiration{IsSet: &no}, Scope: &registry.TokenScope{AllRegistries: &yes, ModeRW: &no}}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	ref := *v.Data.Ref
	refreshed, e := c.V2.Tokens().Refresh(ctx, ref, registry.TokenRefreshRequest{Expiration: &registry.TokenExpiration{IsSet: &no}}, types.CallOptions{})
	if e != nil || refreshed.Data.Token != nil || refreshed.Receipt.OperationID != "registry.v2.tokens_refresh.create" {
		t.Fatal(refreshed, e)
	}
	regenerated, e := c.V2.Tokens().Regenerate(ctx, ref, registry.TokenRegenerateRequest{Expiration: &registry.TokenExpiration{IsSet: &no}}, types.CallOptions{})
	if e != nil || regenerated.Data.Token == nil || string(regenerated.Data.Token.Reveal()) != "replacement-secret" || regenerated.Receipt.OperationID != "registry.v2.tokens_regenerate.create" {
		t.Fatal(regenerated, e)
	}
	empty := ""
	updated, e := c.V2.Tokens().Update(ctx, ref, registry.TokenUpdateRequest{Name: &empty}, types.CallOptions{})
	if e != nil || updated.Receipt.OperationID != "registry.v2.tokens.update" {
		t.Fatal(updated, e)
	}
	revoked, e := c.V2.Tokens().Revoke(ctx, ref, types.CallOptions{})
	if e != nil || revoked.Receipt.OperationID != "registry.v2.tokens_revoke.create" {
		t.Fatal(revoked, e)
	}
	deleted, e := c.V2.Tokens().Delete(ctx, ref, types.CallOptions{})
	if e != nil || deleted.Receipt.OperationID != "registry.v2.tokens.delete" || calls != 6 {
		t.Fatal(deleted, e, calls)
	}
}

func TestRegistryV2TokenListSnapshotsFiltersAndPreservesCounts(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		q := r.URL.Query()
		if r.URL.Path != "/api/v2/tokens" || q.Get("search") != "original" || q.Get("sort_field") != "lastUsedAt" || q.Get("sort_type") != "desc" || q.Get("scope_mode") != "ro" || q.Get("limit") != "1" {
			t.Fatal(r.URL)
		}
		if calls == 1 {
			if q.Get("offset") != "5" {
				t.Fatal(r.URL)
			}
			return managementResponse(200, `{"tokens":[{"id":"tok","name":"name","createdAt":"2025-01-02T00:00:00Z","expiration":{"isSet":false},"scope":{"allRegistries":true,"modeRW":false},"status":"active"}],"totalCount":99}`), nil
		}
		if q.Get("offset") != "6" {
			t.Fatal(r.URL)
		}
		return managementResponse(200, `{"tokens":[],"totalCount":99}`), nil
	})
	search, sort, order, mode := "original", "lastUsedAt", "desc", "ro"
	offset, _ := types.ParseInteger("5")
	f := registry.TokenListOptions{Search: &search, SortField: &sort, SortType: &order, ScopeMode: &mode, Offset: &offset}
	it, e := c.V2.Tokens().Iterate(f, pagination.Options{PageSize: 1})
	if e != nil {
		t.Fatal(e)
	}
	search, sort, order, mode = "changed", "name", "asc", "rw"
	first, e := it.Next(context.Background())
	if e != nil || first.Done || len(first.Items) != 1 || first.Items[0].Ref.ID != "tok" || first.Items[0].Ref.APIGeneration != "v2" || first.Provider.TotalCount.String() != "99" {
		t.Fatal(first, e)
	}
	last, e := it.Next(context.Background())
	if e != nil || !last.Done || len(last.Items) != 0 || last.Provider.TotalCount.String() != "99" || calls != 2 {
		t.Fatal(last, e, calls)
	}
	_, e = it.Next(context.Background())
	if e != io.EOF || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestRegistryV2DockerConfigHasNoInventedTokenRef(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.URL.Path != "/api/v2/tokens" || r.URL.Query().Get("docker-config") != "true" || strings.Contains(string(raw), "DockerConfig") {
			t.Fatal(r.Method, r.URL, string(raw))
		}
		return managementResponse(201, `{"auths":{"cr.selcloud.ru":{"auth":"synthetic-config"}}}`), nil
	})
	no, yes := false, true
	v, e := c.V2.Tokens().CreateDockerConfig(context.Background(), registry.TokenCreateRequest{Scope: &registry.TokenScope{AllRegistries: &yes, ModeRW: &no}, Expiration: &registry.TokenExpiration{IsSet: &no}}, types.CallOptions{})
	if e != nil || v.Receipt == nil || v.Receipt.Outcome != types.Accepted || len(v.Receipt.ResourceRefs) != 0 || !strings.Contains(string(v.Data.Reveal()), "synthetic-config") || calls != 1 {
		t.Fatal(v, e, calls)
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "synthetic-config") || strings.Contains(fmt.Sprintf("%+v", v), "synthetic-config") {
		t.Fatal("unprotected output")
	}
}

func TestRegistryTokenListRetainsValidatedPrefixAndStopsAfterError(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return managementResponse(200, `{"tokens":[{"id":"ok","name":"name","createdAt":"2025-01-02T00:00:00Z","expiration":{"isSet":false},"scope":{"allRegistries":true,"modeRW":false},"status":"active"},{"id":"bad","name":"name","createdAt":"no-timezone","expiration":{"isSet":false},"scope":{"allRegistries":true,"modeRW":false},"status":"active"}],"totalCount":2}`), nil
	})
	it, e := c.V2.Tokens().Iterate(registry.TokenListOptions{}, pagination.Options{})
	if e != nil {
		t.Fatal(e)
	}
	page, e := it.Next(context.Background())
	if types.ErrorCode(e) != types.ProtocolError || len(page.Items) != 1 || page.Items[0].Ref.ID != "ok" || page.Done || page.Provider.TotalCount.String() != "2" {
		t.Fatal(page, e)
	}
	_, e = it.Next(context.Background())
	if types.ErrorCode(e) != types.ProtocolError || calls != 1 {
		t.Fatal(e, calls)
	}
}

func TestRegistryV2TokenRecoveryUsesExactIDWithoutReplayingRegenerate(t *testing.T) {
	calls := 0
	root, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v2/tokens":
			return managementResponse(201, `{"id":"tok","name":"name","createdAt":"2025-01-02T00:00:00Z","expiration":{"isSet":false},"scope":{"allRegistries":true,"modeRW":false},"status":"active"}`), nil
		case "POST /api/v2/tokens/tok/regenerate":
			return nil, io.ErrUnexpectedEOF
		case "GET /api/v2/tokens/tok":
			return managementResponse(200, `{"id":"tok","name":"name","createdAt":"2025-01-02T00:00:00Z","expiration":{"isSet":false},"scope":{"allRegistries":true,"modeRW":false},"status":"active"}`), nil
		}
		t.Fatal(r.Method, r.URL)
		return nil, nil
	})
	ctx := context.Background()
	no, yes := false, true
	created, e := c.V2.Tokens().Create(ctx, registry.TokenCreateRequest{Scope: &registry.TokenScope{AllRegistries: &yes, ModeRW: &no}, Expiration: &registry.TokenExpiration{IsSet: &no}}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	ref := *created.Data.Ref
	unknown, e := c.V2.Tokens().Regenerate(ctx, ref, registry.TokenRegenerateRequest{Expiration: &registry.TokenExpiration{IsSet: &no}}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || unknown.Receipt == nil || unknown.Receipt.Outcome != types.Unknown || len(unknown.Receipt.ResourceRefs) != 1 || unknown.Receipt.ResourceRefs[0].ID != "tok" || calls != 2 {
		t.Fatal(unknown, e, calls)
	}
	h, e := root.Resume(*unknown.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	seen, e := h.Recover(ctx)
	if types.ErrorCode(e) != types.UnknownOutcome || seen.ProofStatus != "indeterminate" || seen.Observation == nil || seen.Observation.Ref.ID != "tok" || seen.Observation.ProviderState != "active" || calls != 3 {
		t.Fatal(seen, e, calls)
	}
	h, e = root.Resource(ref)
	if e != nil {
		t.Fatal(e)
	}
	s, e := h.Status(ctx)
	if e != nil || s.Classification != "succeeded" || s.ProviderState != "active" || calls != 4 {
		t.Fatal(s, e, calls)
	}
	notSent, e := c.V1.Tokens().Revoke(ctx, types.Protect(nil), types.CallOptions{})
	if e == nil || notSent.Receipt == nil {
		t.Fatal(e)
	}
	h, e = root.Resume(*notSent.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	terminal, e := h.Recover(ctx)
	if e != nil || terminal.ProofStatus != "conclusive" || calls != 4 {
		t.Fatal(terminal, e, calls)
	}
}

func TestRegistryTokenMalformedAcceptedResponseRetainsOnlyTrustworthyID(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		refs       int
	}{{"valid_id_malformed_field", `{"id":"trusted","name":42}`, 1}, {"empty_id", `{"id":"","name":"n"}`, 0}, {"malformed_id", `{"id":42,"name":"n"}`, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, c := managementClient(t, func(r *http.Request) (*http.Response, error) { calls++; return managementResponse(201, tc.body), nil })
			no := false
			v, e := c.V2.Tokens().Create(context.Background(), registry.TokenCreateRequest{Scope: &registry.TokenScope{AllRegistries: &no, ModeRW: &no}, Expiration: &registry.TokenExpiration{IsSet: &no}}, types.CallOptions{})
			if types.ErrorCode(e) != types.ProtocolError || v.Receipt == nil || v.Receipt.Outcome != types.Accepted || len(v.Receipt.ResourceRefs) != tc.refs || calls != 1 {
				t.Fatal(v, e, calls)
			}
			if tc.refs == 1 && v.Receipt.ResourceRefs[0].ID != "trusted" {
				t.Fatal(v)
			}
		})
	}
}

func TestRegistryTokenCursorRejectsChangedFiltersAndEndpointBeforeIO(t *testing.T) {
	calls := 0
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return managementResponse(200, `{"tokens":[{"id":"tok","name":"name","createdAt":"2025-01-02T00:00:00Z","expiration":{"isSet":false},"scope":{"allRegistries":true,"modeRW":false},"status":"active"}],"totalCount":10}`), nil
	})
	search := "original"
	p, e := c.V2.Tokens().ListPage(context.Background(), registry.TokenListOptions{Search: &search}, pagination.Options{PageSize: 1, MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || p.Done || len(p.Items) != 1 || p.NextCursor == nil {
		t.Fatal(p, e)
	}
	search = "changed"
	_, e = c.V2.Tokens().Iterate(registry.TokenListOptions{Search: &search}, pagination.Options{Cursor: p.NextCursor})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatal(e, calls)
	}
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	overrides := map[types.EndpointKey]types.EndpointBinding{{Service: "registry", APIGeneration: "v2", Scope: scope}: {ID: "selectel-registry", URL: "https://cr.selcloud.ru/changed-api", APIGeneration: "v2", Scope: scope, Audience: "selectel"}}
	_, other := managementClient(t, func(r *http.Request) (*http.Response, error) { t.Fatal("wrong target I/O", r.URL); return nil, nil }, overrides)
	search = "original"
	_, e = other.V2.Tokens().Iterate(registry.TokenListOptions{Search: &search}, pagination.Options{Cursor: p.NextCursor})
	if types.ErrorCode(e) != types.ScopeMismatch {
		t.Fatal(e)
	}
	_, e = other.V2.Tokens().Get(context.Background(), *p.Items[0].Ref, types.CallOptions{})
	if types.ErrorCode(e) != types.EndpointUnavailable {
		t.Fatal(e)
	}
}

func TestLegacyUnknownTokenMutationNeverPersistsSecretOrReplays(t *testing.T) {
	calls := 0
	root, c := managementClient(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })
	v, e := c.V1.Tokens().Refresh(context.Background(), types.Protect([]byte("synthetic-path-secret")), types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || v.Receipt == nil || v.Receipt.Outcome != types.Unknown || len(v.Receipt.ResourceRefs) != 0 || calls != 1 {
		t.Fatal(v, e, calls)
	}
	raw, _ := json.Marshal(v.Receipt)
	if strings.Contains(string(raw), "synthetic-path-secret") || strings.Contains(fmt.Sprintf("%+v", e), "synthetic-path-secret") {
		t.Fatal("secret persisted")
	}
	_, e = root.Resume(*v.Receipt)
	if types.ErrorCode(e) != types.RecoveryUnavailable || calls != 1 {
		t.Fatal(e, calls)
	}
}

func TestRegistryTokenExpirationRequestUsesUTCWithoutMutatingCaller(t *testing.T) {
	calls := 0
	fixture := `{"id":"tok","name":"name","createdAt":"2025-01-02T00:00:00Z","expiration":{"isSet":true,"expiresAt":"2030-01-02T03:04:05.123+03:00"},"scope":{"allRegistries":true,"modeRW":false},"status":"active"}`
	_, c := managementClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return managementResponse(201, fixture), nil
		}
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/api/v2/tokens/tok/refresh" || string(raw) != `{"expiration":{"expiresAt":"2030-01-02T00:04:05.123Z","isSet":true}}` {
			t.Fatal(string(raw), r.URL)
		}
		return managementResponse(200, fixture), nil
	})
	ctx := context.Background()
	yes, no := true, false
	v, e := c.V2.Tokens().Create(ctx, registry.TokenCreateRequest{Scope: &registry.TokenScope{AllRegistries: &yes, ModeRW: &no}, Expiration: &registry.TokenExpiration{IsSet: &no}}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	stamp, _ := time.Parse(time.RFC3339Nano, "2030-01-02T03:04:05.123+03:00")
	out, e := c.V2.Tokens().Refresh(ctx, *v.Data.Ref, registry.TokenRefreshRequest{Expiration: &registry.TokenExpiration{IsSet: &yes, ExpiresAt: &stamp}}, types.CallOptions{})
	if e != nil || out.Data.Expiration.ExpiresAt.Format(time.RFC3339Nano) != "2030-01-02T00:04:05.123Z" || stamp.Format(time.RFC3339Nano) != "2030-01-02T03:04:05.123+03:00" || calls != 2 {
		t.Fatal(out, e, calls)
	}
}
