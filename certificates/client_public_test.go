package certificates_test

import (
	"context"
	"encoding/json"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/certificates"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type wireTransport func(*http.Request) (*http.Response, error)

func (f wireTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCertificateGetMapsSourceMetadataAndExactRef(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	calls := 0
	root, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Service != "certificates" || r.Scope != scope || r.Mode != types.IAMProject {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: wireTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return &http.Response{StatusCode: 200, Header: http.Header{"X-Subject-Token": {"synthetic-token"}}, Body: io.NopCloser(strings.NewReader(`{"token":{"expires_at":"2099-01-01T00:00:00Z","user":{"id":"synthetic-user"},"project":{"id":"p","domain":{"name":"a"}},"catalog":[]}}`))}, nil
		}
		if r.Method != "GET" || r.URL.String() != "https://cloud.api.selcloud.ru/certificate-manager/v1/cert/cert-1" || r.Header.Get("X-Auth-Token") != "synthetic-token" {
			t.Fatal(r.Method, r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"cert-1","name":"example","version":9007199254740993,"private_key":{"type":"RSA"},"issued_by":{"commonName":"issuer","organization":["source-org"]},"validity":{"notBefore":"2026-10-16T00:00:00","notAfter":"2027-10-16T00:00:00","basic_constraints":false},"consumers":[{"id":"lb-1","region":"ru-1","type":"loadbalancer"}]}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	project, _ := root.Project("p")
	c := project.Certificates()
	ref, e := c.Ref("cert-1")
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.Get(context.Background(), ref, types.CallOptions{})
	if e != nil || got.Receipt != nil || got.Data.Ref.ID != "cert-1" || got.Data.Ref.Scope != scope || got.Data.Version.String() != "9007199254740993" || *got.Data.IssuedBy.CommonName != "issuer" || *got.Data.Validity.NotBefore != "2026-10-16T00:00:00" || calls != 2 {
		t.Fatal(got, e, calls)
	}
}

func TestCertificatePrivateMaterialRequiresExplicitReadsAndStaysProtected(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	reads := []string{}
	root, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: wireTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return &http.Response{StatusCode: 200, Header: http.Header{"X-Subject-Token": {"synthetic-token"}}, Body: io.NopCloser(strings.NewReader(`{"token":{"expires_at":"2099-01-01T00:00:00Z","user":{"id":"synthetic-user"},"project":{"id":"p","domain":{"name":"a"}},"catalog":[]}}`))}, nil
		}
		reads = append(reads, r.URL.Path)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("synthetic-sensitive-bytes"))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	project, _ := root.Project("p")
	c := project.Certificates()
	ref, _ := c.Ref("c")
	for _, read := range []func(context.Context, types.ResourceRef, types.CallOptions) (types.Result[types.ProtectedValue], error){c.ReadPrivateKey, c.ReadPKCS12, c.ReadPublicChain} {
		got, e := read(context.Background(), ref, types.CallOptions{})
		if e != nil || string(got.Data.Reveal()) != "synthetic-sensitive-bytes" {
			t.Fatal(got, e)
		}
		for _, s := range []string{fmt.Sprintf("%+v", got), fmt.Sprintf("%#v", got), func() string { raw, _ := json.Marshal(got); return string(raw) }()} {
			if strings.Contains(s, "synthetic-sensitive-bytes") || strings.Contains(s, "115 121") || strings.Contains(s, "115, 121") || strings.Contains(s, "c3ludGhldGljLXNlbnNpdGl2ZS1ieXRlcw==") || strings.Contains(s, "0x73, 0x79") {
				t.Fatal("protected bytes disclosed")
			}
		}
	}
	if strings.Join(reads, ",") != "/certificate-manager/v1/cert/c/private_key,/certificate-manager/v1/cert/c/p12,/certificate-manager/v1/cert/c/ca_chain" {
		t.Fatal(reads)
	}
}

func TestCertificateUploadSendsProtectedKeyAndRetainsAcceptedIdentity(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	root, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: wireTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return &http.Response{StatusCode: 200, Header: http.Header{"X-Subject-Token": {"synthetic-token"}}, Body: io.NopCloser(strings.NewReader(`{"token":{"expires_at":"2099-01-01T00:00:00Z","user":{"id":"synthetic-user"},"project":{"id":"p","domain":{"name":"a"}},"catalog":[]}}`))}, nil
		}
		if r.Method != "POST" || r.URL.Path != "/certificate-manager/v1/certs" {
			t.Fatal(r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"name":"source-name","pem":{"certificates":["synthetic-cert"],"private_key":"synthetic-private"}}` {
			t.Fatal("wrong request wire shape")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"created","version":"malformed"}`))}, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	project, _ := root.Project("p")
	c := project.Certificates()
	name := "source-name"
	certs := []string{"synthetic-cert"}
	key := types.Protect([]byte("synthetic-private"))
	out, e := c.Upload(context.Background(), certificates.UploadCertificateRequest{Name: &name, PEM: &certificates.PEMCertificate{Certificates: &certs, PrivateKey: &key}}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "created" {
		t.Fatal(out, e)
	}
}

