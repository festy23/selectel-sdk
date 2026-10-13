package secrets_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/secrets"
	"github.com/festy23/selectel-sdk/types"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func client(t *testing.T, transport transportFunc) *secrets.Client {
	t.Helper()
	source := credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "test-source", Revision: "revision", Scheme: credentials.IAMPassword, Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Username: types.Protect([]byte("test-user")), Password: types.Protect([]byte("synthetic-password"))}, nil
	})
	rt := transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/identity/v3/auth/tokens" {
			body, _ := json.Marshal(map[string]any{"token": map[string]any{"expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), "project": map[string]any{"id": "test-project", "domain": map[string]string{"name": "test-account"}}}})
			r := response(201, string(body))
			r.Header.Set("X-Subject-Token", "synthetic-token")
			return r, nil
		}
		if req.Header.Get("X-Auth-Token") == "" {
			t.Error("project authentication missing")
		}
		return transport(req)
	})
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source, Transport: rt})
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.Project("test-project")
	if err != nil {
		t.Fatal(err)
	}
	return p.Secrets()
}

func TestMetadataReadNeverFetchesSecretValue(t *testing.T) {
	paths := []string{}
	c := client(t, func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if req.URL.Path != "/secrets-manager/v1/key/versions" {
			t.Error("metadata read fetched payload endpoint")
		}
		return response(200, `{"name":"key","description":"description","created_at":"provider-time","versions":[{"version_id":900719925474099312345,"is_current":true,"created_at":"provider-time"}]}`), nil
	})
	r, err := c.GetMetadata(context.Background(), "key")
	if err != nil || r.Data.Versions[0].ID.String() != "900719925474099312345" || len(paths) != 1 {
		t.Fatal("version metadata not preserved")
	}
	if r.Data.Ref.Scope.ProjectID != "test-project" {
		t.Fatal("reference lost project scope")
	}
}

func TestReadValueRequiresExplicitDisclosure(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("synthetic-private-value"))
	c := client(t, func(req *http.Request) (*http.Response, error) {
		return response(200, `{"name":"key","version":{"version_id":1,"value":"`+encoded+`","is_current":true}}`), nil
	})
	r, err := c.ReadValue(context.Background(), "key")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "private-value") || strings.Contains(string(raw), encoded) {
		t.Fatal("secret serialized")
	}
	if string(r.Data.Version.Value.Reveal()) != "synthetic-private-value" {
		t.Fatal("explicit access changed value")
	}
}

func TestSecretCreateVersionUpdateActivateDelete(t *testing.T) {
	methods := []string{}
	c := client(t, func(req *http.Request) (*http.Response, error) {
		methods = append(methods, req.Method+" "+req.URL.EscapedPath())
		if req.Method == "POST" && !strings.HasSuffix(req.URL.Path, "/activate") {
			var b map[string]string
			if json.NewDecoder(req.Body).Decode(&b) != nil {
				t.Error("bad body")
			}
			value, err := base64.StdEncoding.DecodeString(b["value"])
			if err != nil || string(value) != "synthetic-private-value" {
				t.Error("wire value not encoded explicitly")
			}
			if strings.HasSuffix(req.URL.Path, "/versions") && req.URL.Query().Get("activate") != "false" {
				t.Error("explicit false activation lost")
			}
			return response(200, `{"name":"key","version":{"version_id":1,"value":"`+b["value"]+`"}}`), nil
		}
		return response(204, ""), nil
	})
	value := types.Protect([]byte("synthetic-private-value"))
	r, err := c.Create(context.Background(), secrets.CreateRequest{Name: "key", Value: &value})
	if err != nil || r.Receipt.Outcome != types.Accepted || len(r.Receipt.ResourceRefs) != 1 {
		t.Fatal("create did not preserve acknowledgement")
	}
	ref := r.Data.Ref
	activate := false
	v, err := c.CreateVersion(context.Background(), secrets.CreateVersionRequest{Name: "key", Value: &value, Activate: &activate})
	if err != nil || v.Receipt.Outcome != types.Accepted {
		t.Fatal("version create failed")
	}
	desc := "updated"
	if r, err := c.Update(context.Background(), secrets.UpdateRequest{Ref: ref, Description: &desc}); err != nil || r.Receipt.Outcome != types.Accepted {
		t.Fatal("update failed")
	}
	id, _ := types.ParseInteger("1")
	if r, err := c.ActivateVersion(context.Background(), ref, id); err != nil || r.Receipt.Outcome != types.Accepted {
		t.Fatal("activate failed")
	}
	if r, err := c.Delete(context.Background(), ref); err != nil || r.Receipt.Outcome != types.Accepted {
		t.Fatal("delete failed")
	}
	if len(methods) != 5 {
		t.Fatal("unexpected extra mutation or cleanup")
	}
}

