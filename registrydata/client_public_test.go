package registrydata_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/registrydata"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type wireTransport func(*http.Request) (*http.Response, error)

func (f wireTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func dataClient(t *testing.T, wire wireTransport, configure ...func(*sdk.Config)) (*sdk.Client, *registrydata.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p"}
	cfg := sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Service != "registry_data" || r.Mode != types.Basic || r.Scope != scope {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "synthetic-registry", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.Basic, Username: types.Protect([]byte("saved-user")), Password: types.Protect([]byte("synthetic-registry-secret"))}, nil
	}), Transport: wire}
	for _, change := range configure {
		change(&cfg)
	}
	c, e := sdk.New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("p")
	return c, p.RegistryData()
}

func TestManifestPullNegotiatesScopedBearerAndVerifiesDigest(t *testing.T) {
	const payload = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","size":0},"layers":[]}`
	const digest = "sha256:aebe5b78f401a93ac344af2a85f218cdf8fc05fbbac41d7a3d960a44e0d2ee71"
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "cr.selcloud.ru" {
			t.Fatal("foreign I/O")
		}
		switch calls {
		case 1:
			if r.URL.Path != "/v2/" || r.Header.Get("Authorization") != "" {
				t.Fatal(r.URL, r.Header)
			}
			v := response(401, ``)
			v.Header.Set("WWW-Authenticate", `Bearer realm="https://cr.selcloud.ru/auth",service="cr.selcloud.ru",scope="repository:reg/repo:pull,push"`)
			return v, nil
		case 2:
			u, p, ok := r.BasicAuth()
			if r.URL.Path != "/auth" || !ok || u != "saved-user" || p != "synthetic-registry-secret" || r.URL.Query().Get("scope") != "repository:reg/repo:pull" || r.URL.Query().Get("service") != "cr.selcloud.ru" {
				t.Fatal("bad scoped token request")
			}
			return response(200, `{"token":"derived-bearer","expires_in":60}`), nil
		case 3:
			if r.URL.Path != "/v2/reg/repo/manifests/latest" || r.Header.Get("Authorization") != "Bearer derived-bearer" || r.Header.Get("Accept") != "application/vnd.oci.image.manifest.v1+json" {
				t.Fatal("bad manifest request")
			}
			v := response(200, payload)
			v.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			v.Header.Set("Docker-Content-Digest", digest)
			return v, nil
		}
		t.Fatal("extra I/O")
		return nil, nil
	})
	repo, e := c.Repository("reg", "repo")
	if e != nil {
		t.Fatal(e)
	}
	ref, e := c.Manifests.Tag(repo, "latest")
	if e != nil {
		t.Fatal(e)
	}
	v, e := c.Manifests.Get(context.Background(), ref, registrydata.ManifestReadOptions{Accept: []registrydata.ManifestMediaType{registrydata.OCIManifest}}, types.CallOptions{})
	if e != nil || v.Receipt != nil || v.Data.Digest.String() != digest || string(v.Data.Payload.Reveal()) != payload || v.Data.Address.ID != "latest" || v.Data.Address.Parent.ID != "repo" || calls != 3 {
		t.Fatal(v, e, calls)
	}
	safe, _ := json.Marshal(v)
	if strings.Contains(string(safe), "schemaVersion") {
		t.Fatal("payload exposed")
	}
}

