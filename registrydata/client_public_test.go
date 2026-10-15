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
	"reflect"
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
func TestMountExplicitFromAuthorizesOnlyExactRepositoriesAndDoesNotFallback(t *testing.T) {
	const rawDigest = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	for _, status := range []int{201, 202} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path == "/v2/" {
					out := response(401, "")
					out.Header.Set("Www-Authenticate", `Bearer realm="https://cr.selcloud.ru/token",service="registry",scope="repository:foreign/admin:delete"`)
					return out, nil
				}
				if r.URL.Path == "/token" {
					if !reflect.DeepEqual(r.URL.Query()["scope"], []string{"repository:reg/repo:pull,push", "repository:source/images:pull"}) {
						t.Fatal(r.URL.Query())
					}
					return response(200, `{"token":"synthetic-bearer"}`), nil
				}
				if r.Method != "POST" || r.URL.Path != "/v2/reg/repo/blobs/uploads/" || r.URL.Query().Get("mount") != rawDigest || r.URL.Query().Get("from") != "source/images" || r.Header.Get("Authorization") != "Bearer synthetic-bearer" || r.ContentLength != 0 {
					t.Fatal(r.Method, r.URL, r.ContentLength)
				}
				out := response(status, "")
				if status == 201 {
					out.Header.Set("Location", "/v2/reg/repo/blobs/"+rawDigest)
					out.Header.Set("Docker-Content-Digest", rawDigest)
				} else {
					out.Header.Set("Location", "/v2/reg/repo/blobs/uploads/session?_state=saved")
				}
				return out, nil
			})
			dest, _ := c.Repository("reg", "repo")
			source, _ := c.Repository("source", "images")
			digest, _ := registrydata.ParseDigest(rawDigest)
			out, err := c.Uploads.Mount(context.Background(), dest, digest, registrydata.MountOptions{From: &source}, types.CallOptions{})
			if err != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || calls != 3 {
				t.Fatal(out, err, calls)
			}
			if status == 201 {
				if out.Data.Blob == nil || out.Data.Blob.DigestVerified || out.Data.Blob.Address.ID != rawDigest || out.Data.Session != nil {
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
func TestMountInvalidSourceIsNotSentBeforeCredentials(t *testing.T) {
	calls, lookups := 0, 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected HTTP") }, func(cfg *sdk.Config) {
		cfg.Credentials = credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
			lookups++
			return credentials.Material{}, errors.New("unexpected credentials")
		})
	})
	dest, _ := c.Repository("reg", "repo")
	source, _ := c.Repository("source", "images")
	digest, _ := registrydata.ParseDigest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	for _, change := range []func(*types.ResourceRef){func(r *types.ResourceRef) { r.Parent = nil }, func(r *types.ResourceRef) { r.EndpointURL = "https://foreign.invalid" }, func(r *types.ResourceRef) { r.Scope.ProjectID = "other" }} {
		bad := source
		change(&bad)
		out, err := c.Uploads.Mount(context.Background(), dest, digest, registrydata.MountOptions{From: &bad}, types.CallOptions{})
		if err == nil || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 || lookups != 0 {
			t.Fatal(out, err, calls, lookups)
		}
	}
}
func TestMountReceiptResumePreservesSourceCanonicalAddressAndUnknownNoReplay(t *testing.T) {
	requested, _ := registrydata.ParseDigest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	canonical, _ := registrydata.ParseDigest("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	for _, named := range []bool{false, true} {
		for _, status := range []int{201, 202, 500} {
			t.Run(fmt.Sprintf("named=%v/status=%d", named, status), func(t *testing.T) {
				posts := 0
				root, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
					if r.URL.Path == "/v2/" {
						return response(200, ""), nil
					}
					if r.Method == "HEAD" {
						if r.URL.Path != "/v2/reg/repo/blobs/"+requested.String() {
							t.Fatal(r.URL)
						}
						return response(200, ""), nil
					}
					posts++
					if r.Method != "POST" {
						t.Fatal(r.Method)
					}
					if r.URL.Query().Has("from") != named {
						t.Fatal(r.URL)
					}
					out := response(status, "")
					if status == 201 {
						out.Header.Set("Docker-Content-Digest", canonical.String())
						out.Header.Set("Location", "/v2/reg/repo/blobs/"+canonical.String())
					}
					if status == 202 {
						out.Header.Set("Location", "/v2/reg/repo/blobs/uploads/session?_state=secret")
					}
					return out, nil
				})
				dest, _ := c.Repository("reg", "repo")
				source, _ := c.Repository("source", "images")
				opts := registrydata.MountOptions{}
				if named {
					opts.From = &source
				}
				out, err := c.Uploads.Mount(context.Background(), dest, requested, opts, types.CallOptions{})
				if status == 500 {
					if types.ErrorCode(err) != types.UnknownOutcome {
						t.Fatal(err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if status == 201 && (out.Data.Blob == nil || out.Data.Blob.DigestVerified || out.Data.Blob.RequestedDigest != requested || out.Data.Blob.CanonicalDigest == nil || *out.Data.Blob.CanonicalDigest != canonical || out.Data.Blob.Address.ID != canonical.String()) {
					t.Fatal(out)
				}
				h, err := root.Resume(*out.Receipt)
				if err != nil {
					t.Fatal(err)
				}
				recovered, err := h.Recover(context.Background())
				if status == 500 {
					if types.ErrorCode(err) != types.UnknownOutcome || recovered.ProofStatus != "indeterminate" || recovered.Observation == nil || recovered.Observation.Ref.ID != requested.String() {
						t.Fatal(recovered, err)
					}
				} else if err != nil || recovered.ProofStatus != "conclusive" {
					t.Fatal(recovered, err)
				}
				if posts != 1 {
					t.Fatal(posts)
				}
			})
		}
	}
}
func TestMountRejectsForeignLocationAndPreservesProtectedResult(t *testing.T) {
	digest, _ := registrydata.ParseDigest("sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	for _, variant := range []string{"protected", "foreign", "wrong-repository", "rejected"} {
		t.Run(variant, func(t *testing.T) {
			posts := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				posts++
				out := response(201, "")
				location := "/v2/reg/repo/blobs/" + digest.String() + "?signature=secret-mount"
				if variant == "foreign" {
					location = "https://foreign.invalid" + location
				}
				if variant == "wrong-repository" {
					location = "/v2/reg/other/blobs/" + digest.String()
				}
				if variant == "rejected" {
					out = response(403, `{"errors":[{"code":"DENIED","message":"secret-mount"}]}`)
				}
				out.Header.Set("Location", location)
				return out, nil
			})
			dest, _ := c.Repository("reg", "repo")
			out, err := c.Uploads.Mount(context.Background(), dest, digest, registrydata.MountOptions{}, types.CallOptions{})
			if posts != 1 || out.Receipt == nil {
				t.Fatal(posts, out, err)
			}
			if variant == "rejected" {
				if out.Receipt.Outcome != types.Rejected || types.ErrorCode(err) != types.PermissionDenied {
					t.Fatal(out, err)
				}
				return
			}
			if out.Receipt.Outcome != types.Accepted {
				t.Fatal(out, err)
			}
			if variant != "protected" {
				if err == nil || out.Data.Blob != nil {
					t.Fatal(out, err)
				}
				return
			}
			if err != nil || out.Data.Blob == nil {
				t.Fatal(out, err)
			}
			for _, value := range []any{out, out.Data, *out.Data.Blob, out.Data.Blob, &out.Data.Blob.Location} {
				for _, format := range []string{"%v", "%+v", "%#v"} {
					text := fmt.Sprintf(format, value)
					if strings.Contains(text, "secret-mount") || strings.Contains(text, "115 101 99 114") || strings.Contains(text, "0x73, 0x65, 0x63") {
						t.Fatal("protected mount Location exposed")
					}
				}
				raw, _ := json.Marshal(value)
				if strings.Contains(string(raw), "secret-mount") {
					t.Fatal("protected JSON exposed")
				}
			}
		})
	}
}
func TestReferrersTypedDescriptorsAndArtifactTypeWire(t *testing.T) {
	const subject = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	const referred = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		if r.Method != "GET" || r.URL.Path != "/v2/reg/repo/referrers/"+subject || r.URL.Query().Get("artifactType") != "application/vnd.example.sbom" || r.URL.Query().Has("n") {
			t.Fatal(r.Method, r.URL)
		}
		out := response(200, `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","annotations":{"secret":"protected-index"},"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","size":9007199254740993,"digest":"`+referred+`","artifactType":"application/vnd.example.sbom","annotations":{"secret":"protected-descriptor"},"urls":["https://external.invalid/private"],"data":"cHJvdGVjdGVk","platform":{"architecture":"amd64","os":"linux","os.version":"1","os.features":["feature"],"variant":"v1"}}]}`)
		out.Header.Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		out.Header.Set("OCI-Filters-Applied", "artifactType")
		return out, nil
	})
	repo, _ := c.Repository("reg", "repo")
	d, _ := registrydata.ParseDigest(subject)
	ref, _ := c.Manifests.Digest(repo, d)
	artifact := "application/vnd.example.sbom"
	it, err := c.Referrers.List(ref, registrydata.ReferrerOptions{ArtifactType: &artifact}, pagination.Options{MaxItems: 5, MaxPages: 1})
	if err != nil {
		t.Fatal(err)
	}
	out, err := it.Collect(context.Background())
	if err != nil || !out.Complete || len(out.Items) != 1 || calls != 2 {
		t.Fatal(out, err, calls)
	}
	v := out.Items[0]
	if v.Address.ID != referred || v.Size.String() != "9007199254740993" || v.ArtifactType == nil || *v.ArtifactType != artifact || v.Platform == nil || *v.Platform.Architecture != "amd64" || v.Data == nil || v.Annotations == nil || len(v.URLs) != 1 || len(out.ProviderPages) != 1 || out.ProviderPages[0].OCIReferrersIndex == nil {
		t.Fatal(out)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "protected-index") || strings.Contains(string(raw), "protected-descriptor") || strings.Contains(string(raw), "external.invalid") || strings.Contains(string(raw), "cHJvdGVjdGVk") {
		t.Fatal("protected referrer metadata exposed")
	}
	for _, value := range []any{out, v, v.Annotations, *v.Annotations, v.URLs, v.Data, out.ProviderPages[0], out.ProviderPages[0].OCIReferrersIndex, *out.ProviderPages[0].OCIReferrersIndex} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			text := fmt.Sprintf(format, value)
			if strings.Contains(text, "protected-index") || strings.Contains(text, "protected-descriptor") || strings.Contains(text, "external.invalid") || strings.Contains(text, "cHJvdGVjdGVk") || strings.Contains(text, "112 114 111 116") || strings.Contains(text, "0x70, 0x72, 0x6f") {
				t.Fatal("protected formatting exposed metadata")
			}
		}
	}
}