func certificateClient(t *testing.T, wire wireTransport, configure ...func(*sdk.Config)) (*sdk.Client, *certificates.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	cfg := sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.IAMToken, Token: types.Protect([]byte("synthetic-token")), ExpiresAt: time.Now().Add(time.Hour)}, nil
	}), Transport: wireTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return &http.Response{StatusCode: 200, Header: http.Header{"X-Subject-Token": {"synthetic-token"}}, Body: io.NopCloser(strings.NewReader(`{"token":{"expires_at":"2099-01-01T00:00:00Z","user":{"id":"synthetic-user"},"project":{"id":"p","domain":{"name":"a"}},"catalog":[]}}`))}, nil
		}
		return wire(r)
	})}
	for _, change := range configure {
		change(&cfg)
	}
	root, e := sdk.New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	project, _ := root.Project("p")
	return root, project.Certificates()
}
func TestCertificateVersionAndNameUpdatesRetainExactReceipt(t *testing.T) {
	calls := 0
	_, c := certificateClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/certificate-manager/v1/cert/c" {
			t.Fatal(r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if calls == 1 && (r.Method != "POST" || string(raw) != `{"pem":{"certificates":[],"private_key":"protected-new-key"}}`) {
			t.Fatal("version request mismatch")
		}
		if calls == 2 && (r.Method != "PUT" || string(raw) != `{"name":""}`) {
			t.Fatal("name request mismatch")
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: http.NoBody}, nil
	})
	ref, _ := c.Ref("c")
	empty := []string{}
	key := types.Protect([]byte("protected-new-key"))
	out, e := c.UpdateVersion(context.Background(), ref, certificates.UpdateCertificateVersionRequest{PEM: &certificates.PEMCertificate{Certificates: &empty, PrivateKey: &key}}, types.CallOptions{})
	if e != nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].ID != "c" {
		t.Fatal(out, e)
	}
	name := ""
	out, e = c.UpdateName(context.Background(), ref, certificates.UpdateCertificateRequest{Name: &name}, types.CallOptions{})
	if e != nil || out.Receipt.Outcome != types.Accepted || calls != 2 {
		t.Fatal(out, e, calls)
	}
}