func TestRegistryRejectsUntrustedOrAmbiguousChallengeBeforeCredentialIO(t *testing.T) {
	for _, tc := range []struct{ name, challenge string }{
		{"foreign_origin", `Bearer realm="https://foreign.example/token",service="cr.selcloud.ru"`},
		{"cleartext", `Bearer realm="http://cr.selcloud.ru/auth",service="cr.selcloud.ru"`},
		{"userinfo", `Bearer realm="https://name@cr.selcloud.ru/auth",service="cr.selcloud.ru"`},
		{"fragment", `Bearer realm="https://cr.selcloud.ru/auth#fragment",service="cr.selcloud.ru"`},
		{"duplicate_realm", `Bearer realm="https://cr.selcloud.ru/auth",realm="https://cr.selcloud.ru/other",service="cr.selcloud.ru"`},
		{"missing_service", `Bearer realm="https://cr.selcloud.ru/auth"`},
		{"realm_query_auth_override", `Bearer realm="https://cr.selcloud.ru/auth?offline_token=true",service="cr.selcloud.ru"`},
		{"broken_quote", `Bearer realm="https://cr.selcloud.ru/auth,service="cr.selcloud.ru"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if calls != 1 || r.URL.Path != "/v2/" || r.Header.Get("Authorization") != "" {
					t.Fatal("credential or resource I/O after invalid challenge")
				}
				v := response(401, ``)
				v.Header.Set("WWW-Authenticate", tc.challenge)
				return v, nil
			})
			repo, _ := c.Repository("reg", "repo")
			ref, _ := c.Manifests.Tag(repo, "latest")
			_, e := c.Manifests.Get(context.Background(), ref, registrydata.ManifestReadOptions{}, types.CallOptions{})
			if types.ErrorCode(e) != types.ProtocolError || calls != 1 {
				t.Fatal(e, calls)
			}
		})
	}
}

func TestRegistryManifestFormatsAndDigestAddressesRemainExplicit(t *testing.T) {
	for _, media := range []registrydata.ManifestMediaType{registrydata.OCIManifest, registrydata.OCIIndex, registrydata.DockerManifest, registrydata.DockerManifestList} {
		t.Run(string(media), func(t *testing.T) {
			calls := 0
			var payload string
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path == "/v2/" {
					return response(200, ``), nil
				}
				if r.Header.Get("Accept") != string(media) {
					t.Fatal("accept")
				}
				payload = `{"schemaVersion":2,"mediaType":"` + string(media) + `","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","size":0},"layers":[],"manifests":[]}`
				v := response(200, payload)
				v.Header.Set("Content-Type", string(media)+"; charset=utf-8")
				return v, nil
			})
			repo, _ := c.Repository("reg", "team/repo")
			ref, _ := c.Manifests.Tag(repo, "v1")
			v, e := c.Manifests.Get(context.Background(), ref, registrydata.ManifestReadOptions{Accept: []registrydata.ManifestMediaType{media}}, types.CallOptions{})
			if e != nil || string(v.Data.Payload.Reveal()) != payload || v.Data.MediaType != media || v.Data.Digest.Algorithm != "sha256" || calls != 2 {
				t.Fatal(v, e, calls)
			}
		})
	}
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		t.Fatal("invalid digest reached HTTP")
		return nil, nil
	})
	repo, _ := c.Repository("reg", "repo")
	digest, e := registrydata.ParseDigest("sha256:aebe5b78f401a93ac344af2a85f218cdf8fc05fbbac41d7a3d960a44e0d2ee71")
	if e != nil {
		t.Fatal(e)
	}
	ref, e := c.Manifests.Digest(repo, digest)
	if e != nil || ref.Kind != "manifest_digest" || ref.ID != digest.String() {
		t.Fatal(ref, e)
	}
	ref.ID = "not-a-digest"
	_, e = c.Manifests.Get(context.Background(), ref, registrydata.ManifestReadOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 0 {
		t.Fatal(e, calls)
	}
}

func TestRegistryErrorCodeKeepsStatusAndDiscardsProviderText(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ``), nil
		}
		return response(404, `{"errors":[{"code":"MANIFEST_UNKNOWN","message":"sensitive-provider-message","detail":{"secret":"sensitive-detail"}}]}`), nil
	})
	repo, _ := c.Repository("reg", "repo")
	ref, _ := c.Manifests.Tag(repo, "missing")
	_, e := c.Manifests.Get(context.Background(), ref, registrydata.ManifestReadOptions{}, types.CallOptions{})
	var sdkError *types.SDKError
	if !errors.As(e, &sdkError) || sdkError.Code != types.NotFound || sdkError.ProviderCode != "MANIFEST_UNKNOWN" || calls != 2 {
		t.Fatal(e, calls)
	}
	safe, _ := json.Marshal(e)
	if strings.Contains(string(safe), "sensitive-") || strings.Contains(fmt.Sprintf("%#v", e), "sensitive-") {
		t.Fatal("provider payload disclosed")
	}
}

func TestAuthEndpointAbsenceIsNotManifestAbsence(t *testing.T) {
	for _, stage := range []string{"probe", "token"} {
		t.Run(stage, func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if stage == "token" && r.URL.Path == "/v2/" {
					v := response(401, ``)
					v.Header.Set("WWW-Authenticate", `Bearer realm="https://cr.selcloud.ru/auth",service="cr.selcloud.ru"`)
					return v, nil
				}
				return response(404, ``), nil
			})
			repo, _ := c.Repository("reg", "repo")
			ref, _ := c.Manifests.Tag(repo, "latest")
			_, e := c.Manifests.Get(context.Background(), ref, registrydata.ManifestReadOptions{}, types.CallOptions{})
			if types.ErrorCode(e) != types.EndpointUnavailable || calls > 2 {
				t.Fatal(e, calls)
			}
		})
	}
}

func TestRegistryManifestHeadPushAndExplicitDeletionKeepReceipts(t *testing.T) {
	const payload = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","size":0},"layers":[]}`
	const digest = "sha256:aebe5b78f401a93ac344af2a85f218cdf8fc05fbbac41d7a3d960a44e0d2ee71"
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ``), nil
		}
		switch r.Method {
		case "HEAD":
			v := response(200, ``)
			v.Header.Set("Content-Type", string(registrydata.OCIManifest))
			v.Header.Set("Content-Length", "9007199254740993")
			v.Header.Set("Docker-Content-Digest", digest)
			return v, nil
		case "PUT":
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != payload || r.Header.Get("Content-Type") != string(registrydata.OCIManifest) || r.URL.Path != "/v2/reg/repo/manifests/latest" {
				t.Fatal("bad push")
			}
			v := response(201, ``)
			v.Header.Set("Location", "/v2/reg/repo/manifests/"+digest)
			v.Header.Set("Docker-Content-Digest", digest)
			return v, nil
		case "DELETE":
			if r.URL.Path != "/v2/reg/repo/manifests/"+digest && r.URL.Path != "/v2/reg/repo/manifests/latest" {
				t.Fatal(r.URL)
			}
			return response(202, ``), nil
		}
		t.Fatal(r.Method, r.URL)
		return nil, nil
	})
	repo, _ := c.Repository("reg", "repo")
	tag, _ := c.Manifests.Tag(repo, "latest")
	ctx := context.Background()
	head, e := c.Manifests.Head(ctx, tag, registrydata.ManifestReadOptions{}, types.CallOptions{})
	if e != nil || head.Data.Size.String() != "9007199254740993" || head.Data.Digest.String() != digest || head.Data.DigestVerified {
		t.Fatal(head, e)
	}
	var document types.ProtectedJSON
	_ = json.Unmarshal([]byte(payload), &document)
	pushed, e := c.Manifests.Put(ctx, tag, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: document}, types.CallOptions{})
	if e != nil || pushed.Receipt == nil || pushed.Receipt.Outcome != types.Accepted || pushed.Receipt.ResourceRefs[0].ID != "latest" || pushed.Data.Digest.String() != digest || !strings.Contains(string(pushed.Data.Location.Reveal()), digest) {
		t.Fatal(pushed, e)
	}
	d, _ := registrydata.ParseDigest(digest)
	ref, _ := c.Manifests.Digest(repo, d)
	deleted, e := c.Manifests.Delete(ctx, ref, types.CallOptions{})
	if e != nil || deleted.Receipt == nil || deleted.Receipt.Outcome != types.Accepted || deleted.Receipt.ResourceRefs[0].ID != digest {
		t.Fatal(deleted, e)
	}
	deleted, e = c.Manifests.DeleteTag(ctx, tag, types.CallOptions{})
	if e != nil || deleted.Receipt.ResourceRefs[0].ID != "latest" || calls != 8 {
		t.Fatal(deleted, e, calls)
	}
}