func TestReferrersLinkCursorPreservesOpaqueQueryAndFilterSnapshot(t *testing.T) {
	const subject = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	const first = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	const nextQuery = "artifactType=application%2Fvnd.example.sbom&cursor=opaque%2fsecret+token&n=1"
	pages := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		pages++
		digest := first
		if pages == 2 {
			if r.URL.RawQuery != nextQuery {
				t.Fatal(r.URL.RawQuery)
			}
			digest = subject
		}
		out := response(200, `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","size":1,"digest":"`+digest+`","artifactType":"application/vnd.example.sbom"}]}`)
		out.Header.Set("Content-Type", string(registrydata.OCIIndex))
		if pages == 1 {
			out.Header.Set("Link", `<https://foreign.invalid/prev>; rel="prev", </v2/reg/repo/referrers/`+subject+`?`+nextQuery+`>; rel="next"; title="quoted, title"`)
		}
		return out, nil
	})
	repo, _ := c.Repository("reg", "repo")
	d, _ := registrydata.ParseDigest(subject)
	ref, _ := c.Manifests.Digest(repo, d)
	artifact := "application/vnd.example.sbom"
	it, err := c.Referrers.List(ref, registrydata.ReferrerOptions{ArtifactType: &artifact}, pagination.Options{MaxItems: 1, MaxPages: 3})
	if err != nil {
		t.Fatal(err)
	}
	artifact = "application/changed"
	out, err := it.Collect(context.Background())
	if types.ErrorCode(err) != types.Incomplete || out.NextCursor == nil || out.NextCursor.SensitiveWire == nil || len(out.Items) != 1 || pages != 1 {
		t.Fatal(out, err, pages)
	}
	raw, _ := json.Marshal(out.NextCursor)
	if strings.Contains(string(raw), "opaque") || out.NextCursor.WireCursor != "" {
		t.Fatal("opaque cursor leaked")
	}
	artifact = "application/vnd.example.sbom"
	resumed, err := c.Referrers.List(ref, registrydata.ReferrerOptions{ArtifactType: &artifact}, pagination.Options{MaxItems: 5, MaxPages: 3, Cursor: out.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	remaining, err := resumed.Collect(context.Background())
	if err != nil || !remaining.Complete || len(remaining.Items) != 1 || remaining.Items[0].Address.ID != subject || pages != 2 {
		t.Fatal(remaining, err, pages)
	}
}
func TestReferrersMalformedDescriptorRetainsValidatedPrefix(t *testing.T) {
	const digest = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	for _, suffix := range []string{`,"annotations":null`, `,"urls":[null]`, `,"annotations":{"key":null}`, `,"platform":{"architecture":"amd64","os":"linux","os.features":[null]}`, `,"data":"not-base64!"`} {
		t.Run(suffix, func(t *testing.T) {
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				v := response(200, `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","size":1,"digest":"`+digest+`"},{"mediaType":"application/vnd.oci.image.manifest.v1+json","size":1,"digest":"`+digest+`"`+suffix+`}]}`)
				v.Header.Set("Content-Type", string(registrydata.OCIIndex))
				return v, nil
			})
			repo, _ := c.Repository("reg", "repo")
			d, _ := registrydata.ParseDigest(digest)
			subject, _ := c.Manifests.Digest(repo, d)
			it, e := c.Referrers.List(subject, registrydata.ReferrerOptions{}, pagination.Options{MaxItems: 5, MaxPages: 1})
			if e != nil {
				t.Fatal(e)
			}
			out, e := it.Collect(context.Background())
			if types.ErrorCode(e) != types.ProtocolError || out.Complete || len(out.Items) != 1 {
				t.Fatal(out, e)
			}
		})
	}
}
func TestReferrersUnappliedFilterIsLocalAnd404DoesNotFallback(t *testing.T) {
	const subject = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	for _, status := range []int{200, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				calls++
				if !strings.Contains(r.URL.Path, "/referrers/") {
					t.Fatal("implicit fallback", r.URL)
				}
				out := response(status, `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","size":1,"digest":"`+subject+`","artifactType":"other/type"}]}`)
				out.Header.Set("Content-Type", string(registrydata.OCIIndex))
				return out, nil
			})
			repo, _ := c.Repository("reg", "repo")
			digest, _ := registrydata.ParseDigest(subject)
			ref, _ := c.Manifests.Digest(repo, digest)
			artifact := "application/vnd.example.sbom"
			it, err := c.Referrers.List(ref, registrydata.ReferrerOptions{ArtifactType: &artifact}, pagination.Options{MaxItems: 1, MaxPages: 1})
			if err != nil {
				t.Fatal(err)
			}
			out, err := it.Collect(context.Background())
			if calls != 1 || len(out.Items) != 0 {
				t.Fatal(out, err, calls)
			}
			if status == 404 {
				if types.ErrorCode(err) != types.NotFound || out.Complete {
					t.Fatal(out, err)
				}
			} else if err != nil || !out.Complete || len(out.ProviderPages) != 1 {
				t.Fatal(out, err)
			}
		})
	}
}
func TestReferrersRejectUnsafeOrAmbiguousLinkWithoutFollowing(t *testing.T) {
	const subject = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	for _, test := range []struct {
		link string
		code types.Code
	}{
		{`<https://foreign.invalid/v2/reg/repo/referrers/` + subject + `?cursor=x>; rel="next"`, types.ScopeMismatch},
		{`</v2/reg/other/referrers/` + subject + `?cursor=x>; rel="next"`, types.ScopeMismatch},
		{`<?scope=repository%3Aforeign%3Adelete>; rel="next"`, types.ScopeMismatch},
		{`<?cursor=x>; rel="next", <?cursor=y>; rel="next"`, types.ProtocolError},
		{"<?cursor=x>; rel=\"next\"\r\nX-Foreign: y", types.ProtocolError},
	} {
		t.Run(test.link, func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				calls++
				if calls != 1 {
					t.Fatal("unsafe continuation followed")
				}
				v := response(200, `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","size":1,"digest":"`+subject+`"}]}`)
				v.Header.Set("Content-Type", string(registrydata.OCIIndex))
				v.Header.Set("Link", test.link)
				return v, nil
			})
			repo, _ := c.Repository("reg", "repo")
			d, _ := registrydata.ParseDigest(subject)
			ref, _ := c.Manifests.Digest(repo, d)
			it, e := c.Referrers.List(ref, registrydata.ReferrerOptions{}, pagination.Options{MaxItems: 5, MaxPages: 5})
			if e != nil {
				t.Fatal(e)
			}
			out, e := it.Collect(context.Background())
			if types.ErrorCode(e) != test.code || out.Complete || len(out.Items) != 1 || calls != 1 {
				t.Fatal(out, e, calls)
			}
		})
	}
}
func TestReferrersCursorScopeAndEndpointIsolationBeforeHTTP(t *testing.T) {
	const digest = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		calls++
		v := response(200, `{"schemaVersion":2,"manifests":[]}`)
		v.Header.Set("Content-Type", string(registrydata.OCIIndex))
		v.Header.Set("Link", `<?cursor=next>; rel="next"`)
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	d, _ := registrydata.ParseDigest(digest)
	subject, _ := c.Manifests.Digest(repo, d)
	artifact := "application/example"
	it, e := c.Referrers.List(subject, registrydata.ReferrerOptions{ArtifactType: &artifact}, pagination.Options{MaxItems: 1, MaxPages: 1})
	if e != nil {
		t.Fatal(e)
	}
	out, e := it.Collect(context.Background())
	if types.ErrorCode(e) != types.Incomplete || out.NextCursor == nil || len(out.ProviderPages) != 1 {
		t.Fatal(out, e)
	}
	changed := "application/other"
	if _, e = c.Referrers.List(subject, registrydata.ReferrerOptions{ArtifactType: &changed}, pagination.Options{Cursor: out.NextCursor}); types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatal(e, calls)
	}
	other := subject
	other.EndpointURL = "https://other.invalid"
	parent := *subject.Parent
	parent.EndpointURL = other.EndpointURL
	namespace := *parent.Parent
	namespace.EndpointURL = other.EndpointURL
	parent.Parent = &namespace
	other.Parent = &parent
	if _, e = c.Referrers.List(other, registrydata.ReferrerOptions{ArtifactType: &artifact}, pagination.Options{Cursor: out.NextCursor}); types.ErrorCode(e) != types.ScopeMismatch || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestReferrersMaxItemsReplaysOnlyUnreadDescriptors(t *testing.T) {
	const first = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	const second = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		v := response(200, `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","size":1,"digest":"`+first+`"},{"mediaType":"application/vnd.oci.image.manifest.v1+json","size":1,"digest":"`+second+`"}]}`)
		v.Header.Set("Content-Type", string(registrydata.OCIIndex))
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	d, _ := registrydata.ParseDigest(first)
	subject, _ := c.Manifests.Digest(repo, d)
	it, e := c.Referrers.List(subject, registrydata.ReferrerOptions{}, pagination.Options{MaxItems: 1, MaxPages: 1})
	if e != nil {
		t.Fatal(e)
	}
	out, e := it.Collect(context.Background())
	if types.ErrorCode(e) != types.Incomplete || len(out.Items) != 1 || out.NextCursor == nil || out.NextCursor.Skip != 1 {
		t.Fatal(out, e)
	}
	resumed, e := c.Referrers.List(subject, registrydata.ReferrerOptions{}, pagination.Options{MaxItems: 5, MaxPages: 1, Cursor: out.NextCursor})
	if e != nil {
		t.Fatal(e)
	}
	remaining, e := resumed.Collect(context.Background())
	if e != nil || !remaining.Complete || len(remaining.Items) != 1 || remaining.Items[0].Address.ID != second {
		t.Fatal(remaining, e)
	}
}
func TestReferrersMultipleAppliedFilterHeadersArePreserved(t *testing.T) {
	const subject = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		out := response(200, `{"schemaVersion":2,"manifests":[]}`)
		out.Header.Set("Content-Type", string(registrydata.OCIIndex))
		out.Header["Oci-Filters-Applied"] = []string{"unknown-filter", "artifactType"}
		return out, nil
	})
	repo, _ := c.Repository("reg", "repo")
	d, _ := registrydata.ParseDigest(subject)
	ref, _ := c.Manifests.Digest(repo, d)
	artifact := "application/example"
	it, e := c.Referrers.List(ref, registrydata.ReferrerOptions{ArtifactType: &artifact}, pagination.Options{MaxItems: 1, MaxPages: 1})
	if e != nil {
		t.Fatal(e)
	}
	out, e := it.Collect(context.Background())
	if e != nil || !out.Complete || len(out.ProviderPages) != 1 || out.ProviderPages[0].OCIFiltersApplied == nil || string(out.ProviderPages[0].OCIFiltersApplied.Reveal()) != "unknown-filter,artifactType" {
		t.Fatal(out, e)
	}
}
func TestSubjectPushReportsAcknowledgementWithoutHiddenFallback(t *testing.T) {
	const subject = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	const other = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	payload := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","subject":{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + subject + `","size":1},"config":{"mediaType":"application/example","digest":"` + subject + `","size":0},"layers":[]}`
	for _, ack := range []string{"", subject, other, "bad"} {
		t.Run(ack, func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				calls++
				if r.Method != "PUT" || r.URL.Path != "/v2/reg/repo/manifests/artifact" {
					t.Fatal("implicit fallback", r.Method, r.URL)
				}
				out := response(201, "")
				out.Header.Set("Location", "/v2/reg/repo/manifests/artifact")
				if ack != "" {
					out.Header.Set("OCI-Subject", ack)
				}
				return out, nil
			})
			repo, _ := c.Repository("reg", "repo")
			ref, _ := c.Manifests.Tag(repo, "artifact")
			var protected types.ProtectedJSON
			json.Unmarshal([]byte(payload), &protected)
			out, err := c.Manifests.Put(context.Background(), ref, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: protected}, types.CallOptions{})
			if calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Data.RequestedSubject == nil || out.Data.RequestedSubject.String() != subject {
				t.Fatal(out, err, calls)
			}
			if ack == "" {
				if err != nil || !out.Data.NeedsTagFallback || out.Data.AcknowledgedSubject != nil {
					t.Fatal(out, err)
				}
			} else if ack == subject {
				if err != nil || out.Data.NeedsTagFallback || out.Data.AcknowledgedSubject == nil || out.Data.AcknowledgedSubject.String() != subject {
					t.Fatal(out, err)
				}
			} else if err == nil || out.Data.NeedsTagFallback {
				t.Fatal(out, err)
			}
		})
	}
}
func TestFallbackTagUsesExactSubjectWithoutHTTP(t *testing.T) {
	calls, lookups := 0, 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected HTTP") }, func(cfg *sdk.Config) {
		cfg.Credentials = credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
			lookups++
			return credentials.Material{}, errors.New("unexpected lookup")
		})
	})
	repo, _ := c.Repository("reg", "repo")
	for _, test := range []struct{ raw, want string }{
		{"sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", "sha256-2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		{"sha384:" + strings.Repeat("a", 96), "sha384-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"sha512:" + strings.Repeat("b", 128), "sha512-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	} {
		d, _ := registrydata.ParseDigest(test.raw)
		subject, _ := c.Manifests.Digest(repo, d)
		tag, e := c.Referrers.FallbackTag(subject)
		if e != nil || tag.Kind != "manifest_tag" || tag.ID != test.want || tag.Parent.ID != "repo" || calls != 0 || lookups != 0 {
			t.Fatal(tag, e, calls, lookups)
		}
	}
}
func TestPrepareTagIndexProducesOnlyProviderFieldsAndRequiresExplicitPut(t *testing.T) {
	const subjectDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	const payload = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","subject":{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + subjectDigest + `","size":0},"config":{"mediaType":"application/example.config","digest":"` + subjectDigest + `","size":0},"layers":[],"annotations":{"secret":"caller-secret"}}`
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		calls++
		if r.Method != "PUT" || !strings.HasSuffix(r.URL.Path, "/manifests/sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855") {
			t.Fatal(r.Method, r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		var index struct {
			Manifests []map[string]json.RawMessage `json:"manifests"`
		}
		if json.Unmarshal(body, &index) != nil || len(index.Manifests) != 1 {
			t.Fatal("invalid prepared index")
		}
		descriptor := index.Manifests[0]
		if len(descriptor) != 5 || string(descriptor["artifactType"]) != `"application/example.config"` || string(descriptor["annotations"]) != `{"secret":"caller-secret"}` {
			t.Fatal("descriptor source fields lost")
		}
		for _, key := range []string{"address", "ref", "scope", "endpoint_id", "endpoint_url", "parent"} {
			if _, ok := descriptor[key]; ok {
				t.Fatal("SDK metadata sent")
			}
		}
		v := response(201, "")
		v.Header.Set("Location", r.URL.Path)
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	sd, _ := registrydata.ParseDigest(subjectDigest)
	subject, _ := c.Manifests.Digest(repo, sd)
	// Independent SHA256/length fixture for the literal payload above.
	contentDigest, _ := registrydata.ParseDigest("sha256:b13dbe0808e9986b174417bd97f614e052ad67dae3c2e133d196ce5d141ae8c4")
	content, _ := c.Manifests.Digest(repo, contentDigest)
	var protected types.ProtectedJSON
	json.Unmarshal([]byte(payload), &protected)
	prepared, err := c.Referrers.PrepareTagIndex(subject, nil, content, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: protected}, registrydata.TagIndexAdd, registrydata.TagIndexOptions{MaxDescriptors: 5})
	if err != nil || !prepared.Changed || prepared.DescriptorCount != 1 || prepared.BaseDigest != nil || calls != 0 {
		t.Fatal(prepared, err, calls)
	}
	encoded, _ := json.Marshal(prepared)
	if strings.Contains(string(encoded), "caller-secret") {
		t.Fatal("prepared payload exposed")
	}
	result, err := c.Manifests.Put(context.Background(), prepared.Tag, prepared.Write, types.CallOptions{})
	if err != nil || result.Receipt == nil || result.Receipt.Outcome != types.Accepted || calls != 1 {
		t.Fatal(result, err, calls)
	}
}
func TestPrepareTagIndexDuplicateAndRemovePreserveCurrentMetadataWithoutIO(t *testing.T) {
	const subjectDigest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	const contentDigest = "sha256:b13dbe0808e9986b174417bd97f614e052ad67dae3c2e133d196ce5d141ae8c4"
	const payload = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","subject":{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + subjectDigest + `","size":0},"config":{"mediaType":"application/example.config","digest":"` + subjectDigest + `","size":0},"layers":[],"annotations":{"secret":"caller-secret"}}`
	const index = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","annotations":{"keep":"metadata-secret"},"x-extension":{"keep":true},"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + contentDigest + `","size":434,"artifactType":"application/example.config","annotations":{"secret":"caller-secret"}}]}`
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855") {
			t.Fatal(r.Method, r.URL)
		}
		v := response(200, index)
		v.Header.Set("Content-Type", string(registrydata.OCIIndex))
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	sd, _ := registrydata.ParseDigest(subjectDigest)
	subject, _ := c.Manifests.Digest(repo, sd)
	cd, _ := registrydata.ParseDigest(contentDigest)
	content, _ := c.Manifests.Digest(repo, cd)
	tag, _ := c.Referrers.FallbackTag(subject)
	current, err := c.Manifests.Get(context.Background(), tag, registrydata.ManifestReadOptions{Accept: []registrydata.ManifestMediaType{registrydata.OCIIndex}}, types.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var body types.ProtectedJSON
	json.Unmarshal([]byte(payload), &body)
	duplicate, err := c.Referrers.PrepareTagIndex(subject, &current.Data, content, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: body}, registrydata.TagIndexAdd, registrydata.TagIndexOptions{MaxDescriptors: 5})
	if err != nil || duplicate.Changed || duplicate.DescriptorCount != 1 || duplicate.BaseDigest == nil || *duplicate.BaseDigest != current.Data.Digest || string(duplicate.Write.Payload.Reveal()) != index || calls != 2 {
		t.Fatal(duplicate, err, calls)
	}
	removed, err := c.Referrers.PrepareTagIndex(subject, &current.Data, content, registrydata.ManifestWriteOptions{}, registrydata.TagIndexRemove, registrydata.TagIndexOptions{MaxDescriptors: 5})
	if err != nil || !removed.Changed || removed.DescriptorCount != 0 || calls != 2 {
		t.Fatal(removed, err, calls)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(removed.Write.Payload.Reveal(), &fields) != nil || string(fields["manifests"]) != "[]" || string(fields["annotations"]) != `{"keep":"metadata-secret"}` || string(fields["x-extension"]) != `{"keep":true}` {
		t.Fatal("existing metadata lost")
	}
}
func TestPrepareTagIndexRejectsEmptyConfigArtifactFallback(t *testing.T) {
	const sd = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		v := response(200, `{"schemaVersion":2,"subject":{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"`+sd+`","size":0},"config":{"mediaType":""},"layers":[]}`)
		v.Header.Set("Content-Type", string(registrydata.OCIManifest))
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	tag, _ := c.Manifests.Tag(repo, "bad-config")
	document, e := c.Manifests.Get(context.Background(), tag, registrydata.ManifestReadOptions{}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	content, _ := c.Manifests.Digest(repo, document.Data.Digest)
	digest, _ := registrydata.ParseDigest(sd)
	subject, _ := c.Manifests.Digest(repo, digest)
	if _, e = c.Referrers.PrepareTagIndex(subject, nil, content, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: document.Data.Payload}, registrydata.TagIndexAdd, registrydata.TagIndexOptions{}); types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal(e)
	}
}
func TestPrepareTagIndexBoundsAndSnapshotScopeGuards(t *testing.T) {
	const first = "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	const second = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		v := response(200, `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"`+first+`","size":1},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"`+second+`","size":0}]}`)
		v.Header.Set("Content-Type", string(registrydata.OCIIndex))
		return v, nil
	})
	repo, _ := c.Repository("reg", "repo")
	digest, _ := registrydata.ParseDigest(first)
	subject, _ := c.Manifests.Digest(repo, digest)
	content, _ := c.Manifests.Digest(repo, digest)
	tag, _ := c.Referrers.FallbackTag(subject)
	current, e := c.Manifests.Get(context.Background(), tag, registrydata.ManifestReadOptions{}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Referrers.PrepareTagIndex(subject, &current.Data, content, registrydata.ManifestWriteOptions{}, registrydata.TagIndexRemove, registrydata.TagIndexOptions{MaxDescriptors: 1}); types.ErrorCode(e) != types.ResponseLimit || calls != 2 {
		t.Fatal(e, calls)
	}
	wrong := current.Data
	wrong.Address.ID = "different-tag"
	if _, e = c.Referrers.PrepareTagIndex(subject, &wrong, content, registrydata.ManifestWriteOptions{}, registrydata.TagIndexRemove, registrydata.TagIndexOptions{MaxDescriptors: 5}); types.ErrorCode(e) != types.ScopeMismatch || calls != 2 {
		t.Fatal(e, calls)
	}
	changed := current.Data
	json.Unmarshal([]byte(`{"schemaVersion":2,"manifests":[]}`), &changed.Payload)
	if _, e = c.Referrers.PrepareTagIndex(subject, &changed, content, registrydata.ManifestWriteOptions{}, registrydata.TagIndexRemove, registrydata.TagIndexOptions{MaxDescriptors: 5}); types.ErrorCode(e) != types.InvalidArgument || calls != 2 {
		t.Fatal(e, calls)
	}
	otherRepo, _ := c.Repository("reg", "other")
	other, _ := c.Manifests.Digest(otherRepo, digest)
	if _, e = c.Referrers.PrepareTagIndex(subject, &current.Data, other, registrydata.ManifestWriteOptions{}, registrydata.TagIndexRemove, registrydata.TagIndexOptions{MaxDescriptors: 5}); types.ErrorCode(e) != types.ScopeMismatch || calls != 2 {
		t.Fatal(e, calls)
	}
}
func TestSubjectUnsupportedAlgorithmIsUnavailableBeforeIO(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected HTTP") })
	repo, _ := c.Repository("reg", "repo")
	tag, _ := c.Manifests.Tag(repo, "artifact")
	var payload types.ProtectedJSON
	json.Unmarshal([]byte(`{"schemaVersion":2,"subject":{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha3-256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size":1}}`), &payload)
	out, e := c.Manifests.Put(context.Background(), tag, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: payload}, types.CallOptions{})
	if types.ErrorCode(e) != types.CapabilityUnavailable || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || calls != 0 {
		t.Fatal(out, e, calls)
	}
}
func TestPrepareTagIndexByteBoundRequiresNoCredentialsOrHTTP(t *testing.T) {
	calls, lookups := 0, 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected HTTP") }, func(cfg *sdk.Config) {
		cfg.MaxJSONBytes = 256
		cfg.Credentials = credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
			lookups++
			return credentials.Material{}, errors.New("unexpected credentials")
		})
	})
	repo, _ := c.Repository("reg", "repo")
	sd, _ := registrydata.ParseDigest("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	subject, _ := c.Manifests.Digest(repo, sd)
	cd, _ := registrydata.ParseDigest("sha256:b13dbe0808e9986b174417bd97f614e052ad67dae3c2e133d196ce5d141ae8c4")
	content, _ := c.Manifests.Digest(repo, cd)
	var payload types.ProtectedJSON
	json.Unmarshal([]byte(`{"padding":"`+strings.Repeat("x", 300)+`"}`), &payload)
	out, e := c.Referrers.PrepareTagIndex(subject, nil, content, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: payload}, registrydata.TagIndexAdd, registrydata.TagIndexOptions{MaxDescriptors: 1})
	if types.ErrorCode(e) != types.ResponseLimit || calls != 0 || lookups != 0 || out.Changed {
		t.Fatal(out, e, calls, lookups)
	}
}

