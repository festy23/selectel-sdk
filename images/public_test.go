package images_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/images"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func clients(t *testing.T, wire transport) (*sdk.Client, *images.Client) {
	return clientsAt(t, wire, "https://glance.test")
}
func clientsAt(t *testing.T, wire transport, endpoint string) (*sdk.Client, *images.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "r", Pool: "pool"}
	c, e := sdk.New(sdk.Config{AccountID: "a", PollInterval: time.Millisecond, Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "s", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), EndpointOverrides: map[types.EndpointKey]types.EndpointBinding{{Service: "images", APIGeneration: "v2", Scope: scope}: {ID: "glance", URL: endpoint, Scope: scope, APIGeneration: "v2", Audience: "selectel"}}, Transport: transport(func(r *http.Request) (*http.Response, error) {
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
	return c, r.Images()
}
func TestCreateProtectsProviderExtensionsAndKeepsExactNumbers(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v2/images" {
			t.Fatal(r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), `"min_ram":9007199254740993`) || !strings.Contains(string(raw), `"custom":"private-marker"`) {
			t.Fatal("wire")
		}
		return response(201, `{"id":"i","owner":"p","status":"queued","size":null,"virtual_size":9007199254740993,"direct_url":"https://store.test/private-marker","private":"private-marker"}`), nil
	})
	n, _ := types.ParseInteger("9007199254740993")
	got, e := c.Create(context.Background(), images.CreateSpec{MinRAM: &n, Properties: map[string]types.ProtectedValue{"custom": types.Protect([]byte("private-marker"))}}, types.CallOptions{})
	if e != nil || got.Receipt.Outcome != types.Accepted || got.Data.VirtualSize.String() != n.String() || got.Data.Size != nil {
		t.Fatalf("%+v %v", got, e)
	}
	for _, v := range []any{got.Data, got.Data.Properties, got.Data.DirectURL} {
		b, _ := json.Marshal(v)
		if strings.Contains(string(b), "private-marker") || strings.Contains(fmt.Sprintf("%+v %#v", v, v), "private-marker") {
			t.Fatal("protected output leaked")
		}
	}
}
