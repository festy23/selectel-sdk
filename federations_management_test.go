package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/federations"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSAMLFederationLifecycle(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		status := 200
		body := `{"id":"f1","account_id":"test-account","name":"synthetic"}`
		if r.Method == "GET" && r.URL.Path == "/v1/federations/saml" {
			body = `{"federations":[{"id":"f1","account_id":"test-account","name":"synthetic"}]}`
		}
		if r.Method == "PATCH" || r.Method == "DELETE" {
			status = 204
			body = ""
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	api := c.Account().Federations().V1.SAML
	name, issuer, sso := "synthetic", "https://idp.example.invalid", "https://idp.example.invalid/saml"
	no := false
	created, e := api.Create(context.Background(), federations.CreateSAMLRequest{Name: &name, Issuer: &issuer, SSOURL: &sso, ForceAuthn: &no}, types.CallOptions{})
	if e != nil || created.Data.Ref == nil || created.Receipt.Outcome != types.Accepted {
		t.Fatalf("create %+v %v", created, e)
	}
	ref := *created.Data.Ref
	listed, e := api.Collect(context.Background(), pagination.Options{})
	if e != nil || !listed.Complete || len(listed.Items) != 1 {
		t.Fatalf("list %+v %v", listed, e)
	}
	got, e := api.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || got.Data.Ref == nil {
		t.Fatalf("get %+v %v", got, e)
	}
	updated, e := api.Update(context.Background(), ref, federations.UpdateSAMLRequest{ForceAuthn: &no}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatalf("update %+v %v", updated, e)
	}
	deleted, e := api.Delete(context.Background(), ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatalf("delete %+v %v", deleted, e)
	}
}