func TestManifestConditionalPutAndETagMetadata(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		t.Run(method, func(t *testing.T) {
			puts := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				if r.Method == "PUT" {
					puts++
					if r.Header.Get("If-Match") != `"opaque,tag"` || r.Header.Get("If-None-Match") != `W/"old", "other"` {
						t.Fatal(r.Header)
					}
					return response(412, `{"errors":[{"code":"DENIED"}]}`), nil
				}
				out := response(200, `{}`)
				out.Header.Set("Content-Type", string(registrydata.OCIManifest))
				out.Header.Set("ETag", `"opaque,tag"`)
				return out, nil
			})
			repo, _ := c.Repository("reg", "repo")
			ref, _ := c.Manifests.Tag(repo, "tag")
			var tag *types.ProtectedValue
			if method == "GET" {
				v, e := c.Manifests.Get(context.Background(), ref, registrydata.ManifestReadOptions{}, types.CallOptions{})
				if e != nil {
					t.Fatal(e)
				}
				tag = v.Data.ETag
			} else {
				v, e := c.Manifests.Head(context.Background(), ref, registrydata.ManifestReadOptions{}, types.CallOptions{})
				if e != nil {
					t.Fatal(e)
				}
				tag = v.Data.ETag
			}
			if tag == nil || string(tag.Reveal()) != `"opaque,tag"` || strings.Contains(fmt.Sprintf("%+v", tag), "opaque") {
				t.Fatal(tag)
			}
			var payload types.ProtectedJSON
			json.Unmarshal([]byte(`{}`), &payload)
			out, e := c.Manifests.Put(context.Background(), ref, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: payload, IfMatch: &registrydata.EntityTagCondition{Tags: []types.ProtectedValue{*tag}}, IfNoneMatch: &registrydata.EntityTagCondition{Tags: []types.ProtectedValue{types.Protect([]byte(`W/"old"`)), types.Protect([]byte(`"other"`))}}}, types.CallOptions{})
			if e == nil || puts != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Rejected {
				t.Fatal(out, e, puts)
			}
		})
	}
}
func TestManifestConditionalWildcardAndValidationBeforeIO(t *testing.T) {
	for _, tc := range []struct {
		name      string
		condition registrydata.EntityTagCondition
		valid     bool
	}{
		{"wildcard", registrydata.EntityTagCondition{Any: true}, true},
		{"weak", registrydata.EntityTagCondition{Tags: []types.ProtectedValue{types.Protect([]byte(`W/""`))}}, true},
		{"empty", registrydata.EntityTagCondition{}, false},
		{"mixed", registrydata.EntityTagCondition{Any: true, Tags: []types.ProtectedValue{types.Protect([]byte(`"a"`))}}, false},
		{"unquoted", registrydata.EntityTagCondition{Tags: []types.ProtectedValue{types.Protect([]byte("secret"))}}, false},
		{"injection", registrydata.EntityTagCondition{Tags: []types.ProtectedValue{types.Protect([]byte("\"a\"\r\nAuthorization: evil"))}}, false},
		{"list-in-tag", registrydata.EntityTagCondition{Tags: []types.ProtectedValue{types.Protect([]byte(`"a", "b"`))}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, lookups := 0, 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				if tc.name == "wildcard" && r.Header.Get("If-None-Match") != "*" {
					t.Fatal(r.Header)
				}
				out := response(201, "")
				out.Header.Set("Location", "/v2/reg/repo/manifests/tag")
				return out, nil
			}, func(cfg *sdk.Config) {
				cfg.Credentials = credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
					lookups++
					return credentials.Material{Identity: "test", Revision: "1", Binding: types.Scope{AccountID: "a", ProjectID: "p"}, Audience: "selectel", Scheme: credentials.Basic, Username: types.Protect([]byte("u")), Password: types.Protect([]byte("p"))}, nil
				})
			})
			repo, _ := c.Repository("reg", "repo")
			ref, _ := c.Manifests.Tag(repo, "tag")
			var payload types.ProtectedJSON
			json.Unmarshal([]byte(`{}`), &payload)
			out, e := c.Manifests.Put(context.Background(), ref, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: payload, IfNoneMatch: &tc.condition}, types.CallOptions{})
			if tc.valid {
				if e != nil || out.Receipt == nil || out.Receipt.Outcome != types.Accepted {
					t.Fatal(out, e)
				}
			} else if e == nil || calls != 0 || lookups != 0 || out.Receipt == nil || out.Receipt.Outcome != types.NotSent {
				t.Fatal(out, e, calls, lookups)
			}
		})
	}
}