func TestCertificateConsumersMapBothDocumentedRequestForms(t *testing.T) {
	methods := []string{}
	_, c := certificateClient(t, func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.Method)
		if r.URL.Path != "/certificate-manager/v1/cert/c/consumers" {
			t.Fatal(r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"consumers":[{"id":"child","region":"source-region","type":"source-type"}],"id":"flat-child","region":"flat-region","type":"flat-type"}` {
			t.Fatal("consumer wire differs")
		}
		return &http.Response{StatusCode: 204, Header: http.Header{}, Body: http.NoBody}, nil
	})
	ref, _ := c.Ref("c")
	id, region, kind := "child", "source-region", "source-type"
	flatID, flatRegion, flatKind := "flat-child", "flat-region", "flat-type"
	items := []certificates.Consumer{{ID: &id, Region: &region, Type: &kind}}
	in := certificates.UpdateCertificateConsumerRequest{Consumers: &items, ID: &flatID, Region: &flatRegion, Type: &flatKind}
	for _, call := range []func(context.Context, types.ResourceRef, certificates.UpdateCertificateConsumerRequest, types.CallOptions) (types.Result[struct{}], error){c.AddConsumers, c.RemoveConsumers} {
		out, e := call(context.Background(), ref, in, types.CallOptions{})
		if e != nil || out.Receipt.Outcome != types.Accepted || out.Receipt.ResourceRefs[0].ID != "c" {
			t.Fatal(out, e)
		}
	}
	if strings.Join(methods, ",") != "PUT,DELETE" {
		t.Fatal(methods)
	}
}

func TestCertificateDeleteForceAndUnknownNeverReplay(t *testing.T) {
	for _, status := range []int{204, 403, 404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			_, c := certificateClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "DELETE" || r.URL.Path != "/certificate-manager/v1/cert/c" || r.URL.RawQuery != "force=false" {
					t.Fatal(r.Method, r.URL)
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: http.NoBody}, nil
			})
			ref, _ := c.Ref("c")
			force := false
			out, e := c.Delete(context.Background(), ref, certificates.DeleteOptions{Force: &force}, types.CallOptions{})
			want := types.Accepted
			if status == 403 || status == 404 {
				want = types.Rejected
			}
			if status == 500 {
				want = types.Unknown
			}
			if out.Receipt == nil || out.Receipt.Outcome != want || len(out.Receipt.ResourceRefs) != 1 || calls != 1 {
				t.Fatal(out, e, calls)
			}
			if status == 500 && types.ErrorCode(e) != types.UnknownOutcome {
				t.Fatal(e)
			}
			if status == 204 && e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestCertificateListBoundsReplayAndMalformedPrefix(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprint(malformed), func(t *testing.T) {
			calls := 0
			_, c := certificateClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/certificate-manager/v1/certs" || r.URL.RawQuery != "" {
					t.Fatal(r.Method, r.URL)
				}
				body := `[{"id":"a"},{"id":"b"}]`
				if malformed {
					body = `[{"id":"a"},{"id":"b","version":"bad"}]`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			it, e := c.List(pagination.Options{MaxItems: 1, MaxPages: 1})
			if e != nil {
				t.Fatal(e)
			}
			out, e := it.Collect(context.Background())
			if out.Complete || len(out.Items) != 1 || out.Items[0].Ref.ID != "a" || out.NextCursor == nil || out.NextCursor.Skip != 1 || calls != 1 {
				t.Fatal(out, e, calls)
			}
			if malformed {
				if types.ErrorCode(e) != types.ProtocolError {
					t.Fatal(e)
				}
				return
			}
			if types.ErrorCode(e) != types.Incomplete {
				t.Fatal(e)
			}
			it, e = c.List(pagination.Options{MaxItems: 5, MaxPages: 1, Cursor: out.NextCursor})
			if e != nil {
				t.Fatal(e)
			}
			next, e := it.Collect(context.Background())
			if e != nil || !next.Complete || len(next.Items) != 1 || next.Items[0].Ref.ID != "b" || calls != 2 {
				t.Fatal(next, e, calls)
			}
		})
	}
}

func TestCertificateResourceResumeObservesUnknownWithoutMutationReplay(t *testing.T) {
	writes, reads := 0, 0
	root, c := certificateClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/certificate-manager/v1/cert/c" {
			t.Fatal(r.URL)
		}
		if r.Method == "DELETE" {
			writes++
			return &http.Response{StatusCode: 500, Header: http.Header{}, Body: http.NoBody}, nil
		}
		if r.Method != "GET" {
			t.Fatal(r.Method)
		}
		reads++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"c","version":2}`))}, nil
	})
	ref, _ := c.Ref("c")
	out, e := c.Delete(context.Background(), ref, certificates.DeleteOptions{}, types.CallOptions{})
	if e == nil || out.Receipt.Outcome != types.Unknown {
		t.Fatal(out, e)
	}
	handle, e := root.Resume(*out.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	observed, e := handle.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || observed.ProofStatus != "indeterminate" || observed.Observation == nil || observed.Observation.Classification != "succeeded" || writes != 1 || reads != 1 {
		t.Fatal(observed, e, writes, reads)
	}
	handle, e = root.Resource(ref)
	if e != nil {
		t.Fatal(e)
	}
	status, e := handle.Status(context.Background())
	if e != nil || status.ProviderState != "present" || status.Classification != "succeeded" || reads != 2 || writes != 1 {
		t.Fatal(status, e, reads, writes)
	}
	known := *out.Receipt
	known.Outcome = types.Rejected
	known.ResourceRefs = nil
	known.Endpoint = nil
	handle, e = root.Resume(known)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := handle.Recover(context.Background())
	if e != nil || recovered.ProofStatus != "conclusive" || reads != 2 || writes != 1 {
		t.Fatal(recovered, e, reads, writes)
	}
}