func TestListIsBoundedAndCursorDoesNotCrossProject(t *testing.T) {
	calls := 0
	c := client(t, func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Query().Get("list") != "true" {
			t.Error("required query missing")
		}
		return response(200, `{"keys":[{"name":"one","type":"secret"},{"name":"two","type":"secret"},{"name":"three","type":"secret"}]}`), nil
	})
	first, err := c.ListMetadata(context.Background(), pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || first.Complete || len(first.Items) != 1 {
		t.Fatal("limit hidden")
	}
	rest, err := c.ListMetadata(context.Background(), pagination.Options{Cursor: first.NextCursor, MaxItems: 2})
	if err != nil || !rest.Complete || len(rest.Items) != 2 || *rest.Items[0].Name != "two" {
		t.Fatal("continuation skipped items")
	}
	cursor := *first.NextCursor
	cursor.Scope.ProjectID = "different"
	if _, err := c.ListMetadata(context.Background(), pagination.Options{Cursor: &cursor}); types.ErrorCode(err) != types.ScopeMismatch || calls != 2 {
		t.Fatal("foreign cursor performed I/O")
	}
}

func TestKeyIsEscapedOnceAndVersionIsExact(t *testing.T) {
	c := client(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.EscapedPath() != "/secrets-manager/v1/name%2Fwith%25escape/versions/900719925474099312345" {
			t.Error("key or id changed")
		}
		return response(200, `{"name":"name/with%escape","version":{"version_id":900719925474099312345,"value":""}}`), nil
	})
	id, _ := types.ParseInteger("900719925474099312345")
	r, err := c.ReadVersion(context.Background(), "name/with%escape", id)
	if err != nil || r.Data.Version == nil || !r.Data.Version.Value.Empty() {
		t.Fatal("exact version read failed")
	}
}

func TestMalformedPayloadAndDeniedDeleteAreVisible(t *testing.T) {
	c := client(t, func(req *http.Request) (*http.Response, error) {
		if req.Method == "GET" {
			return response(200, `{"name":"key","version":{"value":"not-base64"}}`), nil
		}
		return response(403, `{"error_text":"synthetic-private-value"}`), nil
	})
	if _, err := c.ReadValue(context.Background(), "key"); types.ErrorCode(err) != types.ProtocolError {
		t.Fatal("bad base64 accepted")
	}
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account", ProjectID: "test-project"}, Service: "secrets", APIGeneration: "v1", Kind: "secret_key", ID: "key", EndpointID: "selectel-secrets", EndpointURL: "https://cloud.api.selcloud.ru/secrets-manager"}
	r, err := c.Delete(context.Background(), ref)
	if types.ErrorCode(err) != types.PermissionDenied || r.Receipt.Outcome != types.Rejected {
		t.Fatal("denial claimed deletion")
	}
	raw, _ := json.Marshal(err)
	if strings.Contains(string(raw), "private-value") {
		t.Fatal("provider payload leaked")
	}
}