func TestManifestETagMalformedIsProtocolError(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		for _, raw := range []string{"", `unquoted`, `"a" , "b"`, `w/"a"`, "\"a\x7f\""} {
			t.Run(method+raw, func(t *testing.T) {
				_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
					out := response(200, `{}`)
					if r.URL.Path != "/v2/" {
						out.Header.Set("Content-Type", string(registrydata.OCIManifest))
						out.Header.Set("ETag", raw)
					}
					return out, nil
				})
				repo, _ := c.Repository("reg", "repo")
				ref, _ := c.Manifests.Tag(repo, "tag")
				var err error
				if method == "GET" {
					_, err = c.Manifests.Get(context.Background(), ref, registrydata.ManifestReadOptions{}, types.CallOptions{})
				} else {
					_, err = c.Manifests.Head(context.Background(), ref, registrydata.ManifestReadOptions{}, types.CallOptions{})
				}
				if types.ErrorCode(err) != types.ProtocolError {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestConditionalManifestUnknownDoesNotReplay(t *testing.T) {
	puts := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		puts++
		return nil, errors.New("lost response")
	})
	repo, _ := c.Repository("reg", "repo")
	ref, _ := c.Manifests.Tag(repo, "tag")
	var payload types.ProtectedJSON
	json.Unmarshal([]byte(`{}`), &payload)
	out, e := c.Manifests.Put(context.Background(), ref, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: payload, IfMatch: &registrydata.EntityTagCondition{Any: true}}, types.CallOptions{})
	if e == nil || puts != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Unknown {
		t.Fatal(out, e, puts)
	}
}

