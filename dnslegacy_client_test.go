package selectelsdk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/dnslegacy"
	"github.com/festy23/selectel-sdk/types"
)

func TestLegacyDNSGetTagUsesExactAccountV1TokenAndProtectedDTO(t *testing.T) {
	lookups, sends := 0, 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		lookups++
		if r.Mode != types.StaticToken || r.Service != "dns" || r.Scope.ProjectID != "" {
			t.Fatalf("wrong legacy auth request: %+v", r)
		}
		return credentials.Material{Identity: "dns-static", Revision: "r1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-dns-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		sends++
		if r.Method != "GET" || r.URL.String() != "https://api.selectel.ru/domains/v1/tags/0" || r.Header.Get("X-Token") != "synthetic-dns-token" || r.Header.Get("X-Auth-Token") != "" {
			t.Fatalf("wrong legacy wire: %s %s", r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":0,"name":"synthetic-private-tag","domains":[9007199254740993],"ref":{"service":"foreign"}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().Tags.Get(context.Background(), "0", types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data.ID == nil || out.Data.ID.String() != "0" || out.Data.Name == nil || string(out.Data.Name.Reveal()) != "synthetic-private-tag" || out.Data.Domains[0].String() != "9007199254740993" || out.Data.Ref == nil || out.Data.Ref.Service != "dns" || out.Data.Ref.APIGeneration != "v1" || out.Data.Ref.ID != "0" || out.Data.Ref.Scope != (types.Scope{AccountID: "test-account"}) {
		t.Fatalf("wrong typed identity: %+v", out)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", out, out), "synthetic-private-tag") || lookups != 1 || sends != 1 {
		t.Fatal("protected output or I/O boundary violated")
	}
}

func TestLegacyDNSReadTagRejectsBadIDBeforeCredentials(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		calls++
		return credentials.Material{}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "-1", "01", "1/2"} {
		_, err = c.Account().LegacyDNS().Tags.Get(context.Background(), id, types.CallOptions{})
		if types.ErrorCode(err) != types.InvalidArgument {
			t.Fatalf("bad ID %q accepted: %v", id, err)
		}
	}
	if calls != 0 {
		t.Fatal("invalid selector performed I/O")
	}
}

func TestLegacyDNSTagUpdateRevealsOnlyAtWireAndRetainsAcceptedExactRef(t *testing.T) {
	token := types.Protect([]byte("synthetic-token"))
	name := types.Protect([]byte("synthetic-new-private-tag"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: token}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || string(raw) != `{"name":"synthetic-new-private-tag","domains":[]}` {
			t.Fatalf("incorrect protected request boundary: method=%s", r.Method)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":0,"domains":[]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "tag", ID: "0", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	spec := dnslegacy.TagUpdateSpec{Name: &name, Domains: []types.Integer{}}
	out, err := c.Account().LegacyDNS().Tags.Update(context.Background(), ref, spec, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref || out.Receipt.Attempts != 1 {
		t.Fatalf("accepted mutation evidence lost: %+v %v", out, err)
	}
	raw, _ := json.Marshal(spec)
	if strings.Contains(string(raw), "synthetic-new-private-tag") {
		t.Fatal("protected value escaped outside wire")
	}
}

func TestLegacyDNSTagDeleteAcceptsOnlyStatusEvidenceAndProtectsUnknownBody(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "DELETE" {
			t.Fatal("wrong verb")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"undocumented":"synthetic-private-ack"}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "tag", ID: "0", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	out, err := c.Account().LegacyDNS().Tags.Delete(context.Background(), ref, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Data.Representation == nil || string(out.Data.Representation.Reveal()) != `{"undocumented":"synthetic-private-ack"}` {
		t.Fatalf("lost bounded acknowledgement: %+v %v", out, err)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", out, out), "synthetic-private-ack") {
		t.Fatal("unknown body leaked")
	}
	cap, err := c.Catalog().Describe("dns.v1.tags.delete")
	if err != nil || cap.Implementation.Status != "partial" {
		t.Fatal("missing response contract promoted")
	}
}

func TestLegacyDNSInvalidMutationReturnsRevisionBoundNotSentWithoutIO(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		calls++
		return credentials.Material{}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	name := types.Protect([]byte("tag"))
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "other-account"}, Service: "dns", APIGeneration: "v1", Kind: "tag", ID: "0", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	out, err := c.Account().LegacyDNS().Tags.Update(context.Background(), ref, dnslegacy.TagUpdateSpec{Name: &name}, types.CallOptions{})
	if types.ErrorCode(err) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || out.Receipt.Attempts != 0 || calls != 0 {
		t.Fatalf("invalid mutation lost receipt: %+v %v calls=%d", out, err, calls)
	}
}

func TestLegacyDNSDomainGetKeepsNumericOwnerMetadataAndDistinctExactRef(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/domains/v1/42" || r.URL.RawQuery != "show_ips=false" {
			t.Fatal("wrong selector/explicit false mapping")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":42,"name":"private.example","user_id":777,"create_date":1700000000,"tags":[],"ips":[]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	f := false
	out, err := c.Account().LegacyDNS().Domains.Get(context.Background(), dnslegacy.DomainSelector{ID: "42"}, &f, types.CallOptions{})
	if err != nil || out.Data.ID == nil || out.Data.ID.String() != "42" || out.Data.UserID == nil || out.Data.UserID.String() != "777" || out.Data.Ref == nil || out.Data.Ref.ID != "42" || out.Data.Ref.Scope.ProjectID != "" || out.Data.Ref.Parent != nil {
		t.Fatalf("domain identity/owner inferred incorrectly: %+v %v", out, err)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", out, out), "private.example") {
		t.Fatal("domain name leaked")
	}
}

func TestLegacyDNSDomainReadRejectsMismatchedRecordDiscriminator(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":42,"name":"private.example","user_id":777,"create_date":1,"ips":[{"id":7,"type":"AAAA","ttl":1,"name":"private.example","change_date":1,"content":"2001:db8::1"}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Account().LegacyDNS().Domains.Get(context.Background(), dnslegacy.DomainSelector{ID: "42"}, nil, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError {
		t.Fatalf("RecordARead source shape ignored: %v", err)
	}
}

func TestLegacyDNSDomainReadRejectsInvalidIPv4Content(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":42,"name":"private.example","user_id":777,"create_date":1,"ips":[{"id":7,"type":"A","ttl":1,"name":"private.example","change_date":1,"content":"2001:db8::1"}]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Account().LegacyDNS().Domains.Get(context.Background(), dnslegacy.DomainSelector{ID: "42"}, nil, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError {
		t.Fatalf("IPv4 source format ignored: %v", err)
	}
}
