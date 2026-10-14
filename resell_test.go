package selectelsdk_test

import (
	"context"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"testing"
)

type resellTransport func(*http.Request) (*http.Response, error)

func (f resellTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestResellAccountPreservesFalseAndRequiresAccountFields(t *testing.T) {
	for _, tc := range []struct {
		body string
		code types.Code
	}{{`{"account":{"enabled":false,"locked":true,"name":"synthetic","onboarding":false,"locks":[{"created_at":"2026-10-14","reason":"synthetic"}]}}`, ""}, {`{"account":{"enabled":false,"name":"synthetic","onboarding":false}}`, types.ProtocolError}} {
		c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != "https://api.selectel.ru/vpc/resell/v2/accounts" {
				t.Fatal("wrong endpoint")
			}
			return inventoryReply(200, tc.body), nil
		}, 0)
		out, e := c.Account().Resell().Account(context.Background(), types.CallOptions{})
		if types.ErrorCode(e) != tc.code {
			t.Fatalf("error %v", e)
		}
		if e == nil && (out.Data.Enabled == nil || *out.Data.Enabled || out.Data.Onboarding == nil || *out.Data.Onboarding || len(out.Data.Locks) != 1) {
			t.Fatal("account fields lost")
		}
	}
}

func TestResellDNSCheckPreservesHostnameAsOnePathSegment(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/vpc/resell/v2/dnscheck/host%2Fone" {
			t.Fatal("hostname not escaped")
		}
		return inventoryReply(200, `{"result":{"content":"synthetic","delegated":false,"ns_servers":["ns.example"]}}`), nil
	}, 0)
	out, e := c.Account().Resell().DNSCheck(context.Background(), "host/one", types.CallOptions{})
	if e != nil || out.Data.Delegated == nil || *out.Data.Delegated || len(out.Data.NsServers) != 1 {
		t.Fatal("DNS info lost", e)
	}
}

func TestResellDNSCheckRejectsEmptyHostnameWithoutIOAndMapsNotFound(t *testing.T) {
	requests := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		requests++
		return inventoryReply(400, `{"message":"synthetic-sensitive-marker"}`), nil
	}, 0)
	out, e := c.Account().Resell().DNSCheck(context.Background(), "", types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || requests != 0 || out.Receipt != nil {
		t.Fatal("invalid hostname reached provider", e)
	}
	_, e = c.Account().Resell().DNSCheck(context.Background(), "example.invalid", types.CallOptions{})
	if types.ErrorCode(e) != types.NotFound || requests != 1 {
		t.Fatal("documented not-found mapping lost", e)
	}
}