func TestManifestConditionByteBoundBeforeIO(t *testing.T) {
	calls, lookups := 0, 0
	_, c := dataClient(t, func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected HTTP") }, func(cfg *sdk.Config) {
		cfg.MaxJSONBytes = 256
		cfg.Credentials = credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
			lookups++
			return credentials.Material{}, errors.New("unexpected lookup")
		})
	})
	repo, _ := c.Repository("reg", "repo")
	ref, _ := c.Manifests.Tag(repo, "tag")
	var payload types.ProtectedJSON
	json.Unmarshal([]byte(`{}`), &payload)
	tag := types.Protect([]byte(`"` + strings.Repeat("x", 130) + `"`))
	out, e := c.Manifests.Put(context.Background(), ref, registrydata.ManifestWriteOptions{MediaType: registrydata.OCIManifest, Payload: payload, IfMatch: &registrydata.EntityTagCondition{Tags: []types.ProtectedValue{tag}}, IfNoneMatch: &registrydata.EntityTagCondition{Tags: []types.ProtectedValue{tag}}}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || calls != 0 || lookups != 0 || out.Receipt == nil || out.Receipt.Outcome != types.NotSent {
		t.Fatal(out, e, calls, lookups)
	}
}

func TestUploadMinimumChunkHintPreservesExactMetadataAndSession(t *testing.T) {
	for _, variant := range []string{"start", "monolithic", "mount"} {
		for _, hint := range []string{"", "0", "00012", "184467440737095516160000", "-1", "1.5", "bad"} {
			t.Run(variant+hint, func(t *testing.T) {
				calls := 0
				_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
					if r.URL.Path == "/v2/" {
						return response(200, ""), nil
					}
					calls++
					if r.Method == "POST" {
						out := response(202, "")
						out.Header.Set("Location", "/v2/reg/repo/blobs/uploads/session?_state=protected")
						if hint != "" {
							out.Header.Set("OCI-Chunk-Min-Length", hint)
						}
						return out, nil
					}
					out := response(204, "")
					out.Header.Set("Location", "/v2/reg/repo/blobs/uploads/session?_state=next")
					out.Header.Set("Range", "0-0")
					return out, nil
				})
				repo, _ := c.Repository("reg", "repo")
				digest, _ := registrydata.ParseDigest("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
				var session registrydata.UploadSession
				var receipt *types.Receipt
				var err error
				switch variant {
				case "start":
					out, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
					session, receipt, err = out.Data, out.Receipt, e
				case "monolithic":
					out, e := c.Uploads.Monolithic(context.Background(), repo, digest, strings.NewReader(""), 0, types.CallOptions{})
					receipt, err = out.Receipt, e
					if out.Data.Session != nil {
						session = *out.Data.Session
					}
				case "mount":
					out, e := c.Uploads.Mount(context.Background(), repo, digest, registrydata.MountOptions{}, types.CallOptions{})
					receipt, err = out.Receipt, e
					if out.Data.Session != nil {
						session = *out.Data.Session
					}
				}
				if receipt == nil || receipt.Outcome != types.Accepted || session.Ref.ID != "session" || calls != 1 {
					t.Fatal(session, receipt, err, calls)
				}
				if hint == "-1" || hint == "1.5" || hint == "bad" {
					if types.ErrorCode(err) != types.ProtocolError || len(receipt.ResourceRefs) == 0 {
						t.Fatal(err, receipt)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if hint == "" {
					if session.MinChunkLength != nil {
						t.Fatal(session)
					}
					return
				}
				canonical := strings.TrimLeft(hint, "0")
				if canonical == "" {
					canonical = "0"
				}
				want, _ := types.ParseInteger(canonical)
				if session.MinChunkLength == nil || session.MinChunkLength.String() != want.String() {
					t.Fatal(session)
				}
				next, e := c.Uploads.Status(context.Background(), session, types.CallOptions{})
				if e != nil || next.Data.MinChunkLength == nil || next.Data.MinChunkLength.String() != want.String() || calls != 2 {
					t.Fatal(next, e, calls)
				}
			})
		}
	}
}

func TestUploadHintHeaderFailureKeepsAcceptedIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		values []string
		code   types.Code
	}{{"empty", []string{""}, types.ProtocolError}, {"duplicate", []string{"12", "12"}, types.ProtocolError}, {"bounded", []string{strings.Repeat("1", 257)}, types.ResponseLimit}} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				calls++
				out := response(202, "")
				out.Header.Set("Location", "/v2/reg/repo/blobs/uploads/session")
				out.Header["Oci-Chunk-Min-Length"] = test.values
				return out, nil
			}, func(cfg *sdk.Config) { cfg.MaxJSONBytes = 256 })
			repo, _ := c.Repository("reg", "repo")
			out, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
			if types.ErrorCode(e) != test.code || calls != 1 || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || out.Data.Ref.ID != "session" || len(out.Receipt.ResourceRefs) == 0 || out.Receipt.ResourceRefs[len(out.Receipt.ResourceRefs)-1].ID != "session" {
				t.Fatal(out, e, calls)
			}
		})
	}
}
func TestUploadHintIsAdvisoryAndAppendCarriesSnapshot(t *testing.T) {
	calls := 0
	_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v2/" {
			return response(200, ""), nil
		}
		calls++
		out := response(202, "")
		out.Header.Set("Location", "/v2/reg/repo/blobs/uploads/session")
		if r.Method == "POST" {
			out.Header.Set("OCI-Chunk-Min-Length", "1000")
		} else {
			body, _ := io.ReadAll(r.Body)
			if r.Method != "PATCH" || string(body) != "x" {
				t.Fatal(r.Method, string(body))
			}
			out.Header.Set("Range", "0-0")
		}
		return out, nil
	})
	repo, _ := c.Repository("reg", "repo")
	initial, e := c.Uploads.Start(context.Background(), repo, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	next, e := c.Uploads.Append(context.Background(), initial.Data, strings.NewReader("x"), 1, types.CallOptions{})
	if e != nil || calls != 2 || next.Data.MinChunkLength == nil || next.Data.MinChunkLength.String() != "1000" || next.Data.Offset != 1 || next.Receipt == nil || next.Receipt.Outcome != types.Accepted {
		t.Fatal(next, e, calls)
	}
	original, _ := types.ParseInteger("7")
	*initial.Data.MinChunkLength = original
	if next.Data.MinChunkLength.String() != "1000" {
		t.Fatal("hint alias")
	}
}