func TestOIDCFederationSecretAndNullableUpdate(t *testing.T) {
	secret := "synthetic-oidc-secret"
	var wire []map[string]json.RawMessage
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		fields := map[string]json.RawMessage{}
		_ = json.NewDecoder(r.Body).Decode(&fields)
		wire = append(wire, fields)
		status := 200
		body := `{"id":"o1","account_id":"test-account"}`
		if r.Method == "PATCH" {
			status = 204
			body = ""
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	name, id, issuer, authURL, tokenURL, jwksURL := "synthetic", "client", "https://idp.example.invalid", "https://idp.example.invalid/auth", "https://idp.example.invalid/token", "https://idp.example.invalid/keys"
	hours, _ := types.ParseInteger("8")
	protected := types.Protect([]byte(secret))
	in := federations.CreateOIDCRequest{Name: &name, ClientID: &id, Issuer: &issuer, AuthURL: &authURL, TokenURL: &tokenURL, JWKSURL: &jwksURL, SessionMaxAgeHours: &hours, ClientSecret: &protected}
	raw, _ := json.Marshal(in)
	if strings.Contains(string(raw), secret) || strings.Contains(fmt.Sprintf("%+v", in), secret) {
		t.Fatal("OIDC secret disclosed")
	}
	created, e := c.Account().Federations().V1.OIDC.Create(context.Background(), in, types.CallOptions{})
	if e != nil || created.Data.Ref == nil {
		t.Fatalf("create %+v %v", created, e)
	}
	no := false
	updated, e := c.Account().Federations().V1.OIDC.Update(context.Background(), *created.Data.Ref, federations.UpdateOIDCRequest{ClientSecret: types.Null[types.ProtectedValue](), Description: types.Value(""), AutoUsersCreation: types.Value(no), SessionMaxAgeHours: types.Null[types.Integer]()}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatalf("update %+v %v", updated, e)
	}
	if len(wire) != 2 || string(wire[0]["client_secret"]) != `"synthetic-oidc-secret"` || string(wire[1]["client_secret"]) != "null" || string(wire[1]["auto_users_creation"]) != "false" || string(wire[1]["description"]) != `""` || string(wire[1]["session_max_age_hours"]) != "null" {
		t.Fatal("nullable/secret wire semantics lost")
	}
}

func TestPublicFederationReadsNeverLookupOrSendCredentials(t *testing.T) {
	lookups := 0
	var requests []string
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		lookups++
		return credentials.Material{}, fmt.Errorf("synthetic credential lookup must not happen")
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Auth-Token") != "" {
			t.Fatal("public read sent credentials")
		}
		requests = append(requests, r.Method+" "+r.URL.Path)
		body := ""
		if r.Method == "GET" {
			body = `{"id":"f1","name":"public","alias":"public-alias"}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	preview, e := c.Account().Federations().V1.Preview(context.Background(), "public-alias", types.CallOptions{})
	if e != nil || preview.Data.ID == nil || *preview.Data.ID != "f1" {
		t.Fatalf("preview %+v %v", preview, e)
	}
	for _, kind := range []string{"saml_federation"} {
		ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "federations", APIGeneration: "v1", Kind: kind, ID: "f1", EndpointID: "selectel-federations", EndpointURL: "https://api.selectel.ru"}
		var out types.Result[struct{}]
		if kind == "saml_federation" {
			out, e = c.Account().Federations().V1.SAML.Exists(context.Background(), ref, types.CallOptions{})
		} else {
			out, e = c.Account().Federations().V1.OIDC.Exists(context.Background(), ref, types.CallOptions{})
		}
		if e != nil || out.Receipt != nil {
			t.Fatalf("exists %+v %v", out, e)
		}
	}
	if lookups != 0 || len(requests) != 2 {
		t.Fatalf("lookups=%d requests=%v", lookups, requests)
	}
	name, issuer, sso := "must-authenticate", "https://idp.example.invalid", "https://idp.example.invalid/saml"
	out, e := c.Account().Federations().V1.SAML.Create(context.Background(), federations.CreateSAMLRequest{Name: &name, Issuer: &issuer, SSOURL: &sso}, types.CallOptions{})
	if types.ErrorCode(e) != types.CredentialsUnavailable || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || lookups != 1 || len(requests) != 2 {
		t.Fatalf("mutation bypassed auth %+v %v", out, e)
	}
}

func TestSAMLCertificateLifecycleProtectsMaterialAndParent(t *testing.T) {
	material := "synthetic-pem-certificate"
	var wireData string
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		status := 200
		body := `{"id":"c1","account_id":"test-account","federation_id":"f1","data":"synthetic-pem-certificate"}`
		if r.Method == "POST" {
			var in struct {
				Data string `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			wireData = in.Data
		}
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/certificates") {
			body = `{"certificates":[{"id":"c1","account_id":"test-account","federation_id":"f1","data":"synthetic-pem-certificate"}]}`
		}
		if r.Method == "DELETE" {
			status = 204
			body = ""
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "federations", APIGeneration: "v1", Kind: "saml_federation", ID: "f1", EndpointID: "selectel-federations", EndpointURL: "https://api.selectel.ru"}
	name := "synthetic"
	data := types.Protect([]byte(material))
	api := c.Account().Federations().V1.SAML.Certificates
	created, e := api.Create(context.Background(), parent, federations.CreateCertificateRequest{Name: &name, Data: &data}, types.CallOptions{})
	if e != nil || created.Data.Ref == nil || created.Data.Ref.Parent == nil || created.Data.Ref.Parent.ID != "f1" || len(created.Receipt.ResourceRefs) != 2 || created.Receipt.ResourceRefs[1].Parent == nil || wireData != material {
		t.Fatalf("create %+v %v", created, e)
	}
	raw, _ := json.Marshal(created)
	if strings.Contains(string(raw), material) || strings.Contains(fmt.Sprintf("%+v", created), material) || created.Data.Data == nil || string(created.Data.Data.Reveal()) != material {
		t.Fatal("certificate protection failed")
	}
	ref := *created.Data.Ref
	listed, e := api.Collect(context.Background(), parent, pagination.Options{})
	if e != nil || !listed.Complete || len(listed.Items) != 1 || listed.Items[0].Ref.Parent.ID != "f1" {
		t.Fatalf("list %+v %v", listed, e)
	}
	got, e := api.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || got.Data.Ref.Parent.ID != "f1" {
		t.Fatalf("get %+v %v", got, e)
	}
	updated, e := api.Update(context.Background(), ref, federations.UpdateCertificateRequest{Name: &name}, types.CallOptions{})
	if e != nil || updated.Receipt.Outcome != types.Accepted {
		t.Fatalf("update %+v %v", updated, e)
	}
	deleted, e := api.Delete(context.Background(), ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatalf("delete %+v %v", deleted, e)
	}
}