func TestManifestPushAuthRequestsPublishedPullPushAccessOnly(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			v := response(401, ``)
			v.Header.Set("WWW-Authenticate", `Bearer realm="https://cr.selcloud.ru/auth",service="cr.selcloud.ru",scope="repository:foreign/repo:pull,push,delete"`)
			return v, nil
		case 2:
			if r.URL.Path != "/auth" || r.URL.Query().Get("scope") != "repository:reg/repo:pull,push" {
				t.Fatal("wrong compiled push scope", r.URL.Query().Get("scope"))
			}
			return response(200, `{"access_token":"scoped-push-token"}`), nil
		case 3:
			if r.Method != "PUT" || r.URL.Path != "/v2/reg/repo/manifests/latest" || r.Header.Get("Authorization") != "Bearer scoped-push-token" {
				t.Fatal("wrong resource request")
			}
			v := response(201, ``)
			v.Header.Set("Location", "/v2/reg/repo/manifests/latest")
			return v, nil
		}
		t.Fatal("extra I/O")
		return nil, nil
	})
	repo, _ := c.Repository("reg", "repo")
	ref, _ := c.Manifests.Tag(repo, "latest")
	var payload types.ProtectedJSON
	_ = json.Unmarshal([]byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`), &payload)
	v, e := c.Manifests.Put(context.Background(), ref, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIIndex, Payload: payload}, types.CallOptions{})
	if e != nil || v.Receipt == nil || v.Receipt.Outcome != types.Accepted || calls != 3 {
		t.Fatal(v, e, calls)
	}
}

func TestRegistryBlobDownloadAndResumeVerifyWholeDigestWithoutBuffering(t *testing.T) {
	for _, offset := range []uint64{0, 2} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path == "/v2/" {
					return response(200, ``), nil
				}
				if r.URL.Path != "/v2/reg/repo/blobs/sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" || r.Header.Get("Accept-Encoding") != "identity" {
					t.Fatal("wrong blob request")
				}
				v := response(200, "hello")
				if offset == 2 {
					if r.Header.Get("Range") != "bytes=2-" {
						t.Fatal("range")
					}
					v = response(206, "llo")
					v.Header.Set("Content-Range", "bytes 2-4/5")
				}
				v.Header.Set("Docker-Content-Digest", "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
				return v, nil
			})
			repo, _ := c.Repository("reg", "repo")
			digest, _ := registrydata.ParseDigest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
			ref, e := c.Blobs.Digest(repo, digest)
			if e != nil {
				t.Fatal(e)
			}
			var destination bytes.Buffer
			opts := registrydata.BlobDownloadOptions{MaxBytes: 5, Offset: offset}
			if offset == 2 {
				destination.WriteString("he")
				opts.Prefix = strings.NewReader("he")
			}
			v, e := c.Blobs.Download(context.Background(), ref, &destination, opts, types.CallOptions{})
			if e != nil || !v.Data.Complete || !v.Data.DigestVerified || v.Data.ExpectedDigest != digest || v.Data.ActualDigest == nil || *v.Data.ActualDigest != digest || v.Data.TotalBytes.String() != "5" || v.Data.BytesTransferred.String() != fmt.Sprint(5-offset) || destination.String() != "hello" || calls != 2 {
				t.Fatal(v, e, calls, destination.String())
			}
		})
	}
}

type countedBody struct {
	remaining, read int
	closed          bool
}

func (b *countedBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if b.remaining < n {
		n = b.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'h'
	}
	b.remaining -= n
	b.read += n
	return n, nil
}
func (b *countedBody) Close() error { b.closed = true; return nil }
func TestRegistryBlobLimitDoesNotBufferOrExposeUnverifiedCompletion(t *testing.T) {
	body := &countedBody{remaining: 100000}
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ``), nil
		}
		v := response(200, ``)
		v.Body = body
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	digest, _ := registrydata.ParseDigest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	ref, _ := c.Blobs.Digest(repo, digest)
	var out bytes.Buffer
	v, e := c.Blobs.Download(context.Background(), ref, &out, registrydata.BlobDownloadOptions{MaxBytes: 3}, types.CallOptions{})
	if types.ErrorCode(e) != types.ResponseLimit || v.Data.Complete || v.Data.DigestVerified || v.Data.BytesTransferred.String() != "3" || out.String() != "hhh" || body.read != 4 || !body.closed {
		t.Fatal(v, e, out.String(), body.read, body.closed)
	}
}

func TestRegistryBlobResumeRejectsFullFallbackWrongRangeAndWrongPrefix(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		status                 int
		span, prefix, expected string
		written                bool
	}{{"full_fallback", 200, "", "he", "he", false}, {"wrong_range", 206, "bytes 1-3/5", "he", "he", false}, {"wrong_prefix", 206, "bytes 2-4/5", "xx", "xxllo", true}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path == "/v2/" {
					return response(200, ``), nil
				}
				v := response(tc.status, "llo")
				if tc.span != "" {
					v.Header.Set("Content-Range", tc.span)
				}
				return v, nil
			})
			repo, _ := c.Repository("reg", "repo")
			digest, _ := registrydata.ParseDigest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
			ref, _ := c.Blobs.Digest(repo, digest)
			out := bytes.NewBufferString(tc.prefix)
			v, e := c.Blobs.Download(context.Background(), ref, out, registrydata.BlobDownloadOptions{MaxBytes: 5, Offset: 2, Prefix: strings.NewReader(tc.prefix)}, types.CallOptions{})
			if types.ErrorCode(e) != types.ProtocolError || v.Data.Complete || v.Data.DigestVerified || out.String() != tc.expected || calls != 2 {
				t.Fatal(v, e, out.String(), calls)
			}
			if tc.written && v.Data.BytesTransferred.String() != "3" {
				t.Fatal(v)
			}
		})
	}
}

func TestRegistryBlobHeadDeleteUseExactDigestAndUnverifiedMetadata(t *testing.T) {
	const value = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ``), nil
		}
		if r.URL.Path != "/v2/reg/repo/blobs/"+value {
			t.Fatal("wrong digest path")
		}
		if r.Method == "HEAD" {
			v := response(200, ``)
			v.Header.Set("Docker-Content-Digest", value)
			v.Header.Set("Content-Length", "9007199254740993")
			v.Header.Set("Content-Type", "application/octet-stream")
			return v, nil
		}
		if r.Method == "DELETE" {
			return response(202, ``), nil
		}
		t.Fatal(r.Method)
		return nil, nil
	})
	repo, _ := c.Repository("reg", "repo")
	digest, _ := registrydata.ParseDigest(value)
	ref, _ := c.Blobs.Digest(repo, digest)
	head, e := c.Blobs.Head(context.Background(), ref, types.CallOptions{})
	if e != nil || head.Data.Digest.String() != value || head.Data.DigestVerified || head.Data.Size.String() != "9007199254740993" || *head.Data.MediaType != "application/octet-stream" {
		t.Fatal(head, e)
	}
	deleted, e := c.Blobs.Delete(context.Background(), ref, types.CallOptions{})
	if e != nil || deleted.Receipt == nil || deleted.Receipt.Outcome != types.Accepted || deleted.Receipt.ResourceRefs[0].ID != value || calls != 4 {
		t.Fatal(deleted, e, calls)
	}
}

type untilClosedBody struct {
	done chan struct{}
	once sync.Once
}

func (b *untilClosedBody) Read([]byte) (int, error) { <-b.done; return 0, io.ErrClosedPipe }
func (b *untilClosedBody) Close() error             { b.once.Do(func() { close(b.done) }); return nil }
func TestRegistryAuthAndBlobStreamShareCancelableTransferBudget(t *testing.T) {
	for _, stage := range []string{"token", "blob"} {
		t.Run(stage, func(t *testing.T) {
			blocked := &untilClosedBody{done: make(chan struct{})}
			defer blocked.Close()
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path == "/v2/" {
					if stage == "blob" {
						return response(200, ``), nil
					}
					v := response(401, ``)
					v.Header.Set("WWW-Authenticate", `Bearer realm="https://cr.selcloud.ru/auth",service="cr.selcloud.ru"`)
					return v, nil
				}
				v := response(200, ``)
				v.Body = blocked
				return v, nil
			}, func(cfg *sdk.Config) { cfg.TransferTimeout = 20 * time.Millisecond })
			repo, _ := c.Repository("reg", "repo")
			digest, _ := registrydata.ParseDigest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
			ref, _ := c.Blobs.Digest(repo, digest)
			done := make(chan error, 1)
			go func() {
				var out bytes.Buffer
				v, e := c.Blobs.Download(context.Background(), ref, &out, registrydata.BlobDownloadOptions{MaxBytes: 5}, types.CallOptions{})
				if v.Data.Complete || v.Data.DigestVerified {
					done <- errors.New("canceled transfer reported complete")
					return
				}
				done <- e
			}()
			select {
			case e := <-done:
				if types.ErrorCode(e) != types.DeadlineExceeded || calls != 2 {
					t.Fatal(e, calls)
				}
			case <-time.After(time.Second):
				blocked.Close()
				<-done
				t.Fatal("transfer budget did not cancel protocol body")
			}
			select {
			case <-blocked.done:
			default:
				t.Fatal("body not closed")
			}
		})
	}
}

func TestRegistryDiagnosticLimitPreservesPermissionAuthAndSafeRetry(t *testing.T) {
	for _, status := range []int{401, 403, 404, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path == "/v2/" {
					return response(200, ``), nil
				}
				if status == 503 && calls == 3 {
					return response(403, ``), nil
				}
				return response(status, strings.Repeat("x", 512)), nil
			}, func(cfg *sdk.Config) {
				cfg.MaxJSONBytes = 64
				cfg.ReadMaxAttempts = 2
				cfg.RetryBaseDelay = time.Nanosecond
				cfg.RetryMaxDelay = time.Nanosecond
			})
			repo, _ := c.Repository("reg", "repo")
			ref, _ := c.Manifests.Tag(repo, "latest")
			_, e := c.Manifests.Get(context.Background(), ref, registrydata.ManifestReadOptions{}, types.CallOptions{})
			expected := map[int]types.Code{401: types.Unauthenticated, 403: types.PermissionDenied, 404: types.NotFound, 503: types.PermissionDenied}[status]
			var sdkError *types.SDKError
			if !errors.As(e, &sdkError) || sdkError.Code != expected || sdkError.ProviderCode != "" || calls != 2 && status != 503 || status == 503 && calls != 3 {
				t.Fatal(e, calls)
			}
		})
	}
}

func TestUploadStartAndStatusRetainProtectedExactLocation(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		v := response(202, "")
		v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u-1?_state=opaque-secret")
		if r.Method == "POST" {
			if r.URL.Path != "/v2/reg/repo/blobs/uploads/" {
				t.Fatal(r.URL)
			}
			return v, nil
		}
		if r.Method != "GET" || r.URL.Path != "/v2/reg/repo/blobs/uploads/u-1" || r.URL.Query().Get("_state") != "opaque-secret" {
			t.Fatal(r.Method, r.URL)
		}
		v.StatusCode = 204
		v.Header.Set("Range", "0-4")
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	started, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil || started.Receipt == nil || started.Receipt.Outcome != types.Accepted || started.Data.Ref.ID != "u-1" {
		t.Fatal(started, e)
	}
	state, e := c.Uploads.Status(context.Background(), started.Data, types.CallOptions{})
	if e != nil || state.Data.Offset != 5 || state.Data.Ref.Parent.ID != "repo" || calls != 4 {
		t.Fatal(state, e, calls)
	}
	for _, value := range []any{started, state} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "opaque-secret") || strings.Contains(string(raw), "_state") {
			t.Fatal("protected location exposed")
		}
	}
}

func TestUploadAppendAuthenticatesBeforeReadingAndNeverReplaysUnknown(t *testing.T) {
	calls, reads := 0, 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			if reads != 0 {
				t.Fatal("body read before handshake")
			}
			return response(200, ""), nil
		}
		if r.Method == "POST" {
			v := response(202, "")
			v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u?_state=saved")
			return v, nil
		}
		if r.Method != "PATCH" || r.Header.Get("Content-Range") != "0-4" || r.ContentLength != 5 || r.URL.Query().Get("_state") != "saved" {
			t.Fatal(r.Method, r.Header, r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "hello" {
			t.Fatal(string(body))
		}
		return nil, errors.New("synthetic disconnected")
	})
	repo, _ := c.Repository("reg", "repo")
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	body := &countReader{reads: &reads, Reader: strings.NewReader("hello")}
	result, e := c.Uploads.Append(context.Background(), start.Data, body, 5, types.CallOptions{})
	if e == nil || result.Receipt == nil || result.Receipt.Outcome != types.Unknown || calls != 4 || reads == 0 || len(result.Receipt.ResourceRefs) != 1 || result.Receipt.ResourceRefs[0].ID != "u" {
		t.Fatal(result, e, calls, reads)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "_state") {
		t.Fatal("state disclosed")
	}
}

type countReader struct {
	io.Reader
	reads *int
}

func (r *countReader) Read(p []byte) (int, error) { *r.reads++; return r.Reader.Read(p) }

func TestUploadAppendCarriesUpdatedLocationAndExactOffset(t *testing.T) {
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		v := response(202, "")
		v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u?_state=new")
		if r.Method == "PATCH" {
			v.Header.Set("Range", "0-4")
			body, _ := io.ReadAll(r.Body)
			if string(body) != "hello" {
				t.Fatal(string(body))
			}
		}
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	out, e := c.Uploads.Append(context.Background(), start.Data, strings.NewReader("hello"), 5, types.CallOptions{})
	if e != nil || out.Receipt.Outcome != types.Accepted || out.Data.Offset != 5 || !strings.Contains(string(out.Data.Location.Reveal()), "_state=new") {
		t.Fatal(out, e)
	}
}

func TestUploadFinalizeWholeStreamVerifiesDigestAndBlobLocation(t *testing.T) {
	const digest = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		if r.Method == "POST" {
			v := response(202, "")
			v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u?_state=opaque")
			return v, nil
		}
		if r.Method != "PUT" || r.URL.Query().Get("digest") != digest || r.URL.Query().Get("_state") != "opaque" || r.ContentLength != 5 {
			t.Fatal(r.Method, r.URL)
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != "hello" {
			t.Fatal(string(raw))
		}
		v := response(201, "")
		v.Header.Set("Location", "/v2/reg/repo/blobs/"+digest)
		v.Header.Set("Docker-Content-Digest", digest)
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	d, _ := registrydata.ParseDigest(digest)
	result, e := c.Uploads.Finalize(context.Background(), start.Data, d, strings.NewReader("hello"), 5, types.CallOptions{})
	if e != nil || result.Receipt.Outcome != types.Accepted || result.Data.Address.ID != digest || !result.Data.DigestVerified || len(result.Receipt.ResourceRefs) != 2 || calls != 4 {
		t.Fatal(result, e, calls)
	}
}

func TestUploadLocationCriticalQueryBytesSurviveStatusAndFinalize(t *testing.T) {
	const critical = "z=%2f%2F&x=a%20b&x=c+z"
	const digest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		v := response(202, "")
		v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u?"+critical)
		switch r.Method {
		case "GET":
			if r.URL.RawQuery != critical {
				t.Fatalf("query changed: %s", r.URL.RawQuery)
			}
			v.StatusCode = 204
			v.Header.Set("Range", "0-0")
		case "PUT":
			if r.URL.RawQuery != critical+"&digest=sha256%3Ae3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
				t.Fatalf("query changed: %s", r.URL.RawQuery)
			}
			v.StatusCode = 201
			v.Header.Set("Location", "/v2/reg/repo/blobs/"+digest)
		}
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Uploads.Status(context.Background(), start.Data, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	d, _ := registrydata.ParseDigest(digest)
	if _, e = c.Uploads.Finalize(context.Background(), start.Data, d, nil, 0, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
}

func TestUploadCancelExplicitNativeAndLocationGuards(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		if r.Method == "DELETE" {
			if r.URL.Query().Get("state") != "saved" {
				t.Fatal(r.URL)
			}
			return response(204, ""), nil
		}
		v := response(202, "")
		v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u?state=saved")
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	result, e := c.Uploads.CancelNative(context.Background(), start.Data, types.CallOptions{})
	if e != nil || result.Receipt.Outcome != types.Accepted || calls != 4 {
		t.Fatal(result, e, calls)
	}
	for _, location := range []string{"https://foreign.invalid/v2/reg/repo/blobs/uploads/u", "/v2/other/repo/blobs/uploads/u", "/v2/reg/repo/blobs/uploads/../u", "/v2/reg/repo/blobs/uploads/u?digest=bad", "/v2/reg/repo/blobs/uploads/u#fragment"} {
		saved := start.Data
		saved.Location = types.Protect([]byte(location))
		_, e = c.Uploads.CancelNative(context.Background(), saved, types.CallOptions{})
		if e == nil || calls != 4 {
			t.Fatal("untrusted location reached I/O", e, calls)
		}
	}
}

func TestRegistryVersionAndBoundedTagsUseExactRepository(t *testing.T) {
	lists := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			v := response(200, "")
			v.Header.Set("Docker-Distribution-API-Version", "registry/2.0")
			return v, nil
		}
		if r.URL.Path != "/v2/reg/repo/tags/list" || r.URL.Query().Get("n") != "2" {
			t.Fatal(r.URL)
		}
		lists++
		if lists == 1 {
			v := response(200, `{"name":"reg/repo","tags":["a"]}`)
			v.Header.Set("Link", `</v2/reg/repo/tags/list?n=2&last=a>; rel="next"`)
			return v, nil
		}
		if r.URL.Query().Get("last") != "a" {
			t.Fatal(r.URL)
		}
		return response(200, `{"name":"reg/repo","tags":["b"]}`), nil
	})
	version, e := c.Version(context.Background(), types.CallOptions{})
	if e != nil || !version.Data.Supported || version.Data.DistributionVersion == nil || *version.Data.DistributionVersion != "registry/2.0" {
		t.Fatal(version, e)
	}
	repo, _ := c.Repository("reg", "repo")
	it, e := c.Tags.List(repo, pagination.Options{PageSize: 2, MaxItems: 10, MaxPages: 5})
	if e != nil {
		t.Fatal(e)
	}
	out, e := it.Collect(context.Background())
	if e != nil || !out.Complete || len(out.Items) != 2 || out.Items[0].ID != "a" || out.Items[1].ID != "b" || lists != 2 {
		t.Fatal(out, e)
	}
}

func TestRegistryResourceResumeObservesExactContentAndKeepsKnownUploadReceipt(t *testing.T) {
	calls := 0
	root, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		if r.Method == "POST" {
			v := response(202, "")
			v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u?state=protected")
			return v, nil
		}
		if r.Method != "HEAD" || r.URL.Path != "/v2/reg/repo/manifests/latest" {
			t.Fatal(r.Method, r.URL)
		}
		return response(200, ""), nil
	})
	repo, _ := c.Repository("reg", "repo")
	tag, _ := c.Manifests.Tag(repo, "latest")
	handle, e := root.Resource(tag)
	if e != nil {
		t.Fatal(e)
	}
	status, e := handle.Status(context.Background())
	if e != nil || status.Classification != "succeeded" || status.ProviderState != "present" || calls != 2 {
		t.Fatal(status, e, calls)
	}
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	handle, e = root.Resume(*start.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := handle.Recover(context.Background())
	if e != nil || recovered.ProofStatus != "conclusive" || calls != 4 || len(recovered.Receipt.ResourceRefs) != 2 {
		t.Fatal(recovered, e, calls)
	}
	unknown := *start.Receipt
	unknown.Outcome = types.Unknown
	unknown.ResourceRefs = unknown.ResourceRefs[:1]
	handle, e = root.Resume(unknown)
	if e != nil {
		t.Fatal(e)
	}
	_, e = handle.Recover(context.Background())
	if types.ErrorCode(e) != types.RecoveryUnavailable || calls != 4 {
		t.Fatal(e, calls)
	}
}

func TestRegistryHandleRejectsUnrelatedReceiptBeforeIO(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		return response(202, ""), nil
	})
	repo, _ := c.Repository("reg", "repo")
	tag, _ := c.Manifests.Tag(repo, "latest")
	other, _ := c.Manifests.Tag(repo, "other")
	removed, e := c.Manifests.DeleteTag(context.Background(), tag, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Handle(other, removed.Receipt); types.ErrorCode(e) != types.ScopeMismatch || calls != 2 {
		t.Fatal("unrelated receipt accepted", e, calls)
	}
	saved := *removed.Receipt
	saved.Scope.ProjectID = "other"
	if _, e = c.Handle(tag, &saved); types.ErrorCode(e) != types.ScopeMismatch || calls != 2 {
		t.Fatal("wrong receipt view", e, calls)
	}
}

func TestManifestReadRejectsAmbiguousMetadataHeaders(t *testing.T) {
	for _, key := range []string{"Content-Type", "Docker-Content-Digest"} {
		t.Run(key, func(t *testing.T) {
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				v := response(200, `{}`)
				v.Header.Set("Content-Type", string(registrydata.OCIManifest))
				v.Header.Set("Docker-Content-Digest", "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a")
				v.Header.Add(key, v.Header.Get(key))
				return v, nil
			})
			repo, _ := c.Repository("reg", "repo")
			tag, _ := c.Manifests.Tag(repo, "latest")
			_, e := c.Manifests.Get(context.Background(), tag, registrydata.ManifestReadOptions{}, types.CallOptions{})
			if types.ErrorCode(e) != types.ProtocolError {
				t.Fatal(e)
			}
		})
	}
}

func TestRegistryTagsPreserveBoundedValidatedPrefixWithoutExtraIO(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		return response(200, `{"name":"reg/repo","tags":["a","bad tag"]}`), nil
	})
	repo, _ := c.Repository("reg", "repo")
	it, e := c.Tags.List(repo, pagination.Options{PageSize: 2, MaxItems: 10, MaxPages: 5})
	if e != nil {
		t.Fatal(e)
	}
	repo.Parent.ID = "changed-by-caller"
	out, e := it.Collect(context.Background())
	if types.ErrorCode(e) != types.ProtocolError || out.Complete || len(out.Items) != 1 || out.Items[0].ID != "a" || out.Items[0].Parent.Parent.ID != "reg" || out.NextCursor == nil || out.NextCursor.Skip != 1 || calls != 2 {
		t.Fatal(out, e, calls)
	}
	if _, e = it.Next(context.Background()); types.ErrorCode(e) != types.ProtocolError || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestUploadFinalizeNilNonemptyBodyIsNotSentAndRecoverable(t *testing.T) {
	calls := 0
	root, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		v := response(202, "")
		v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u")
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	d, _ := registrydata.ParseDigest("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	out, e := c.Uploads.Finalize(context.Background(), start.Data, d, nil, 1, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || out.Receipt.ContractRevision == "" || calls != 2 {
		t.Fatal(out, e, calls)
	}
	handle, e := root.Resume(*out.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovery, e := handle.Recover(context.Background())
	if e != nil || recovery.ProofStatus != "conclusive" || calls != 2 {
		t.Fatal(recovery, e, calls)
	}
}

func TestFinalizeMalformedAcceptedMetadataRetainsBlobIdentity(t *testing.T) {
	const digest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		v := response(202, "")
		v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u")
		if r.Method == "PUT" {
			v.StatusCode = 201
			v.Header.Set("Location", "/v2/reg/repo/blobs/"+digest)
			v.Header.Set("Docker-Content-Digest", "malformed")
		}
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	d, _ := registrydata.ParseDigest(digest)
	out, e := c.Uploads.Finalize(context.Background(), start.Data, d, nil, 0, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.ResourceRefs[1].ID != digest || out.Data.DigestVerified {
		t.Fatal(out, e)
	}
}

func TestRegistryExactRefsRejectChangedEndpointWithoutIO(t *testing.T) {
	_, original := dataClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("constructors made I/O"); return nil, nil })
	repo, _ := original.Repository("reg", "repo")
	tag, _ := original.Manifests.Tag(repo, "latest")
	calls := 0
	root, changed := dataClient(t, func(*http.Request) (*http.Response, error) { calls++; return response(200, ""), nil }, func(cfg *sdk.Config) {
		scope := types.Scope{AccountID: "a", ProjectID: "p"}
		cfg.EndpointOverrides = map[types.EndpointKey]types.EndpointBinding{{Service: "registry_data", APIGeneration: "oci_v1", Scope: scope}: {ID: "replacement", URL: "https://replacement.invalid", Scope: scope, APIGeneration: "oci_v1", Audience: "selectel"}}
	})
	_, e := changed.Manifests.Head(context.Background(), tag, registrydata.ManifestReadOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.EndpointUnavailable || calls != 0 {
		t.Fatal(e, calls)
	}
	handle, e := root.Resource(tag)
	if e != nil {
		t.Fatal(e)
	}
	_, e = handle.Status(context.Background())
	if types.ErrorCode(e) != types.EndpointUnavailable || calls != 0 {
		t.Fatal(e, calls)
	}
	tag.Parent.Scope.ProjectID = "foreign"
	_, e = changed.Manifests.DeleteTag(context.Background(), tag, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 0 {
		t.Fatal(e, calls)
	}
}

func TestRegistryAuthRedirectNeverReceivesCredentialsAtForeignOrigin(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "cr.selcloud.ru" {
			t.Fatal("foreign credential I/O")
		}
		if calls == 1 {
			v := response(401, "")
			v.Header.Set("WWW-Authenticate", `Bearer realm="https://cr.selcloud.ru/auth",service="registry"`)
			return v, nil
		}
		v := response(302, "")
		v.Header.Set("Location", "https://foreign.invalid/token")
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	tag, _ := c.Manifests.Tag(repo, "latest")
	_, e := c.Manifests.Get(context.Background(), tag, registrydata.ManifestReadOptions{}, types.CallOptions{})
	if e == nil || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestRegistryUnknownContentRecoverObservesOnlyExactAddress(t *testing.T) {
	deletes, heads := 0, 0
	root, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		if r.URL.Path != "/v2/reg/repo/manifests/latest" {
			t.Fatal(r.URL)
		}
		if r.Method == "DELETE" {
			deletes++
			return nil, errors.New("synthetic unknown")
		}
		if r.Method == "HEAD" {
			heads++
			return response(200, ""), nil
		}
		t.Fatal("unexpected request", r.Method)
		return nil, nil
	})
	repo, _ := c.Repository("reg", "repo")
	tag, _ := c.Manifests.Tag(repo, "latest")
	out, e := c.Manifests.DeleteTag(context.Background(), tag, types.CallOptions{})
	if e == nil || out.Receipt.Outcome != types.Unknown {
		t.Fatal(out, e)
	}
	handle, e := root.Resume(*out.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := handle.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || recovered.ProofStatus != "indeterminate" || recovered.Observation == nil || recovered.Observation.Classification != "succeeded" || deletes != 1 || heads != 1 {
		t.Fatal(recovered, e, deletes, heads)
	}
}

func TestRegistryUploadAndTagLinksRespectEndpointPathPrefix(t *testing.T) {
	const digest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if !strings.HasPrefix(r.URL.Path, "/registry/v2/") {
			t.Fatal("prefix lost/doubled", r.URL)
		}
		if r.URL.Path == "/registry/v2/" {
			return response(200, ""), nil
		}
		if r.URL.Path == "/registry/v2/reg/repo/tags/list" {
			if r.URL.Query().Get("last") == "a" {
				return response(200, `{"name":"reg/repo","tags":[]}`), nil
			}
			v := response(200, `{"name":"reg/repo","tags":["a"]}`)
			v.Header.Set("Link", `</registry/v2/reg/repo/tags/list?n=2&last=a>; rel="next"`)
			return v, nil
		}
		v := response(202, "")
		v.Header.Set("Location", "/registry/v2/reg/repo/blobs/uploads/u?state=opaque")
		if r.Method == "GET" {
			v.StatusCode = 204
			v.Header.Set("Range", "0-0")
		}
		if r.Method == "PUT" {
			v.StatusCode = 201
			v.Header.Set("Location", "/registry/v2/reg/repo/blobs/"+digest)
		}
		return v, nil
	}, func(cfg *sdk.Config) {
		scope := types.Scope{AccountID: "a", ProjectID: "p"}
		cfg.EndpointOverrides = map[types.EndpointKey]types.EndpointBinding{{Service: "registry_data", APIGeneration: "oci_v1", Scope: scope}: {ID: "proxy", URL: "https://proxy.invalid/registry", Scope: scope, APIGeneration: "oci_v1", Audience: "selectel"}}
	})
	repo, _ := c.Repository("reg", "repo")
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Uploads.Status(context.Background(), start.Data, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	d, _ := registrydata.ParseDigest(digest)
	if _, e = c.Uploads.Finalize(context.Background(), start.Data, d, nil, 0, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
	it, e := c.Tags.List(repo, pagination.Options{PageSize: 2, MaxPages: 3, MaxItems: 5})
	if e != nil {
		t.Fatal(e)
	}
	out, e := it.Collect(context.Background())
	if e != nil || !out.Complete || len(out.Items) != 1 {
		t.Fatal(out, e)
	}
}

func TestRegistryTagsAcceptRelativeNextLinkWithTokenRelation(t *testing.T) {
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		if r.URL.Query().Get("last") == "a" {
			return response(200, `{"name":"reg/repo","tags":[]}`), nil
		}
		v := response(200, `{"name":"reg/repo","tags":["a"]}`)
		v.Header.Set("Link", `<?n=2&last=a>; rel=next`)
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	it, e := c.Tags.List(repo, pagination.Options{PageSize: 2, MaxItems: 5, MaxPages: 3})
	if e != nil {
		t.Fatal(e)
	}
	out, e := it.Collect(context.Background())
	if e != nil || !out.Complete || len(out.Items) != 1 {
		t.Fatal(out, e)
	}
}

func TestRegistryVersion404IsEndpointUnavailable(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(200, ""), nil
		}
		return response(404, ""), nil
	})
	out, e := c.Version(context.Background(), types.CallOptions{})
	if types.ErrorCode(e) != types.EndpointUnavailable || out.Data.Supported || calls != 2 {
		t.Fatal(out, e, calls)
	}
}

func TestUnknownFinalizeRetainsRequestedBlobAddressWithoutCreationProof(t *testing.T) {
	const digest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	puts, heads := 0, 0
	root, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		if r.Method == "POST" {
			v := response(202, "")
			v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/u?state=protected")
			return v, nil
		}
		if r.Method == "PUT" {
			puts++
			return nil, errors.New("synthetic unknown")
		}
		if r.Method == "HEAD" && r.URL.Path == "/v2/reg/repo/blobs/"+digest {
			heads++
			return response(200, ""), nil
		}
		t.Fatal(r.Method, r.URL)
		return nil, nil
	})
	repo, _ := c.Repository("reg", "repo")
	start, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	d, _ := registrydata.ParseDigest(digest)
	out, e := c.Uploads.Finalize(context.Background(), start.Data, d, nil, 0, types.CallOptions{})
	if e == nil || out.Receipt.Outcome != types.Unknown || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.ResourceRefs[1].ID != digest {
		t.Fatal(out, e)
	}
	handle, e := root.Resume(*out.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	observation, e := handle.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || observation.ProofStatus != "indeterminate" || puts != 1 || heads != 1 {
		t.Fatal(observation, e, puts, heads)
	}
}

type blockingDiagnosticBody struct {
	started, closed  chan struct{}
	reading, closing sync.Once
	closeCalls       atomic.Int32
}

func (b *blockingDiagnosticBody) Read([]byte) (int, error) {
	b.reading.Do(func() { close(b.started) })
	<-b.closed
	return 0, io.ErrClosedPipe
}
func (b *blockingDiagnosticBody) Close() error {
	b.closeCalls.Add(1)
	b.closing.Do(func() { close(b.closed) })
	return nil
}

func TestOCIDiagnosticBodyCancellationClosesOnceAndRetainsKnownRejection(t *testing.T) {
	for _, stage := range []string{"call-503", "transfer-503", "cancel-403", "cancel-404"} {
		t.Run(stage, func(t *testing.T) {
			body := &blockingDiagnosticBody{started: make(chan struct{}), closed: make(chan struct{})}
			t.Cleanup(func() {
				select {
				case <-body.closed:
				default:
					body.Close()
				}
			})
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return response(200, ""), nil
				}
				status := 503
				if stage == "cancel-403" {
					status = 403
				}
				if stage == "cancel-404" {
					status = 404
				}
				v := response(status, "")
				v.Body = body
				return v, nil
			}, func(cfg *sdk.Config) {
				cfg.ReadMaxAttempts = 3
				cfg.CallTimeout = time.Second
				cfg.TransferTimeout = time.Second
				if stage == "call-503" {
					cfg.CallTimeout = 20 * time.Millisecond
				}
				if stage == "transfer-503" {
					cfg.TransferTimeout = 20 * time.Millisecond
				}
			})
			repo, _ := c.Repository("reg", "repo")
			tag, _ := c.Manifests.Tag(repo, "latest")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type completed struct {
				receipt *types.Receipt
				err     error
			}
			done := make(chan completed, 1)
			go func() {
				switch stage {
				case "call-503":
					v, e := c.Version(ctx, types.CallOptions{})
					done <- completed{v.Receipt, e}
				case "transfer-503":
					v, e := c.Manifests.Get(ctx, tag, registrydata.ManifestReadOptions{}, types.CallOptions{})
					done <- completed{v.Receipt, e}
				default:
					v, e := c.Manifests.DeleteTag(ctx, tag, types.CallOptions{})
					done <- completed{v.Receipt, e}
				}
			}()
			select {
			case <-body.started:
			case out := <-done:
				t.Fatalf("diagnostic not read: %v", out.err)
			case <-time.After(time.Second):
				t.Fatal("diagnostic not started")
			}
			if strings.HasPrefix(stage, "cancel-") {
				cancel()
			}
			select {
			case out := <-done:
				want := types.DeadlineExceeded
				if strings.HasPrefix(stage, "cancel-") {
					want = types.Canceled
					if out.receipt == nil || out.receipt.Outcome != types.Rejected {
						t.Fatalf("known rejection lost: %+v %v", out.receipt, out.err)
					}
					var detail *types.SDKError
					status := 403
					if stage == "cancel-404" {
						status = 404
					}
					if !errors.As(out.err, &detail) || detail.HTTPStatus != status || detail.Receipt == nil || detail.Receipt.Outcome != types.Rejected {
						t.Fatal("structured refusal lost", out.err)
					}
				}
				if types.ErrorCode(out.err) != want || calls != 2 || body.closeCalls.Load() != 1 {
					t.Fatal(out.err, calls, body.closeCalls.Load())
				}
			case <-time.After(time.Second):
				body.Close()
				out := <-done
				t.Fatalf("whole-response cancellation blocked; receipt=%+v err=%v", out.receipt, out.err)
			}
		})
	}
}

func TestMonolithicPOSTUploadVerifiesFullBlobWithoutImplicitFallback(t *testing.T) {
	const rawDigest = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	for _, status := range []int{201, 202} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				if r.Method != "POST" || r.URL.Path != "/v2/reg/repo/blobs/uploads/" || r.URL.Query().Get("digest") != rawDigest || r.ContentLength != 5 || r.Header.Get("Content-Type") != "application/octet-stream" {
					t.Fatal(r.Method, r.URL, r.Header, r.ContentLength)
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != "hello" {
					t.Fatal(string(body))
				}
				out := response(status, "")
				if status == 201 {
					out.Header.Set("Location", "/v2/reg/repo/blobs/"+rawDigest)
					out.Header.Set("Docker-Content-Digest", rawDigest)
				} else {
					out.Header.Set("Location", "/v2/reg/repo/blobs/uploads/session?_state=opaque")
				}
				return out, nil
			})
			repo, _ := c.Repository("reg", "repo")
			digest, _ := registrydata.ParseDigest(rawDigest)
			out, err := c.Uploads.Monolithic(context.Background(), repo, digest, strings.NewReader("hello"), 5, types.CallOptions{})
			if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || calls != 2 {
				t.Fatal(out, err, calls)
			}
			if status == 201 {
				if out.Data.Blob == nil || !out.Data.Blob.DigestVerified || out.Data.Blob.Address.ID != rawDigest || out.Data.Session != nil {
					t.Fatal(out)
				}
			} else {
				if out.Data.Session == nil || out.Data.Session.Ref.ID != "session" || out.Data.Blob != nil {
					t.Fatal(out)
				}
			}
		})
	}
}
func TestMonolithicUnknownAndAcceptedSessionResumeDoNotReplay(t *testing.T) {
	const rawDigest = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	for _, status := range []int{500, 202} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			posts := 0
			root, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				if r.Method == "POST" {
					posts++
					io.Copy(io.Discard, r.Body)
					v := response(status, "")
					if status == 202 {
						v.Header.Set("Location", "/v2/reg/repo/blobs/uploads/session?_state=protected")
					}
					return v, nil
				}
				if r.Method != "HEAD" || r.URL.Path != "/v2/reg/repo/blobs/"+rawDigest {
					t.Fatal(r.Method, r.URL)
				}
				return response(200, ""), nil
			})
			repo, _ := c.Repository("reg", "repo")
			digest, _ := registrydata.ParseDigest(rawDigest)
			out, err := c.Uploads.Monolithic(context.Background(), repo, digest, strings.NewReader("hello"), 5, types.CallOptions{})
			if status == 500 && types.ErrorCode(err) != types.UnknownOutcome {
				t.Fatal(err)
			}
			if status == 202 && err != nil {
				t.Fatal(err)
			}
			h, err := root.Resume(*out.Receipt)
			if err != nil {
				t.Fatal(err)
			}
			recovery, err := h.Recover(context.Background())
			if posts != 1 {
				t.Fatal(posts)
			}
			if status == 500 {
				if types.ErrorCode(err) != types.UnknownOutcome || recovery.ProofStatus != "indeterminate" || recovery.Observation == nil || recovery.Observation.Ref.ID != rawDigest {
					t.Fatal(recovery, err)
				}
			} else {
				if err != nil || recovery.ProofStatus != "conclusive" || h.Resource().ID != "session" {
					t.Fatal(recovery, err, h.Resource())
				}
			}
		})
	}
}
func TestMonolithicEmptyBlobHasExactZeroWireContentLength(t *testing.T) {
	const digestString = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		var wire strings.Builder
		if e := r.Write(&wire); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(wire.String(), "Content-Length: 0\r\n") || strings.Contains(wire.String(), "Transfer-Encoding: chunked") {
			t.Fatal(wire.String())
		}
		out := response(201, "")
		out.Header.Set("Location", "/v2/reg/repo/blobs/"+digestString)
		return out, nil
	})
	repo, _ := c.Repository("reg", "repo")
	digest, _ := registrydata.ParseDigest(digestString)
	out, err := c.Uploads.Monolithic(context.Background(), repo, digest, nil, 0, types.CallOptions{})
	if err != nil || out.Data.Blob == nil || !out.Data.Blob.DigestVerified {
		t.Fatal(out, err)
	}
}
func TestMonolithicRejectedRealmDoesNotReadBodyOrSendCredentials(t *testing.T) {
	reads, calls := 0, 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "cr.selcloud.ru" || r.URL.Path != "/v2/" || r.Header.Get("Authorization") != "" {
			t.Fatal("credentials or request escaped preflight")
		}
		out := response(401, "")
		out.Header.Set("Www-Authenticate", `Bearer realm="https://foreign.invalid/token",service="registry"`)
		return out, nil
	})
	repo, _ := c.Repository("reg", "repo")
	digest, _ := registrydata.ParseDigest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	reader := &countReader{reads: &reads, Reader: strings.NewReader("hello")}
	out, err := c.Uploads.Monolithic(context.Background(), repo, digest, reader, 5, types.CallOptions{})
	if err == nil || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || reads != 0 || calls != 1 {
		t.Fatal(out, err, reads, calls)
	}
}
func TestMonolithicAcceptedMalformedResponseOrWrongDigestRetainsRequestedAddress(t *testing.T) {
	// Valid expected digest is independent of the transmitted bad payload.
	digest, _ := registrydata.ParseDigest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	for _, variant := range []string{"digest", "foreign-location", "wrong-repository", "short"} {
		t.Run(variant, func(t *testing.T) {
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				io.Copy(io.Discard, r.Body)
				out := response(201, "")
				location := "/v2/reg/repo/blobs/" + digest.String()
				if variant == "foreign-location" {
					location = "https://foreign.invalid" + location
				}
				if variant == "wrong-repository" {
					location = "/v2/reg/other/blobs/" + digest.String()
				}
				out.Header.Set("Location", location)
				return out, nil
			})
			repo, _ := c.Repository("reg", "repo")
			payload := "wrong"
			if variant == "short" {
				payload = "hi"
			}
			out, err := c.Uploads.Monolithic(context.Background(), repo, digest, strings.NewReader(payload), 5, types.CallOptions{})
			if err == nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Receipt.ResourceRefs[1].ID != digest.String() || (out.Data.Blob != nil && out.Data.Blob.DigestVerified) {
				t.Fatal(out, err)
			}
		})
	}
}
