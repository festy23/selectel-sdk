package volumes_test

import (
	"context"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
	"github.com/festy23/selectel-sdk/volumes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func client(t *testing.T, wire transport) *volumes.Client             { _, c := clients(t, wire); return c }
func clients(t *testing.T, wire transport) (*sdk.Client, *volumes.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}
	c, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "s", Revision: "1", Scheme: credentials.IAMToken, Binding: scope, Audience: "selectel", Token: types.Protect([]byte("synthetic")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "volumes", APIGeneration: "v3", Scope: scope}: {ID: "cinder", URL: "https://cinder.test/v3/p", Scope: scope, APIGeneration: "v3", Audience: "selectel"}}, Transport: transport(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "auth/tokens") {
			return response(200, `{"token":{"expires_at":"2099-01-01T00:00:00Z","project":{"id":"p","domain":{"name":"a"}}}}`), nil
		}
		if r.Header.Get("X-Auth-Token") != "synthetic" {
			t.Fatal("auth")
		}
		return wire(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Project("p")
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.Location("r", "pool")
	if e != nil {
		t.Fatal(e)
	}
	return c, r.Volumes()
}
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestCreateBackupUsesRequiredMicroversionAndExactNumbers(t *testing.T) {
	c := client(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v3/p/volumes" || r.Header.Get("OpenStack-API-Version") != "volume 3.47" {
			t.Fatal(r.Method, r.URL, r.Header)
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"size":9007199254740993`) {
			t.Fatal(string(b))
		}
		return response(202, `{"volume":{"id":"v","status":"creating","size":9007199254740993,"attachments":[{"id":"v","attachment_id":"attachment","server_id":"server"}]}}`), nil
	})
	size, _ := types.ParseInteger("9007199254740993")
	got, e := c.Create(context.Background(), volumes.CreateSpec{Size: &size, BackupID: "backup"}, types.CallOptions{})
	if e != nil || got.Receipt.Outcome != types.Accepted || got.Data.Size.String() != size.String() || got.Data.Attachments[0].AttachmentID != "attachment" {
		t.Fatalf("%+v %v", got, e)
	}
}