type countedCertificateBody struct {
	reader io.Reader
	read   int
	closed bool
}

func (b *countedCertificateBody) Read(p []byte) (int, error) {
	n, e := b.reader.Read(p)
	b.read += n
	return n, e
}
func (b *countedCertificateBody) Close() error { b.closed = true; return nil }

type canceledCertificateBody struct {
	closed chan struct{}
	once   sync.Once
}

func (b *canceledCertificateBody) Read([]byte) (int, error) { <-b.closed; return 0, io.ErrClosedPipe }
func (b *canceledCertificateBody) Close() error             { b.once.Do(func() { close(b.closed) }); return nil }
func TestCertificateExplicitReadsPreserveBinaryAndBoundBytesAndBudget(t *testing.T) {
	t.Run("binary", func(t *testing.T) {
		_, c := certificateClient(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("\x00\xff\x01synthetic"))}, nil
		})
		ref, _ := c.Ref("c")
		out, e := c.ReadPKCS12(context.Background(), ref, types.CallOptions{})
		if e != nil || string(out.Data.Reveal()) != "\x00\xff\x01synthetic" {
			t.Fatal(out, e)
		}
	})
	t.Run("limit", func(t *testing.T) {
		body := &countedCertificateBody{reader: strings.NewReader(strings.Repeat("x", 4096))}
		calls := 0
		_, c := certificateClient(t, func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}, nil
		}, func(cfg *sdk.Config) { cfg.MaxJSONBytes = 256 })
		ref, _ := c.Ref("c")
		out, e := c.ReadPKCS12(context.Background(), ref, types.CallOptions{})
		if types.ErrorCode(e) != types.ResponseLimit || body.read != 257 || !body.closed || len(out.Data.Reveal()) != 0 || calls != 1 {
			t.Fatal(out, e, body.read, body.closed, calls)
		}
	})
	t.Run("budget", func(t *testing.T) {
		body := &canceledCertificateBody{closed: make(chan struct{})}
		defer body.Close()
		calls := 0
		_, c := certificateClient(t, func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: body}, nil
		}, func(cfg *sdk.Config) { cfg.TransferTimeout = 20 * time.Millisecond; cfg.ReadMaxAttempts = 3 })
		ref, _ := c.Ref("c")
		done := make(chan error, 1)
		go func() { _, e := c.ReadPrivateKey(context.Background(), ref, types.CallOptions{}); done <- e }()
		select {
		case e := <-done:
			if types.ErrorCode(e) != types.DeadlineExceeded || calls != 1 {
				t.Fatal(e, calls)
			}
		case <-time.After(time.Second):
			body.Close()
			<-done
			t.Fatal("private-read transfer budget did not close body")
		}
	})
}
func TestCertificateExactScopeGenerationEndpointAndCursorGuards(t *testing.T) {
	httpCalls, lookups := 0, 0
	root, c := certificateClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/certificate-manager/v1/certs" {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"id":"c"},{"id":"d"}]`))}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"c"}`))}, nil
	}, func(cfg *sdk.Config) {
		transport := cfg.Transport
		cfg.Transport = wireTransport(func(r *http.Request) (*http.Response, error) { httpCalls++; return transport.RoundTrip(r) })
		source := cfg.Credentials
		cfg.Credentials = credentials.SourceFunc(func(ctx context.Context, r types.AuthRequest) (credentials.Material, error) {
			lookups++
			return source.Lookup(ctx, r)
		})
	})
	ref, _ := c.Ref("c")
	for _, change := range []func(*types.ResourceRef){func(r *types.ResourceRef) { r.Scope.ProjectID = "foreign" }, func(r *types.ResourceRef) { r.APIGeneration = "lets_encrypt_v1" }, func(r *types.ResourceRef) { r.Kind = "lets_encrypt_certificate" }, func(r *types.ResourceRef) { parent := ref; r.Parent = &parent }} {
		forged := ref
		change(&forged)
		_, e := c.Get(context.Background(), forged, types.CallOptions{})
		if types.ErrorCode(e) != types.ScopeMismatch || httpCalls != 0 || lookups != 0 {
			t.Fatal(e, httpCalls, lookups)
		}
	}
	if _, e := c.Get(context.Background(), ref, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	httpCalls = 0
	forged := ref
	forged.EndpointURL = "https://foreign.invalid"
	if _, e := c.ReadPrivateKey(context.Background(), forged, types.CallOptions{}); types.ErrorCode(e) != types.EndpointUnavailable || httpCalls != 0 {
		t.Fatal(e, httpCalls)
	}
	it, _ := c.List(pagination.Options{MaxItems: 1})
	page, e := it.Next(context.Background())
	if types.ErrorCode(e) != types.Incomplete || page.NextCursor == nil {
		t.Fatal(e)
	}
	if page.NextCursor != nil {
		foreign, _ := root.Project("foreign")
		if _, e = foreign.Certificates().List(pagination.Options{Cursor: page.NextCursor}); types.ErrorCode(e) != types.ScopeMismatch {
			t.Fatal(e)
		}
	}
	out, e := c.Delete(context.Background(), forged, certificates.DeleteOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.EndpointUnavailable || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || httpCalls != 1 {
		t.Fatal(out, e)
	}
}

func TestCertificateUploadRejectsUnaddressableIdentityWithoutAdoption(t *testing.T) {
	for _, id := range []string{"", ".", "..", "bad\nidentity"} {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			calls := 0
			root, c := certificateClient(t, func(*http.Request) (*http.Response, error) {
				calls++
				raw, _ := json.Marshal(map[string]string{"id": id})
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})
			out, e := c.Upload(context.Background(), certificates.UploadCertificateRequest{}, types.CallOptions{})
			if types.ErrorCode(e) != types.ProtocolError || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 0 || calls != 1 {
				t.Fatal(out, e, calls)
			}
			handle, e := root.Resume(*out.Receipt)
			if e != nil {
				t.Fatal(e)
			}
			known, e := handle.Recover(context.Background())
			if e != nil || known.ProofStatus != "conclusive" || calls != 1 {
				t.Fatal(known, e, calls)
			}
		})
	}
}