func TestFederationGroupMappingsPreserveReplaceSemanticsAndExactTargets(t *testing.T) {
	for _, protocol := range []string{"saml", "oidc"} {
		t.Run(protocol, func(t *testing.T) {
			var seen []string
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				seen = append(seen, r.Method+" "+r.URL.EscapedPath())
				status := 204
				body := ""
				if r.Method == "GET" || (r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "group-mappings") && protocol == "saml") {
					status = 200
					body = `{"group_mappings":[{"internal_group_id":"g1","external_group_id":"idp/team"}]}`
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "federations", APIGeneration: "v1", Kind: protocol + "_federation", ID: "f1", EndpointID: "selectel-federations", EndpointURL: "https://api.selectel.ru"}
			api := c.Account().Federations().V1.SAML.GroupMappings
			if protocol == "oidc" {
				api = c.Account().Federations().V1.OIDC.GroupMappings
			}
			internal, external := "g1", "idp/team"
			replaced, e := api.Replace(context.Background(), parent, federations.GroupMappingsRequest{GroupMappings: []federations.GroupMapping{{InternalGroupID: &internal, ExternalGroupID: &external}}}, types.CallOptions{})
			if e != nil || replaced.Receipt.Outcome != types.Accepted {
				t.Fatalf("replace %+v %v", replaced, e)
			}
			listed, e := api.Collect(context.Background(), parent, pagination.Options{})
			if e != nil || !listed.Complete || len(listed.Items) != 1 || listed.Items[0].Ref == nil || listed.Items[0].Ref.Parent == nil || listed.Items[0].Ref.Parent.ID != "g1" || listed.Items[0].Ref.Parent.Parent.ID != "f1" {
				t.Fatalf("list %+v %v", listed, e)
			}
			ref := *listed.Items[0].Ref
			for _, call := range []func() (types.Result[struct{}], error){func() (types.Result[struct{}], error) { return api.Put(context.Background(), ref, types.CallOptions{}) }, func() (types.Result[struct{}], error) {
				return api.Exists(context.Background(), ref, types.CallOptions{})
			}, func() (types.Result[struct{}], error) {
				return api.Delete(context.Background(), ref, types.CallOptions{})
			}} {
				out, e := call()
				if e != nil {
					t.Fatalf("mapping %+v %v", out, e)
				}
			}
			if len(seen) != 5 || !strings.HasSuffix(seen[2], "/group-mappings/g1/external-groups/idp%2Fteam") {
				t.Fatalf("wire %v", seen)
			}
		})
	}
}

func TestFederationCertificateRejectsForeignParentBeforeIO(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })})
	if e != nil {
		t.Fatal(e)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "federations", APIGeneration: "v1", Kind: "saml_federation", ID: "f1", EndpointID: "foreign-endpoint", EndpointURL: "https://foreign.example.invalid"}
	ref := parent
	ref.Kind = "saml_certificate"
	ref.ID = "c1"
	ref.EndpointID = "selectel-federations"
	ref.EndpointURL = "https://api.selectel.ru"
	ref.Parent = &parent
	out, e := c.Account().Federations().V1.SAML.Certificates.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("foreign parent %+v %v calls=%d", out, e, calls)
	}
}

func TestOIDCCreationValidatesDocumentedBoundsBeforeIO(t *testing.T) {
	calls := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })})
	if e != nil {
		t.Fatal(e)
	}
	name, id, issuer, authURL, tokenURL, jwksURL := "synthetic", "client", "https://idp.example.invalid", "https://idp.example.invalid/auth", "https://idp.example.invalid/token", "https://idp.example.invalid/keys"
	zero, _ := types.ParseInteger("0")
	secret := types.Protect([]byte("synthetic-secret"))
	out, e := c.Account().Federations().V1.OIDC.Create(context.Background(), federations.CreateOIDCRequest{Name: &name, ClientID: &id, Issuer: &issuer, AuthURL: &authURL, TokenURL: &tokenURL, JWKSURL: &jwksURL, SessionMaxAgeHours: &zero, ClientSecret: &secret}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatalf("OIDC bounds %+v %v calls=%d", out, e, calls)
	}
}

func TestNullableOIDCSecretCannotLeakAsEncodedBytes(t *testing.T) {
	marker := "synthetic-nullable-oidc-secret"
	secret := types.Protect([]byte(marker))
	nullable := types.Value(secret)
	nested := federations.UpdateOIDCRequest{ClientSecret: nullable}
	decimal := fmt.Sprintf("%v", []byte(marker))
	hex := fmt.Sprintf("%#v", []byte(marker))
	for _, value := range []any{nullable, *nullable, nested, &nested} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			text := fmt.Sprintf(format, value)
			if strings.Contains(text, marker) || strings.Contains(text, decimal) || strings.Contains(text, hex) {
				t.Fatalf("nullable secret leaked through %s", format)
			}
		}
		encoded, _ := json.Marshal(value)
		if strings.Contains(string(encoded), marker) {
			t.Fatal("JSON disclosure")
		}
	}
	got, present := nullable.Value()
	if !present || string(got.Reveal()) != marker {
		t.Fatal("explicit caller-directed reveal lost")
	}
}

