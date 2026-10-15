package ptr_test

import (
	"context"
	"encoding/json"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/ptr"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, b string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(b))}
}
func clients(t *testing.T, f transport) (*sdk.Client, *ptr.Client) {
	t.Helper()
	c, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Mode != types.StaticToken || r.Service != "ip_addresses" || r.Scope != (types.Scope{AccountID: "a"}) {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "s", Revision: "1", Binding: r.Scope, Audience: "selectel", Scheme: credentials.StaticToken, Token: types.Protect([]byte("static-synthetic"))}, nil
	}), Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Token") != "static-synthetic" || r.Header.Get("X-Auth-Token") != "" {
			t.Fatal("auth")
		}
		return f(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	return c, c.Account().PTR()
}
func TestPTRCreateUsesSeparateAccountStaticAPIAndProtectedContent(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.selectel.ru/ipam/v1/" || r.Method != "POST" {
			t.Fatal(r.URL, r.Method)
		}
		var body map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&body)
		if string(body["ip"]) != `"192.0.2.1"` || string(body["content"]) != `"host.test."` || string(body["ttl"]) != "60" {
			t.Fatal("wire")
		}
		return response(200, `{"id":9007199254740993,"content":"host.test.","ip":"192.0.2.1","user_id":9007199254740994,"ttl":60}`), nil
	})
	content := types.Protect([]byte("host.test."))
	ttl, _ := types.ParseInteger("60")
	v, e := c.Create(context.Background(), ptr.CreateRequest{IP: "192.0.2.1", Content: &content, TTL: &ttl}, types.CallOptions{})
	if e != nil || v.Receipt.Outcome != types.Accepted || v.Data.Ref.ID != "9007199254740993" || v.Data.UserID.String() != "9007199254740994" || string(v.Data.Content.Reveal()) != "host.test." {
		t.Fatal(e)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "host.test.") {
		t.Fatal("disclosure")
	}
}
func TestPTRExactLifecycleAndNoMutationReplay(t *testing.T) {
	calls := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/ipam/v1/9" {
			t.Fatal(r.URL)
		}
		switch calls {
		case 1:
			if r.Method != "GET" {
				t.Fatal(r.Method)
			}
			return response(200, `{"id":9,"content":"host.test.","ip":"192.0.2.1","user_id":1,"ttl":60}`), nil
		case 2:
			b, _ := io.ReadAll(r.Body)
			if r.Method != "PUT" || string(b) != `{"content":"other.test."}` {
				t.Fatal(r.Method, string(b))
			}
			return response(200, `{"id":9,"content":"other.test.","ip":"192.0.2.1","user_id":1,"ttl":60}`), nil
		case 3:
			if r.Method != "DELETE" {
				t.Fatal(r.Method)
			}
			return nil, io.ErrUnexpectedEOF
		case 4:
			if r.Method != "GET" {
				t.Fatal("replay")
			}
			return response(404, `{"error":"private-provider-text"}`), nil
		}
		t.Fatal("unexpected")
		return nil, nil
	})
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a"}, Service: "ip_addresses", APIGeneration: "v1", Kind: "ptr", ID: "9", EndpointID: "selectel-ipam", EndpointURL: "https://api.selectel.ru/ipam/v1"}
	if _, e := c.Get(context.Background(), ref, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	content := types.Protect([]byte("other.test."))
	if _, e := c.Update(context.Background(), ref, ptr.UpdateRequest{Content: &content}, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	v, e := c.Delete(context.Background(), ref, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || v.Receipt.Outcome != types.Unknown || calls != 3 {
		t.Fatal(e)
	}
	h, e := root.Resume(*v.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	r, e := h.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || r.ProofStatus != "indeterminate" || r.Observation.Classification != "absent" || calls != 4 {
		t.Fatal(e)
	}
}
func TestPTROffsetCollectionKeepsPrefixAndCountVariantExplicit(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Query().Get("limit") != "1" || r.URL.Query().Get("filter") != "private-filter" {
			t.Fatal(r.URL)
		}
		if r.URL.Query().Get("offset") == "2" {
			return response(200, `[{"id":9,"content":"host.test.","ip":"192.0.2.1","user_id":1,"ttl":60}]`), nil
		}
		if r.URL.Query().Get("offset") == "3" {
			return response(200, `[]`), nil
		}
		t.Fatal(r.URL)
		return nil, nil
	})
	filter := types.Protect([]byte("private-filter"))
	offset, _ := types.ParseInteger("2")
	v, e := c.Collect(context.Background(), ptr.ListOptions{Filter: &filter, Offset: &offset}, pagination.Options{PageSize: 1})
	if e != nil || !v.Complete || len(v.Items) != 1 || calls != 2 {
		t.Fatal(e)
	}
	if _, e = c.Count(context.Background(), ptr.ListOptions{}, types.CallOptions{}); types.ErrorCode(e) != types.CapabilityUnavailable || calls != 2 {
		t.Fatal(e)
	}
}
func TestPTRMalformedSecondPagePreservesPrefixAndUnknownIdentity(t *testing.T) {
	calls := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "POST" {
			return response(200, `{"id":9,"content":false}`), nil
		}
		if r.URL.Query().Get("offset") == "0" {
			return response(200, `[{"id":9,"content":"host.test.","ip":"192.0.2.1","user_id":1,"ttl":60}]`), nil
		}
		return response(200, `[{"id":"bad"}]`), nil
	})
	v, e := c.Collect(context.Background(), ptr.ListOptions{}, pagination.Options{PageSize: 1})
	if types.ErrorCode(e) != types.ProtocolError || v.Complete || len(v.Items) != 1 || v.NextCursor == nil {
		t.Fatal(e, len(v.Items))
	}
	create, e := c.Create(context.Background(), ptr.CreateRequest{IP: "192.0.2.1"}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || create.Receipt.Outcome != types.Accepted || len(create.Receipt.ResourceRefs) != 1 {
		t.Fatal(e)
	}
	h, e := root.Resume(*create.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	before := calls
	r, e := h.Recover(context.Background())
	if e != nil || r.ProofStatus != "conclusive" || before != calls {
		t.Fatal(e)
	}
}
func TestPTRLocalValidationAndWrongEndpointNoTargetIO(t *testing.T) {
	root, c := clients(t, func(*http.Request) (*http.Response, error) { t.Fatal("unexpected target I/O"); return nil, nil })
	ttl, _ := types.ParseInteger("59")
	v, e := c.Create(context.Background(), ptr.CreateRequest{IP: "192.0.2.1", TTL: &ttl}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || v.Receipt.Outcome != types.NotSent {
		t.Fatal(e)
	}
	h, e := root.Resume(*v.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	if r, e := h.Recover(context.Background()); e != nil || r.ProofStatus != "conclusive" {
		t.Fatal(e)
	}
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "a"}, Service: "ip_addresses", APIGeneration: "v1", Kind: "ptr", ID: "9", EndpointID: "selectel-ipam", EndpointURL: "https://old.test/v1"}
	if v, e := c.Delete(context.Background(), ref, types.CallOptions{}); types.ErrorCode(e) != types.EndpointUnavailable || v.Receipt.Outcome != types.NotSent {
		t.Fatal(e)
	}
}