func TestCertificateUploadAuthenticationRefusalIsConclusive(t *testing.T) {
	calls := 0
	_, c := certificateClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 401, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error_text":"synthetic-private","status_text":"secret detail"}`))}, nil
	})
	out, e := c.Upload(context.Background(), certificates.UploadCertificateRequest{}, types.CallOptions{})
	if types.ErrorCode(e) != types.Unauthenticated || out.Receipt == nil || out.Receipt.Outcome != types.Rejected || calls != 1 {
		t.Fatal(out, e, calls)
	}
	safe, _ := json.Marshal(e)
	if strings.Contains(string(safe), "synthetic-private") || strings.Contains(string(safe), "secret detail") {
		t.Fatal("provider error text disclosed")
	}
}

func TestCertificateObservationNeverClaimsPresenceOnReadFailure(t *testing.T) {
	root, c := certificateClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Header: http.Header{}, Body: http.NoBody}, nil
	})
	ref, _ := c.Ref("c")
	handle, e := root.Resource(ref)
	if e != nil {
		t.Fatal(e)
	}
	status, e := handle.Status(context.Background())
	if types.ErrorCode(e) != types.NotFound || status.Classification == "succeeded" || status.ProviderState == "present" {
		t.Fatal(status, e)
	}
}

func TestCertificateUnknownRecoveryDoesNotInferAbsenceFromAuth404(t *testing.T) {
	_, c := certificateClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: http.NoBody}, nil
	})
	ref, _ := c.Ref("c")
	out, e := c.Delete(context.Background(), ref, certificates.DeleteOptions{}, types.CallOptions{})
	if e == nil || out.Receipt == nil || out.Receipt.Outcome != types.Unknown {
		t.Fatal(out, e)
	}
	calls := 0
	root, _ := certificateClient(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("resource I/O after failed auth")
		return nil, nil
	}, func(cfg *sdk.Config) {
		cfg.Transport = wireTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path != "/identity/v3/auth/tokens" {
				t.Fatal(r.URL)
			}
			return &http.Response{StatusCode: 404, Header: http.Header{}, Body: http.NoBody}, nil
		})
	})
	handle, e := root.Resume(*out.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	observed, e := handle.Recover(context.Background())
	if types.ErrorCode(e) != types.EndpointUnavailable || observed.Observation != nil || calls != 1 {
		t.Fatal(observed, e, calls)
	}
}