func TestFederationCreateDoesNotAdoptForeignAccountResponse(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"foreign-federation","account_id":"other-account"}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	name, issuer, sso := "synthetic", "https://idp.example.invalid", "https://idp.example.invalid/saml"
	out, e := c.Account().Federations().V1.SAML.Create(context.Background(), federations.CreateSAMLRequest{Name: &name, Issuer: &issuer, SSOURL: &sso}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 0 {
		t.Fatalf("foreign resource adopted %+v %v", out, e)
	}
}

func TestFederationMutationUnknownIsNotReplayedAndAcceptedRefSurvivesDecode(t *testing.T) {
	for _, tc := range []struct {
		name string
		lost bool
	}{{"lost response", true}, {"accepted malformed body", false}} {
		t.Run(tc.name, func(t *testing.T) {
			sent := 0
			c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, "auth/tokens") {
					return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
				}
				sent++
				if tc.lost {
					return nil, io.ErrUnexpectedEOF
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"f1","account_id":"test-account","name":false}`))}, nil
			})})
			if e != nil {
				t.Fatal(e)
			}
			name, issuer, sso := "synthetic", "https://idp.example.invalid", "https://idp.example.invalid/saml"
			out, e := c.Account().Federations().V1.SAML.Create(context.Background(), federations.CreateSAMLRequest{Name: &name, Issuer: &issuer, SSOURL: &sso}, types.CallOptions{})
			want := types.ProtocolError
			outcome := types.Accepted
			refs := 1
			if tc.lost {
				want = types.UnknownOutcome
				outcome = types.Unknown
				refs = 0
			}
			if types.ErrorCode(e) != want || out.Receipt == nil || out.Receipt.Outcome != outcome || len(out.Receipt.ResourceRefs) != refs || sent != 1 {
				t.Fatalf("mutation %+v %v sent=%d", out, e, sent)
			}
		})
	}
}

func TestFederationCatalogCoversAllPinnedOperationsWithExactAuth(t *testing.T) {
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if e != nil {
		t.Fatal(e)
	}
	ops, e := c.Catalog().List(catalog.Filter{Service: "federations"})
	if e != nil || len(ops) != 28 {
		t.Fatalf("inventory %d %v", len(ops), e)
	}
	for _, op := range ops {
		auth := types.IAMAccount
		if op.OperationID == "federations.v1.saml.exists" || op.OperationID == "federations.v1.preview.get" {
			auth = types.ProtocolSpecific
		}
		if op.Auth.Scheme != auth || op.GoMethod == nil || op.Implementation.Status != "implemented" || op.APIGeneration != "v1" || op.RequestBindingRef == nil || op.ResponseBindingRef == nil || op.ErrorBindingRef == nil {
			t.Fatalf("binding %+v", op)
		}
	}
	if c.Catalog().Summary().InventoryComplete || c.Catalog().Summary().CoverageRatio != nil {
		t.Fatal("global source inventory overclaimed")
	}
}

func TestOIDCReadsExistenceAndDeleteRequireAccountToken(t *testing.T) {
	var seen []string
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		if r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal("OIDC read missing account auth")
		}
		seen = append(seen, r.Method+" "+r.URL.Path)
		status := 200
		body := `{"id":"o1","account_id":"test-account"}`
		if r.URL.Path == "/v1/federations/oidc" {
			body = `{"federations":[{"id":"o1","account_id":"test-account"}]}`
		}
		if r.Method == "HEAD" {
			body = ""
		}
		if r.Method == "DELETE" {
			status = 204
			body = ""
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	api := c.Account().Federations().V1.OIDC
	listed, e := api.Collect(context.Background(), pagination.Options{})
	if e != nil || len(listed.Items) != 1 || listed.Items[0].Ref == nil {
		t.Fatalf("list %+v %v", listed, e)
	}
	ref := *listed.Items[0].Ref
	got, e := api.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || got.Data.Ref == nil {
		t.Fatalf("get %+v %v", got, e)
	}
	exists, e := api.Exists(context.Background(), ref, types.CallOptions{})
	if e != nil || exists.Receipt != nil {
		t.Fatalf("exists %+v %v", exists, e)
	}
	deleted, e := api.Delete(context.Background(), ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted {
		t.Fatalf("delete %+v %v", deleted, e)
	}
	if strings.Join(seen, "\n") != "GET /v1/federations/oidc\nGET /v1/federations/oidc/o1\nHEAD /v1/federations/oidc/o1\nDELETE /v1/federations/oidc/o1" {
		t.Fatalf("wire %v", seen)
	}
}