func TestTagLinksAcceptBoundedExtendedSyntax(t *testing.T) {
	for _, links := range [][]string{
		{`<?last=a>; rel=next; title="optional n"`},
		{`<?n=2&last=a>; rel="prev next"; title="title, with comma"; type="application/json"`},
		{`<https://foreign.invalid/ignored>; rel=prev`, `<?n=2&last=a>; rel=NEXT; title*=utf-8'en'next%20page`},
		{`<?n=2&last=a>; rel=next; example="escaped\\value", <https://foreign.invalid/ignored>; rel="https://example.org/custom"`},
	} {
		t.Run(strings.Join(links, "|"), func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				calls++
				if r.URL.Path != "/v2/reg/repo/tags/list" || r.URL.Query().Get("n") != "2" {
					t.Fatal(r.URL)
				}
				if calls == 2 {
					if r.URL.Query().Get("last") != "a" {
						t.Fatal(r.URL)
					}
					return response(200, `{"name":"reg/repo","tags":[]}`), nil
				}
				out := response(200, `{"name":"reg/repo","tags":["a"]}`)
				out.Header["Link"] = links
				return out, nil
			})
			repo, _ := c.Repository("reg", "repo")
			it, e := c.Tags.List(repo, pagination.Options{PageSize: 2, MaxItems: 5, MaxPages: 3})
			if e != nil {
				t.Fatal(e)
			}
			out, e := it.Collect(context.Background())
			if e != nil || !out.Complete || len(out.Items) != 1 || calls != 2 {
				t.Fatal(out, e, calls)
			}
		})
	}
}