func TestCertificateListCursorBindsResolvedEndpointURL(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	newClient := func(endpoint string, calls *int) *certificates.Client {
		_, c := certificateClient(t, wireTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != endpoint+"/certs" {
				t.Fatal(r.URL)
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`[{"id":"a"},{"id":"b"}]`))}, nil
		}), func(cfg *sdk.Config) {
			cfg.EndpointOverrides = map[types.EndpointKey]types.EndpointBinding{
				{Service: "certificates", APIGeneration: "user_v1", Scope: scope}: {ID: "same-id", URL: endpoint, Scope: scope, APIGeneration: "user_v1", Audience: "selectel"},
			}
			transport := cfg.Transport
			cfg.Transport = wireTransport(func(r *http.Request) (*http.Response, error) { *calls++; return transport.RoundTrip(r) })
		})
		return c
	}
	firstCalls := 0
	first := newClient("https://first.invalid/prefix", &firstCalls)
	it, err := first.List(pagination.Options{MaxItems: 1, MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	page, err := it.Collect(context.Background())
	if types.ErrorCode(err) != types.Incomplete || page.NextCursor == nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	foreignCalls := 0
	foreign := newClient("https://second.invalid/prefix", &foreignCalls)
	if _, err = foreign.List(pagination.Options{Cursor: page.NextCursor}); types.ErrorCode(err) != types.ScopeMismatch || foreignCalls != 0 {
		t.Fatalf("foreign endpoint accepted: err=%v HTTP=%d", err, foreignCalls)
	}
	originalCalls := 0
	original := newClient("https://first.invalid/prefix", &originalCalls)
	resumed, err := original.List(pagination.Options{MaxItems: 5, MaxPages: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	rest, err := resumed.Collect(context.Background())
	if err != nil || !rest.Complete || len(rest.Items) != 1 || rest.Items[0].Ref.ID != "b" || originalCalls == 0 {
		t.Fatal(rest, err, originalCalls)
	}
}
