package selectelsdk_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/dnslegacy"
	"github.com/festy23/selectel-sdk/pagination"
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

func TestLegacyDNSStateReadPreservesFalseWithoutInventingDomainID(t *testing.T) {
	name := types.Protect([]byte("private.example"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/domains/v1/private.example/state" {
			t.Fatal("wrong named state route")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"disabled":false}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().Domains.State(context.Background(), dnslegacy.DomainSelector{Name: &name}, types.CallOptions{})
	if err != nil || out.Data.Disabled == nil || *out.Data.Disabled {
		t.Fatalf("state boolean lost: %+v %v", out, err)
	}
}

func TestLegacyDNSSetStatePreservesFalseAndKnownRefWith204(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PATCH" || string(raw) != `{"disabled":false}` {
			t.Fatal("false state omitted")
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	f := false
	ref := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	out, err := c.Account().LegacyDNS().Domains.SetState(context.Background(), ref, dnslegacy.ZoneStateSpec{Disabled: &f}, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0] != ref {
		t.Fatalf("204 acceptance lost exact ref: %+v %v", out, err)
	}
}

func TestLegacyDNSNamedUpdateRetainsTrustworthyIDBeforeOtherFields(t *testing.T) {
	name := types.Protect([]byte("private.example"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PATCH" || r.URL.EscapedPath() != "/domains/v1/private.example" || string(raw) != `{"tags":[]}` {
			t.Fatal("named update mapping incorrect")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":42,"name":"private.example","user_id":"malformed","create_date":1}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().Domains.UpdateByName(context.Background(), name, dnslegacy.DomainUpdateSpec{Tags: []types.Integer{}}, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "42" || out.Receipt.ResourceRefs[0].Parent != nil {
		t.Fatalf("trusted ID lost or adoption invented: %+v %v", out, err)
	}
}

func TestLegacyDNSNamedDeleteUnknownNeverReplaysOrInventsNumericRef(t *testing.T) {
	sends := 0
	name := types.Protect([]byte("private.example"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		sends++
		if r.Method != "DELETE" || r.URL.EscapedPath() != "/domains/v1/private.example" {
			t.Fatal("wrong named delete mapping")
		}
		return nil, io.ErrUnexpectedEOF
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().Domains.DeleteByName(context.Background(), name, types.CallOptions{})
	if types.ErrorCode(err) != types.UnknownOutcome || sends != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || len(out.Receipt.ResourceRefs) != 0 || out.Receipt.Recovery != "unavailable" {
		t.Fatalf("unknown named mutation replay/adoption: %+v %v sends=%d", out, err, sends)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v %v", out, out, err), "private.example") {
		t.Fatal("named mutation leaked")
	}
}

func TestLegacyDNSDomainListEmptyNeverInventsTerminalRule(t *testing.T) {
	sends := 0
	offset, _ := types.ParseInteger("9007199254740993")
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		sends++
		if r.URL.Query().Get("offset") != "9007199254740993" {
			t.Fatal("offset precision lost")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[]`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.Account().LegacyDNS().Domains.ListPage(context.Background(), dnslegacy.DomainListOptions{Offset: &offset}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || page.Done || page.NextCursor != nil || len(page.Items) != 0 || sends != 1 {
		t.Fatalf("unknown terminal became complete: %+v %v sends=%d", page, err, sends)
	}
}

func TestLegacyDNSRecordGetDecodesTypedAAAAWithCallerParent(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/domains/v1/42/records/7" {
			t.Fatal("record parent route lost")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":7,"type":"AAAA","ttl":60,"name":"private.example","change_date":1,"content":"2001:db8::1","ref":{"service":"foreign"}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	out, err := c.Account().LegacyDNS().Records.Get(context.Background(), parent, "7", types.CallOptions{})
	if err != nil || out.Data.AAAA == nil || out.Data.AAAA.ID == nil || out.Data.AAAA.ID.String() != "7" || out.Data.Ref == nil || out.Data.Ref.Parent == nil || *out.Data.Ref.Parent != parent || out.Data.Ref.Service != "dns" {
		t.Fatalf("typed record/parent lost: %+v %v", out, err)
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", out, out), "private.example") || strings.Contains(fmt.Sprintf("%+v %#v", out, out), "2001:db8::1") {
		t.Fatal("record contents leaked")
	}
}

func TestLegacyDNSNamedRecordReadDoesNotManufactureParentIdentity(t *testing.T) {
	name := types.Protect([]byte("private.example"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/domains/v1/private.example/records/7" {
			t.Fatal("named parent mapping lost")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":7,"type":"A","ttl":60,"name":"private.example","change_date":1,"content":"192.0.2.1","ref":{"parent":{"id":"42"}}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().Records.GetByName(context.Background(), name, "7", types.CallOptions{})
	if err != nil || out.Data.A == nil || out.Data.A.ID == nil || out.Data.A.ID.String() != "7" || out.Data.Ref != nil {
		t.Fatalf("numeric parent inferred from named path/response: %+v %v", out, err)
	}
}

func TestLegacyDNSRecordCreateUsesTypedWireUnionAndRetainsAcceptedID(t *testing.T) {
	kind := "AAAA"
	name := types.Protect([]byte("private.example"))
	content := types.Protect([]byte("2001:db8::1"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(raw) != `{"content":"2001:db8::1","name":"private.example","type":"AAAA"}` {
			t.Fatal("typed protected union did not reach wire")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":7,"type":"AAAA","ttl":"malformed","name":"private.example","change_date":1,"content":"2001:db8::1"}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	spec := dnslegacy.RecordCreateSpec{AAAA: &dnslegacy.RecordAAAACreate{Type: &kind, Name: &name, Content: &content}}
	out, err := c.Account().LegacyDNS().Records.Create(context.Background(), parent, spec, types.CallOptions{})
	if types.ErrorCode(err) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "7" || out.Receipt.ResourceRefs[0].Parent == nil || *out.Receipt.ResourceRefs[0].Parent != parent {
		t.Fatalf("accepted child ID lost/inferred: %+v %v", out, err)
	}
	raw, _ := json.Marshal(spec)
	if strings.Contains(string(raw), "private.example") || strings.Contains(string(raw), "2001:db8::1") {
		t.Fatal("wire reveal escaped encoder")
	}
}

func TestLegacyDNSRecordUpdateRejectsForeignResponseAndKeepsCallerChild(t *testing.T) {
	kind := "TXT"
	content := types.Protect([]byte("synthetic-private-update"))
	name := types.Protect([]byte("private.example"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || r.URL.EscapedPath() != "/domains/v1/42/records/7" || string(raw) != `{"content":"synthetic-private-update","name":"private.example","type":"TXT"}` {
			t.Fatal("typed update mapping lost")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":8,"type":"TXT","ttl":60,"name":"private.example","change_date":1,"content":"foreign"}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	ref := types.ResourceRef{Scope: parent.Scope, Service: "dns", APIGeneration: "v1", Kind: "record", ID: "7", Parent: &parent, EndpointID: parent.EndpointID, EndpointURL: parent.EndpointURL}
	out, err := c.Account().LegacyDNS().Records.Update(context.Background(), ref, dnslegacy.RecordUpdateSpec{TXT: &dnslegacy.RecordTXTUpdate{Type: &kind, Name: &name, Content: &content}}, types.CallOptions{})
	if types.ErrorCode(err) != types.ScopeMismatch || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "7" {
		t.Fatalf("foreign identity adopted or receipt lost: %+v %v", out, err)
	}
}

func TestLegacyDNSRecordDelete204KeepsExactHierarchicalTarget(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "DELETE" || r.URL.EscapedPath() != "/domains/v1/42/records/7" {
			t.Fatal("wrong record delete")
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	ref := types.ResourceRef{Scope: parent.Scope, Service: "dns", APIGeneration: "v1", Kind: "record", ID: "7", Parent: &parent, EndpointID: parent.EndpointID, EndpointURL: parent.EndpointURL}
	out, err := c.Account().LegacyDNS().Records.Delete(context.Background(), ref, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].Parent == nil || *out.Receipt.ResourceRefs[0].Parent != parent {
		t.Fatalf("delete lost exact parent: %+v %v", out, err)
	}
}

func TestLegacyDNSRecordListBoundsValidatedPrefixWithoutCursorGuess(t *testing.T) {
	sends := 0
	a := types.Protect([]byte("A"))
	aaaa := types.Protect([]byte("AAAA"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		sends++
		if strings.Join(r.URL.Query()["record_types"], ",") != "A,AAAA" {
			t.Fatal("query array representation lost")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"id":7,"type":"A","ttl":60,"name":"private.example","change_date":1,"content":"192.0.2.1"},{"id":8,"type":"AAAA","ttl":60,"name":"private.example","change_date":1,"content":"2001:db8::1"}]`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	page, err := c.Account().LegacyDNS().Records.ListPage(context.Background(), parent, dnslegacy.RecordListOptions{RecordTypes: []types.ProtectedValue{a, aaaa}}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(err) != types.Incomplete || page.Done || page.NextCursor != nil || len(page.Items) != 1 || page.Items[0].Ref == nil || page.Items[0].Ref.Parent == nil || *page.Items[0].Ref.Parent != parent || sends != 1 {
		t.Fatalf("bounded prefix lost or cursor invented: %+v %v sends=%d", page, err, sends)
	}
}

func TestLegacyDNSReplaceAllPreviewNeverAdoptsProposedRecordIDs(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || string(raw) != `[]` || r.URL.Query().Get("apply_changes") != "false" {
			t.Fatal("explicit preview/empty replacement mapping lost")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"applied":false,"changes":{"create":[{"id":7,"type":"A","ttl":60,"name":"private.example","change_date":1,"content":"192.0.2.1"}],"update":[],"delete":[]}}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	f := false
	out, err := c.Account().LegacyDNS().Records.ReplaceAll(context.Background(), parent, []dnslegacy.RecordCreateSpec{}, &f, types.CallOptions{})
	if err != nil || out.Data.Applied == nil || *out.Data.Applied || out.Data.Changes == nil || out.Data.Changes.Create[0].Ref != nil || out.Receipt == nil || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "42" {
		t.Fatalf("preview proposed refs adopted: %+v %v", out, err)
	}
}

func TestLegacyDNSBatchScansLaterTrustworthyIDsAfterMalformedRows(t *testing.T) {
	kind := "A"
	name := types.Protect([]byte("private.example"))
	content := types.Protect([]byte("192.0.2.1"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != "PATCH" || r.URL.EscapedPath() != "/domains/v1/42/records/batch_update" {
			t.Fatal("wrong batch route")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"id":"bad"},{"id":8,"type":"A","ttl":60,"name":"private.example","change_date":1,"content":"192.0.2.1"}]`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	spec := dnslegacy.RecordBatchSpec{Create: []dnslegacy.RecordCreateSpec{{A: &dnslegacy.RecordACreate{Type: &kind, Name: &name, Content: &content}}}}
	out, err := c.Account().LegacyDNS().Records.Batch(context.Background(), parent, spec, types.CallOptions{})
	found := false
	if out.Receipt != nil {
		for _, ref := range out.Receipt.ResourceRefs {
			if ref.Kind == "record" && ref.ID == "8" && ref.Parent != nil && *ref.Parent == parent {
				found = true
			}
		}
	}
	if types.ErrorCode(err) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || !found {
		t.Fatalf("later independent ID lost: %+v %v", out, err)
	}
}

func TestLegacyDNSBoundDomainCreatePreservesParentAndLaterChildBeforeFieldDecode(t *testing.T) {
	name := types.Protect([]byte("private.example"))
	zone := types.Protect([]byte("synthetic private BIND zone"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || string(raw) != `{"bind_zone":"synthetic private BIND zone","name":"private.example"}` {
			t.Fatal("protected create mapping lost")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"domain":{"id":42,"name":"private.example","user_id":"malformed","create_date":1},"records":[{"id":"bad"},{"id":8,"type":"A","ttl":60,"name":"private.example","change_date":1,"content":"192.0.2.1"}],"errors":[]}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().Domains.Create(context.Background(), dnslegacy.DomainCreateSpec{Name: &name, BindZone: &zone}, types.CallOptions{})
	domain, child := false, false
	if out.Receipt != nil {
		for _, ref := range out.Receipt.ResourceRefs {
			domain = domain || (ref.Kind == "domain" && ref.ID == "42")
			child = child || (ref.Kind == "record" && ref.ID == "8" && ref.Parent != nil && ref.Parent.ID == "42")
		}
	}
	if types.ErrorCode(err) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || !domain || !child {
		t.Fatalf("independent bound-create IDs lost: %+v %v", out, err)
	}
}

func TestLegacyDNSExportStreamsOnlyToCallerWriterWithBoundedProgress(t *testing.T) {
	body := &dnsExportBody{Reader: strings.NewReader("synthetic-private-BIND-zone")}
	var destination bytes.Buffer
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/domains/v1/42/export" {
			t.Fatal("wrong export route")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}}, Body: body}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().Domains.Export(context.Background(), dnslegacy.DomainSelector{ID: "42"}, &destination, dnslegacy.ExportOptions{MaxBytes: 4}, types.CallOptions{})
	if types.ErrorCode(err) != types.ResponseLimit || out.Data.Bytes != 4 || out.Data.Complete || destination.String() != "synt" || body.closes.Load() != 1 {
		t.Fatalf("export limit/progress/ownership violated: %+v %v closes=%d", out, err, body.closes.Load())
	}
	if strings.Contains(fmt.Sprintf("%+v %#v %v", out, out, err), "synthetic-private-BIND-zone") {
		t.Fatal("export contents escaped writer boundary")
	}
}

type dnsExportBody struct {
	io.Reader
	closes atomic.Int32
}

func (b *dnsExportBody) Close() error { b.closes.Add(1); return nil }

func TestLegacyDNSMigrationIsExplicitAccountActionWithoutInventedV2Refs(t *testing.T) {
	project := types.Protect([]byte("target-project"))
	sends := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Mode != types.StaticToken || r.Scope.ProjectID != "" {
			t.Fatal("migration reinterpreted as project auth")
		}
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		sends++
		raw, _ := io.ReadAll(r.Body)
		if r.Method != "PUT" || r.URL.String() != "https://api.selectel.ru/domains/v1/version/v2" || string(raw) != `{"domains":[],"project_id":"target-project"}` {
			t.Fatal("explicit migration mapping lost")
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().MigrateToV2(context.Background(), dnslegacy.MigrationSpec{ProjectID: &project, Domains: []types.Integer{}}, types.CallOptions{})
	if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Receipt.Scope.ProjectID != "" || len(out.Receipt.ResourceRefs) != 0 || out.Receipt.Recovery != "unavailable" || sends != 1 {
		t.Fatalf("migration adopted destination scope/resources: %+v %v sends=%d", out, err, sends)
	}
}

func TestLegacyDNSAllPublishedRecordResponseVariantsRemainTypedAndProtected(t *testing.T) {
	fixtures := map[string]string{
		"A": `"content":"192.0.2.1"`, "AAAA": `"content":"2001:db8::1"`, "CNAME": `"content":"synthetic-private-content"`, "MX": `"content":"synthetic-private-content","priority":0`, "NS": `"content":"synthetic-private-content"`, "SOA": `"content":"synthetic-private-content","ns":"synthetic-private-content","email":"synthetic-private-content","serial":1,"refresh":1,"retry":1,"expire":1,"minimum":1`, "SRV": `"target":"synthetic-private-content","priority":0,"weight":0,"port":443`, "TXT": `"content":"synthetic-private-content"`, "SSHFP": `"algorithm":0,"fingerprint_type":0,"fingerprint":"synthetic-private-content"`, "ALIAS": `"content":"synthetic-private-content"`, "CAA": `"flag":0,"tag":"issue","value":"synthetic-private-content"`,
	}
	for kind, fields := range fixtures {
		t.Run(kind, func(t *testing.T) {
			c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
				return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
			}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				raw := `{"id":7,"type":"` + kind + `","ttl":60,"name":"synthetic-private-name","change_date":1,` + fields + `}`
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
			out, err := c.Account().LegacyDNS().Records.Get(context.Background(), parent, "7", types.CallOptions{})
			if err != nil || out.Data.RecordID() == nil || out.Data.RecordID().String() != "7" {
				t.Fatalf("source variant failed: %+v %v", out, err)
			}
			assertDNSProtected(t, out, "synthetic-private-name")
			assertDNSProtected(t, out, "synthetic-private-content")
		})
	}
}
func assertDNSProtected(t *testing.T, value any, marker string) {
	t.Helper()
	raw, _ := json.Marshal(value)
	formatted := fmt.Sprintf("%v %+v %#v %x", value, value, value, value) + string(raw)
	for _, encoded := range []string{marker, fmt.Sprint([]byte(marker)), fmt.Sprintf("%#v", []byte(marker)), hex.EncodeToString([]byte(marker))} {
		if strings.Contains(formatted, encoded) {
			t.Fatal("protected value encoding leaked")
		}
	}
}

func TestLegacyDNSExportCancellationAndTransferDeadlineCloseBodyOnce(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint(deadline), func(t *testing.T) {
			body := &dnsBlockingExport{prefix: strings.NewReader("BIND"), entered: make(chan struct{}), closed: make(chan struct{})}
			var destination bytes.Buffer
			sends := atomic.Int32{}
			c, err := sdk.New(sdk.Config{AccountID: "test-account", CallTimeout: time.Second, TransferTimeout: 100 * time.Millisecond, Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
				return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
			}), Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				sends.Add(1)
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: body}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if !deadline {
				go func() {
					select {
					case <-body.entered:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			type answer struct {
				out types.Result[dnslegacy.ExportProgress]
				err error
			}
			done := make(chan answer, 1)
			go func() {
				out, e := c.Account().LegacyDNS().Domains.Export(ctx, dnslegacy.DomainSelector{ID: "42"}, &destination, dnslegacy.ExportOptions{MaxBytes: 64}, types.CallOptions{})
				done <- answer{out, e}
			}()
			select {
			case got := <-done:
				want := types.Canceled
				if deadline {
					want = types.DeadlineExceeded
				}
				if types.ErrorCode(got.err) != want || got.out.Data.Bytes != 4 || got.out.Data.Complete || body.closes.Load() != 1 || sends.Load() != 1 || destination.String() != "BIND" {
					t.Fatalf("stream cancellation/progress failed: %+v %v closes=%d sends=%d", got.out, got.err, body.closes.Load(), sends.Load())
				}
			case <-time.After(2 * time.Second):
				_ = body.Close()
				t.Fatal("cancellation did not unblock owned response body")
			}
		})
	}
}

type dnsBlockingExport struct {
	prefix               *strings.Reader
	entered, closed      chan struct{}
	enterOnce, closeOnce sync.Once
	closes               atomic.Int32
}

func (b *dnsBlockingExport) Read(p []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(p)
	}
	b.enterOnce.Do(func() { close(b.entered) })
	<-b.closed
	return 0, fmt.Errorf("synthetic-private-body-error")
}
func (b *dnsBlockingExport) Close() error {
	b.closes.Add(1)
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

func TestLegacyDNSExactRefReadsRejectForeignEndpointBeforeCredentials(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		calls++
		return credentials.Material{}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	tag := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "tag", ID: "0", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://foreign.invalid"}
	domain := tag
	domain.Kind = "domain"
	domain.ID = "42"
	parent := domain
	parent.EndpointURL = "https://api.selectel.ru/domains/v1"
	record := tag
	record.Kind = "record"
	record.ID = "7"
	record.Parent = &parent
	_, e1 := c.Account().LegacyDNS().Tags.GetRef(context.Background(), tag, types.CallOptions{})
	_, e2 := c.Account().LegacyDNS().Domains.GetRef(context.Background(), domain, nil, types.CallOptions{})
	_, e3 := c.Account().LegacyDNS().Records.GetRef(context.Background(), record, types.CallOptions{})
	for _, e := range []error{e1, e2, e3} {
		if types.ErrorCode(e) != types.ScopeMismatch {
			t.Fatalf("foreign endpoint not refused: %v", e)
		}
	}
	if calls != 0 {
		t.Fatal("foreign exact-ref observation performed I/O")
	}
}

func TestLegacyDNSAllPublishedCreateVariantsRevealOnlyInTypedRequest(t *testing.T) {
	fixtures := map[string]string{
		"A": `"content":"192.0.2.1"`, "AAAA": `"content":"2001:db8::1"`, "CNAME": `"content":"synthetic-private-content"`, "MX": `"content":"synthetic-private-content","priority":0`, "NS": `"content":"synthetic-private-content"`, "SRV": `"content":"synthetic-private-content","target":"synthetic-private-content","priority":0,"weight":0,"port":443`, "TXT": `"content":"synthetic-private-content"`, "SSHFP": `"content":"synthetic-private-content","algorithm":0,"fingerprint_type":0,"fingerprint":"synthetic-private-content"`, "ALIAS": `"content":"synthetic-private-content"`, "CAA": `"content":"synthetic-private-content","flag":0,"tag":"issue","value":"synthetic-private-content"`,
	}
	for kind, fields := range fixtures {
		t.Run(kind, func(t *testing.T) {
			input := `{"type":"` + kind + `","name":"synthetic-private-name",` + fields + `}`
			var spec dnslegacy.RecordCreateSpec
			if err := json.Unmarshal([]byte(input), &spec); err != nil {
				t.Fatal("source fixture is invalid")
			}
			c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
				return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
			}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				raw, _ := io.ReadAll(r.Body)
				var got, want any
				_ = json.Unmarshal(raw, &got)
				_ = json.Unmarshal([]byte(input), &want)
				if !reflect.DeepEqual(got, want) {
					t.Fatal("typed source fields/default zero or protected wire mapping lost")
				}
				response := `{"id":7,"ttl":60,"change_date":1,` + input[1:]
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(response))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
			out, err := c.Account().LegacyDNS().Records.Create(context.Background(), parent, spec, types.CallOptions{})
			if err != nil || out.Data.RecordID() == nil || out.Data.RecordID().String() != "7" {
				t.Fatalf("typed create variant failed: %+v %v", out, err)
			}
			assertDNSProtected(t, spec, "synthetic-private-name")
			assertDNSProtected(t, out, "synthetic-private-content")
		})
	}
}

func TestLegacyDNSRecordInputBoundsFailBeforeCredentialsWithReceipt(t *testing.T) {
	calls := 0
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		calls++
		return credentials.Material{}, nil
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })})
	if err != nil {
		t.Fatal(err)
	}
	kind := "CAA"
	name := types.Protect([]byte("private.example"))
	tag := types.Protect([]byte("issue"))
	value := types.Protect([]byte("private-value"))
	flag, _ := types.ParseInteger("256")
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	out, err := c.Account().LegacyDNS().Records.Create(context.Background(), parent, dnslegacy.RecordCreateSpec{CAA: &dnslegacy.RecordCAACreate{Type: &kind, Name: &name, Tag: &tag, Value: &value, Content: &value, Flag: &flag}}, types.CallOptions{})
	if types.ErrorCode(err) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || out.Receipt.Attempts != 0 || calls != 0 {
		t.Fatalf("source bound bypassed or receipt lost: %+v %v calls=%d", out, err, calls)
	}
}

func TestLegacyDNSCatalogPreservesNineteenIDsAndRequiredParentScope(t *testing.T) {
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source()})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := c.Catalog().List(catalog.Filter{Service: "dns", APIGeneration: "v1"})
	if err != nil || len(rows) != 19 {
		t.Fatal("source operations lost or helper aliases double counted")
	}
	implemented, partial := 0, 0
	for _, row := range rows {
		if row.Implementation.Status == "implemented" {
			implemented++
		}
		if row.Implementation.Status == "partial" {
			partial++
		}
	}
	if implemented != 15 || partial != 4 {
		t.Fatalf("source gaps promoted or covered operations lost: %d/%d", implemented, partial)
	}
	row, err := c.Catalog().Describe("dns.v1.records.replace_1158c7f1")
	if err != nil {
		t.Fatal(err)
	}
	parent := false
	for _, scope := range row.ScopeRequirements {
		parent = parent || scope == "parent"
	}
	if !parent {
		t.Fatal("required record parent omitted from public catalog")
	}
	if c.Catalog().Summary().InventoryComplete || c.Catalog().Summary().CoverageRatio != nil {
		t.Fatal("whole DNS inventory completion invented")
	}
}

func TestLegacyDNSRecordMutationSnapshotsCallerParentBeforeIO(t *testing.T) {
	parent := types.ResourceRef{Scope: types.Scope{AccountID: "test-account"}, Service: "dns", APIGeneration: "v1", Kind: "domain", ID: "42", EndpointID: "selectel-dns-legacy-v1", EndpointURL: "https://api.selectel.ru/domains/v1"}
	ref := types.ResourceRef{Scope: parent.Scope, Service: "dns", APIGeneration: "v1", Kind: "record", ID: "7", Parent: &parent, EndpointID: parent.EndpointID, EndpointURL: parent.EndpointURL}
	kind := "A"
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/domains/v1/42/records/7" {
			t.Fatal("original route lost")
		}
		parent.ID = "99"
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":7,"type":"A","ttl":60,"name":"private.example","change_date":1,"content":"192.0.2.1"}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().Records.Update(context.Background(), ref, dnslegacy.RecordUpdateSpec{A: &dnslegacy.RecordAUpdate{Type: &kind}}, types.CallOptions{})
	if err != nil || out.Data.Ref == nil || out.Data.Ref.Parent == nil || out.Data.Ref.Parent.ID != "42" || out.Receipt == nil || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].Parent.ID != "42" {
		t.Fatalf("caller mutation changed identity after request: %+v %v", out, err)
	}
}

func TestLegacyDNSNamedReadSnapshotsProtectedSelectorBeforeIO(t *testing.T) {
	name := types.Protect([]byte("private.example"))
	c, err := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "dns", Revision: "1", Binding: types.Scope{AccountID: "test-account"}, Audience: credentials.SelectelAudience, Scheme: credentials.StaticToken, Token: types.Protect([]byte("synthetic-token"))}, nil
	}), Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/domains/v1/private.example" {
			t.Fatal("original name route lost")
		}
		name = types.Protect([]byte("other.example"))
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":42,"name":"private.example","user_id":777,"create_date":1}`))}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Account().LegacyDNS().Domains.Get(context.Background(), dnslegacy.DomainSelector{Name: &name}, nil, types.CallOptions{})
	if err != nil || out.Data.Ref == nil || out.Data.Ref.ID != "42" {
		t.Fatalf("selector changed after request: %+v %v", out, err)
	}
}