func TestTagExtendedLinksRetainPrefixOnUnsafeNext(t *testing.T) {
	for _, test := range []struct {
		name  string
		links []string
		code  types.Code
	}{
		{"duplicate-next", []string{`<?n=2&last=a>; rel=next, <?n=2&last=a>; rel=next`}, types.ProtocolError},
		{"anchor", []string{`<?n=2&last=a>; rel=next; anchor=""`}, types.ProtocolError},
		{"foreign", []string{`<https://foreign.invalid/v2/reg/repo/tags/list?n=2&last=a>; rel="prev next"`}, types.ScopeMismatch},
		{"wrongpath", []string{`</v2/other/repo/tags/list?n=2&last=a>; rel=next`}, types.ScopeMismatch},
		{"changedsize", []string{`<?n=3&last=a>; rel=next`}, types.ScopeMismatch},
		{"duplicatedquery", []string{`<?n=2&last=a&last=a>; rel=next`}, types.ScopeMismatch},
		{"unknownquery", []string{`<?n=2&last=a&token=secret>; rel=next`}, types.ScopeMismatch},
		{"malformedquote", []string{`<?n=2&last=a>; rel=next; title="unfinished`}, types.ProtocolError},
		{"bounded", []string{`<?n=2&last=a>; rel=next; title="` + strings.Repeat("x", 257) + `"`}, types.ResponseLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			_, c := dataClient(t, func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v2/" {
					return response(200, ""), nil
				}
				calls++
				out := response(200, `{"name":"reg/repo","tags":["a"]}`)
				out.Header["Link"] = test.links
				return out, nil
			}, func(cfg *sdk.Config) { cfg.MaxJSONBytes = 256 })
			repo, _ := c.Repository("reg", "repo")
			it, e := c.Tags.List(repo, pagination.Options{PageSize: 2, MaxItems: 5, MaxPages: 3})
			if e != nil {
				t.Fatal(e)
			}
			out, e := it.Collect(context.Background())
			if types.ErrorCode(e) != test.code || out.Complete || len(out.Items) != 1 || out.NextCursor == nil || calls != 1 {
				t.Fatal(out, e, calls)
			}
		})
	}
}
