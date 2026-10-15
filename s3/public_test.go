package s3_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/s3"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, b string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(b))}
}
func clients(t *testing.T, f transport) (*sdk.Client, *s3.Client) {
	t.Helper()
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "spb", Pool: "ru-1"}
	c, e := sdk.New(sdk.Config{AccountID: "a", PollInterval: time.Millisecond, Credentials: credentials.SourceFunc(func(_ context.Context, r types.AuthRequest) (credentials.Material, error) {
		if r.Mode != types.S3SigV4 || r.Scope != scope || r.Service != "s3" {
			t.Fatal(r)
		}
		return credentials.Material{Identity: "synthetic", Revision: "1", Binding: scope, Audience: "selectel", Scheme: credentials.S3, AccessKey: types.Protect([]byte("synthetic-access")), SecretKey: types.Protect([]byte("synthetic-secret"))}, nil
	}), Transport: transport(func(r *http.Request) (*http.Response, error) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=synthetic-access/") || !strings.Contains(r.Header.Get("Authorization"), "/ru-1/s3/aws4_request") {
			t.Fatal("signature")
		}
		return f(r)
	})})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := c.Project("p")
	r, _ := p.Location("spb", "ru-1")
	return c, r.S3()
}
func TestCreateBucketUsesExplicitS3ScopeAndPathStyle(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" || r.URL.String() != "https://s3.ru-1.storage.selcloud.ru/test-bucket" {
			t.Fatal(r.Method, r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		if len(b) != 0 {
			t.Fatal(string(b))
		}
		return response(200, ""), nil
	})
	ref, e := c.Bucket("test-bucket")
	if e != nil {
		t.Fatal(e)
	}
	v, e := c.Buckets.Create(context.Background(), ref, s3.CreateBucketOptions{}, types.CallOptions{})
	if e != nil || v.Receipt.Outcome != types.Accepted || len(v.Receipt.ResourceRefs) != 1 || v.Receipt.ResourceRefs[0].Kind != "bucket_name" {
		t.Fatal(e)
	}
}
func TestUnseekableUploadIsSizeBoundSignedAndNeverReplayed(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "PUT" || r.URL.EscapedPath() != "/test-bucket/folder/key%20space" || r.ContentLength != 4 || r.Header.Get("X-Amz-Content-Sha256") != "UNSIGNED-PAYLOAD" {
			t.Fatal(r.Method, r.URL, r.ContentLength)
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != "data" {
			t.Fatal(string(b))
		}
		return nil, io.ErrUnexpectedEOF
	})
	bucket, _ := c.Bucket("test-bucket")
	ref, e := c.Object(bucket, "folder/key space")
	if e != nil {
		t.Fatal(e)
	}
	reader := &onlyReader{strings.NewReader("data-trailing")}
	v, e := c.Objects.Upload(context.Background(), ref, reader, 4, s3.UploadOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || v.Receipt.Outcome != types.Unknown || calls != 1 || v.Data.Bytes != 4 || v.Data.Complete {
		t.Fatal(e, calls, v.Data.Bytes)
	}
	remaining, _ := io.ReadAll(reader)
	if string(remaining) != "-trailing" {
		t.Fatal(string(remaining))
	}
}

type onlyReader struct{ io.Reader }

func TestDownloadCopiesOnlyBoundedPrefixAndPreservesDeadline(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Path != "/test-bucket/key" || r.Header.Get("Range") != "bytes=0-9" {
			t.Fatal(r.URL)
		}
		v := response(206, "abcdefghij")
		v.Header.Set("ETag", `"etag"`)
		v.Header.Set("X-Amz-Meta-Private", "private-metadata")
		return v, nil
	})
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "key")
	var out strings.Builder
	rangeHeader := "bytes=0-9"
	v, e := c.Objects.Download(context.Background(), r, &out, s3.DownloadOptions{MaxBytes: 4, Range: &rangeHeader}, types.CallOptions{})
	if types.ErrorCode(e) != types.ResponseLimit || v.Data.Complete || v.Data.Bytes != 4 || out.String() != "abcd" {
		t.Fatal(e, out.String(), v.Data.Bytes)
	}
	if string(v.Data.Object.Metadata["private"].Reveal()) != "private-metadata" {
		t.Fatal("missing metadata")
	}
}
func TestBucketLifecyclePreservesLocationWithoutRegionAdoption(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if r.Method != "HEAD" {
				t.Fatal(r.Method)
			}
			v := response(200, "")
			v.Header.Set("X-Amz-Bucket-Region", "foreign-pool")
			return v, nil
		case 2:
			if r.Method != "GET" || !r.URL.Query().Has("location") {
				t.Fatal(r.Method, r.URL)
			}
			return response(200, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">ru-1</LocationConstraint>`), nil
		case 3:
			if r.Method != "DELETE" {
				t.Fatal(r.Method)
			}
			return response(204, ""), nil
		}
		t.Fatal("unexpected")
		return nil, nil
	})
	r, _ := c.Bucket("test-bucket")
	v, e := c.Buckets.Head(context.Background(), r, types.CallOptions{})
	if e != nil || v.Data.Region == nil || *v.Data.Region != "foreign-pool" || v.Data.Ref.Scope.Pool != "ru-1" {
		t.Fatal(e)
	}
	l, e := c.Buckets.Location(context.Background(), r, types.CallOptions{})
	if e != nil || l.Data.Location == nil || *l.Data.Location != "ru-1" {
		t.Fatal(e)
	}
	if _, e = c.Buckets.Delete(context.Background(), r, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
}
func TestS3XMLCodeMappingRejectsWithoutProviderTextDisclosure(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		return response(403, `<Error><Code>SignatureDoesNotMatch</Code><Message>private-provider-text</Message></Error>`), nil
	})
	r, _ := c.Bucket("test-bucket")
	v, e := c.Buckets.Delete(context.Background(), r, types.CallOptions{})
	var sdkerr *types.SDKError
	if !errors.As(e, &sdkerr) || sdkerr.Code != types.Unauthenticated || sdkerr.ProviderCode != "SignatureDoesNotMatch" || v.Receipt.Outcome != types.Rejected || strings.Contains(fmt.Sprintf("%+v", e), "private-provider-text") {
		t.Fatal(e)
	}
}
func TestObjectHeadDeleteAndCopyKeepExactDestination(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			if r.Method != "HEAD" || r.URL.Path != "/test-bucket/source" {
				t.Fatal(r.Method, r.URL)
			}
			v := response(200, "")
			v.Header.Set("ETag", `"source-tag"`)
			return v, nil
		case 2:
			if r.Method != "PUT" || r.URL.Path != "/test-bucket/target" || r.Header.Get("X-Amz-Copy-Source") != "/test-bucket/source?versionId=opaque%2Bversion" {
				t.Fatal(r.Method, r.URL, r.Header.Get("X-Amz-Copy-Source"))
			}
			return response(200, `<CopyObjectResult><ETag>"target-tag"</ETag><LastModified>2026-10-15T12:00:00Z</LastModified></CopyObjectResult>`), nil
		case 3:
			if r.Method != "DELETE" || r.URL.Query().Get("versionId") != "opaque+version" {
				t.Fatal(r.Method, r.URL)
			}
			return response(204, ""), nil
		}
		t.Fatal("unexpected")
		return nil, nil
	})
	b, _ := c.Bucket("test-bucket")
	source, _ := c.Object(b, "source")
	target, _ := c.Object(b, "target")
	h, e := c.Objects.Head(context.Background(), source, s3.HeadObjectOptions{}, types.CallOptions{})
	if e != nil || h.Data.ETag == nil || *h.Data.ETag != `"source-tag"` {
		t.Fatal(e)
	}
	version := types.Protect([]byte("opaque+version"))
	v, e := c.Objects.Copy(context.Background(), source, target, s3.CopyOptions{SourceVersionID: &version}, types.CallOptions{})
	if e != nil || v.Receipt.Outcome != types.Accepted || v.Data.Ref.ID != "target" || *v.Data.ETag != `"target-tag"` {
		t.Fatal(e)
	}
	if _, e = c.Objects.Delete(context.Background(), target, s3.DeleteObjectOptions{VersionID: &version}, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
}
func TestDeleteObjectsMixedResultsAndQuietFalse(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || !r.URL.Query().Has("delete") || r.Header.Get("Content-MD5") == "" {
			t.Fatal(r.Method, r.URL)
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), "<Quiet>false</Quiet>") || !strings.Contains(string(b), "<Key>one</Key>") {
			t.Fatal(string(b))
		}
		return response(200, `<DeleteResult><Deleted><Key>one</Key></Deleted><Error><Key>two</Key><Code>AccessDenied</Code><Message>private-provider-text</Message></Error></DeleteResult>`), nil
	})
	bucket, _ := c.Bucket("test-bucket")
	f := false
	v, e := c.Objects.DeleteMany(context.Background(), bucket, s3.DeleteManyRequest{Objects: []s3.DeleteEntry{{Key: "one"}, {Key: "two"}}, Quiet: &f}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProviderError || v.Receipt.Outcome != types.Accepted || len(v.Data.Deleted) != 1 || len(v.Data.Errors) != 1 || v.Data.Errors[0].Code != "AccessDenied" {
		t.Fatal(e)
	}
	if strings.Contains(fmt.Sprintf("%+v", v), "private-provider-text") {
		t.Fatal("error disclosure")
	}
}
func TestBucketAndObjectListingsUseSourceCursorsAndPreserveOpaqueTokens(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path == "/" {
			return response(200, `<ListAllMyBucketsResult><Owner><ID>owner</ID></Owner><Buckets><Bucket><Name>test-bucket</Name><CreationDate>2026-10-15T12:00:00Z</CreationDate></Bucket></Buckets></ListAllMyBucketsResult>`), nil
		}
		if r.URL.Query().Get("list-type") == "2" {
			if r.URL.Query().Get("continuation-token") == "" {
				return response(200, `<ListBucketResult><Name>test-bucket</Name><IsTruncated>true</IsTruncated><KeyCount>1</KeyCount><Contents><Key>folder%2Fkey%20space</Key><Size>9007199254740993</Size><ETag>"etag"</ETag></Contents><NextContinuationToken>private+cursor</NextContinuationToken><EncodingType>url</EncodingType></ListBucketResult>`), nil
			}
			if r.URL.Query().Get("continuation-token") != "private+cursor" {
				t.Fatal(r.URL)
			}
			return response(200, `<ListBucketResult><Name>test-bucket</Name><IsTruncated>false</IsTruncated><KeyCount>0</KeyCount></ListBucketResult>`), nil
		}
		if r.URL.Query().Get("marker") == "" {
			return response(200, `<ListBucketResult><Name>test-bucket</Name><IsTruncated>true</IsTruncated><Contents><Key>one</Key><Size>1</Size></Contents></ListBucketResult>`), nil
		}
		if r.URL.Query().Get("marker") != "one" {
			t.Fatal(r.URL)
		}
		return response(200, `<ListBucketResult><Name>test-bucket</Name><IsTruncated>false</IsTruncated><CommonPrefixes><Prefix>folder/</Prefix></CommonPrefixes></ListBucketResult>`), nil
	})
	buckets, e := c.Buckets.Collect(context.Background(), s3.BucketListOptions{}, pagination.Options{})
	if e != nil || !buckets.Complete || len(buckets.Items) != 1 || buckets.Items[0].Ref == nil {
		t.Fatal(e)
	}
	b, _ := c.Bucket("test-bucket")
	v, e := c.Objects.Collect(context.Background(), b, s3.ObjectListOptions{EncodingTypeURL: true}, pagination.Options{})
	if e != nil || !v.Complete || len(v.Items) != 1 || v.Items[0].Object.Ref.ID != "folder/key space" || v.Items[0].Object.Size.String() != "9007199254740993" {
		t.Fatal(e)
	}
	old, e := c.Objects.CollectV1(context.Background(), b, s3.ObjectListOptions{}, pagination.Options{})
	if e != nil || !old.Complete || len(old.Items) != 2 || old.Items[1].CommonPrefix == nil || calls != 5 {
		t.Fatal(e, calls)
	}
}
func TestPresignIsCredentialScopedProtectedAndHasNoHTTP(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) { t.Fatal("presign HTTP"); return nil, nil })
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "folder/key space")
	v, e := c.Objects.PresignGet(context.Background(), r, s3.PresignOptions{Expires: 15 * time.Minute}, types.CallOptions{})
	if e != nil || v.Receipt != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(string(v.Data.URL.Reveal()))
	if e != nil || u.Host != "s3.ru-1.storage.selcloud.ru" || u.Query().Get("X-Amz-Expires") != "900" || !strings.Contains(u.Query().Get("X-Amz-Credential"), "/ru-1/s3/aws4_request") || u.Query().Get("X-Amz-Signature") == "" {
		t.Fatal("signed URL")
	}
	formatted := fmt.Sprintf("%+v %#v", v, &v)
	encoded, _ := json.Marshal(v)
	if strings.Contains(formatted, "X-Amz-Signature") || strings.Contains(string(encoded), "synthetic-access") {
		t.Fatal("disclosure")
	}
	if _, e = c.Objects.PresignGet(context.Background(), r, s3.PresignOptions{Expires: 15*time.Minute + time.Second}, types.CallOptions{}); types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal(e)
	}
	if _, e = c.Objects.PresignPut(context.Background(), r, s3.PresignOptions{Expires: time.Minute}, s3.UploadOptions{}, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
}
func TestUnknownUploadRecoveryObservesExactAddressWithoutCreationProof(t *testing.T) {
	calls := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method == "PUT" {
			io.Copy(io.Discard, r.Body)
			return nil, io.ErrUnexpectedEOF
		}
		if r.Method != "HEAD" || r.URL.Path != "/test-bucket/key" {
			t.Fatal("mutation replay", r.Method, r.URL)
		}
		return response(200, ""), nil
	})
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "key")
	v, e := c.Objects.Upload(context.Background(), r, strings.NewReader("data"), 4, s3.UploadOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome {
		t.Fatal(e)
	}
	h, e := root.Resume(*v.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := h.Recover(context.Background())
	if types.ErrorCode(e) != types.UnknownOutcome || recovered.ProofStatus != "indeterminate" || recovered.Observation.Ref.ID != "key" || calls != 2 {
		t.Fatal(e)
	}
	if _, e = h.Wait(context.Background(), "object.contents_match"); types.ErrorCode(e) != types.CapabilityUnavailable {
		t.Fatal(e)
	}
	bad, e := c.Objects.Upload(context.Background(), r, strings.NewReader(""), -1, s3.UploadOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || bad.Receipt.Outcome != types.NotSent {
		t.Fatal(e)
	}
	h, e = root.Resume(*bad.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	before := calls
	if got, e := h.Recover(context.Background()); e != nil || got.ProofStatus != "conclusive" || calls != before {
		t.Fatal(e)
	}
}
func TestStreamingDeadlinePreservesEmittedPrefixWithoutRetry(t *testing.T) {
	calls := 0
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		calls++
		v := response(200, "")
		v.Body = &blockingBody{closed: make(chan struct{})}
		return v, nil
	})
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "key")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var out strings.Builder
	v, e := c.Objects.Download(ctx, r, &out, s3.DownloadOptions{MaxBytes: 100}, types.CallOptions{})
	if types.ErrorCode(e) != types.DeadlineExceeded || v.Data.Bytes != 4 || v.Data.Complete || out.String() != "data" || calls != 1 {
		t.Fatal(e, v.Data.Bytes, calls)
	}
}

type blockingBody struct {
	closed chan struct{}
	once   sync.Once
	first  bool
}

func (b *blockingBody) Read(p []byte) (int, error) {
	if !b.first {
		b.first = true
		return copy(p, "data"), nil
	}
	<-b.closed
	return 0, io.ErrClosedPipe
}
func (b *blockingBody) Close() error { b.once.Do(func() { close(b.closed) }); return nil }
func TestRedirectAndWrongExactEndpointNeverReachForeignTarget(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "s3.ru-1.storage.selcloud.ru" {
			t.Fatal("foreign credentials")
		}
		v := response(307, "")
		v.Header.Set("Location", "https://foreign.test/key")
		return v, nil
	})
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "key")
	if _, e := c.Objects.Head(context.Background(), r, s3.HeadObjectOptions{}, types.CallOptions{}); types.ErrorCode(e) != types.ProtocolError || calls != 1 {
		t.Fatal(e, calls)
	}
	r.EndpointURL = "https://old.test"
	r.Parent.EndpointURL = "https://old.test"
	if v, e := c.Objects.Delete(context.Background(), r, s3.DeleteObjectOptions{}, types.CallOptions{}); types.ErrorCode(e) != types.EndpointUnavailable || v.Receipt.Outcome != types.NotSent || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestHeadRetainsExactLengthAndProtectedContentHeaders(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		v := response(200, "")
		v.Header.Set("Content-Length", "9007199254740993")
		v.Header.Set("Cache-Control", "private-header")
		v.Header.Set("Content-Range", "bytes 0-9/100")
		v.Header.Set("X-Amz-Delete-Marker", "false")
		return v, nil
	})
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "key")
	v, e := c.Objects.Head(context.Background(), r, s3.HeadObjectOptions{}, types.CallOptions{})
	if e != nil || v.Data.ContentLength.String() != "9007199254740993" || string(v.Data.CacheControl.Reveal()) != "private-header" || v.Data.DeleteMarker == nil || *v.Data.DeleteMarker || v.Data.ContentRange == nil || *v.Data.ContentRange != "bytes 0-9/100" {
		t.Fatal(e)
	}
}
func TestUnknownVersionDeleteDoesNotObserveDifferentCurrentVersion(t *testing.T) {
	root, c := clients(t, func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF })
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "key")
	version := types.Protect([]byte("private-version"))
	v, e := c.Objects.Delete(context.Background(), r, s3.DeleteObjectOptions{VersionID: &version}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome {
		t.Fatal(e)
	}
	if _, e = root.Resume(*v.Receipt); types.ErrorCode(e) != types.RecoveryUnavailable {
		t.Fatal(e)
	}
}
func TestSigV4PreservesEscapedS3KeyWithoutDoubleEncoding(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if !validSignature(r) {
			t.Fatal("signature does not match S3 canonical request")
		}
		if r.URL.EscapedPath() != "/test-bucket/folder/../key%20space" {
			t.Fatal(r.URL.EscapedPath())
		}
		return response(200, ""), nil
	})
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "folder/../key space")
	if _, e := c.Objects.Head(context.Background(), r, s3.HeadObjectOptions{}, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
}
func validSignature(r *http.Request) bool {
	parts := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 "), ", ")
	if len(parts) != 3 {
		return false
	}
	credential := strings.TrimPrefix(parts[0], "Credential=")
	scope := strings.Split(credential, "/")
	if len(scope) != 5 {
		return false
	}
	signed := strings.TrimPrefix(parts[1], "SignedHeaders=")
	var canonical strings.Builder
	for _, k := range strings.Split(signed, ";") {
		value := r.Header.Get(k)
		if k == "host" {
			value = r.URL.Host
		}
		if k == "content-length" {
			value = strconv.FormatInt(r.ContentLength, 10)
		}
		canonical.WriteString(k + ":" + strings.Join(strings.Fields(value), " ") + "\n")
	}
	query := strings.ReplaceAll(r.URL.Query().Encode(), "+", "%20")
	request := r.Method + "\n" + r.URL.EscapedPath() + "\n" + query + "\n" + canonical.String() + "\n" + signed + "\n" + r.Header.Get("X-Amz-Content-Sha256")
	sum := sha256.Sum256([]byte(request))
	credentialScope := strings.Join(scope[1:], "/")
	signing := "AWS4-HMAC-SHA256\n" + r.Header.Get("X-Amz-Date") + "\n" + credentialScope + "\n" + hex.EncodeToString(sum[:])
	mac := func(key []byte, message string) []byte {
		h := hmac.New(sha256.New, key)
		h.Write([]byte(message))
		return h.Sum(nil)
	}
	key := mac([]byte("AWS4synthetic-secret"), scope[1])
	key = mac(key, scope[2])
	key = mac(key, scope[3])
	key = mac(key, scope[4])
	expected := hex.EncodeToString(mac(key, signing))
	return hmac.Equal([]byte(expected), []byte(strings.TrimPrefix(parts[2], "Signature=")))
}
func TestExplicitVirtualHostedRefsPinBucketHostAndResumeAcrossViews(t *testing.T) {
	calls := 0
	root, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "test-bucket.s3.ru-1.storage.selcloud.ru" || !validSignature(r) {
			t.Fatal("virtual host or signature")
		}
		if calls == 1 {
			if r.Method != "PUT" || r.URL.Path != "/key" {
				t.Fatal(r.Method, r.URL)
			}
			io.Copy(io.Discard, r.Body)
			return nil, io.ErrUnexpectedEOF
		}
		if r.Method != "HEAD" || r.URL.Path != "/key" {
			t.Fatal(r.Method, r.URL)
		}
		return response(200, ""), nil
	})
	virtual := c.WithVirtualHosted()
	b, e := virtual.Bucket("test-bucket")
	if e != nil || b.Kind != "bucket_vhost_name" || b.EndpointURL != "https://test-bucket.s3.ru-1.storage.selcloud.ru" {
		t.Fatal(e, b.Kind)
	}
	r, e := virtual.Object(b, "key")
	if e != nil || r.Kind != "object_vhost_key" {
		t.Fatal(e)
	}
	v, e := virtual.Objects.Upload(context.Background(), r, strings.NewReader("data"), 4, s3.UploadOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome {
		t.Fatal(e)
	}
	h, e := root.Resume(*v.Receipt)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := h.Recover(context.Background()); types.ErrorCode(e) != types.UnknownOutcome || got.Observation.Ref.EndpointURL != r.EndpointURL || calls != 2 {
		t.Fatal(e, calls)
	}
	normal, _ := c.Bucket("test-bucket")
	if normal.Kind != "bucket_name" || normal.EndpointURL != "https://s3.ru-1.storage.selcloud.ru" {
		t.Fatal("mutated original view")
	}
}
func TestVirtualHostedCollectionCursorCannotCrossBucket(t *testing.T) {
	calls := 0
	_, base := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "test-bucket.s3.ru-1.storage.selcloud.ru" || r.URL.Path != "/" {
			t.Fatal(r.URL)
		}
		return response(200, `<ListBucketResult><Name>test-bucket</Name><IsTruncated>true</IsTruncated><Contents><Key>one</Key></Contents><NextContinuationToken>private-token</NextContinuationToken></ListBucketResult>`), nil
	})
	c := base.WithVirtualHosted()
	b, _ := c.Bucket("test-bucket")
	v, e := c.Objects.Collect(context.Background(), b, s3.ObjectListOptions{}, pagination.Options{MaxItems: 1})
	if types.ErrorCode(e) != types.Incomplete || len(v.Items) != 1 || v.NextCursor == nil || calls != 1 {
		t.Fatal(e, len(v.Items), calls)
	}
	other, _ := c.Bucket("other-bucket")
	if _, e = c.Objects.Iterate(other, s3.ObjectListOptions{}, pagination.Options{Cursor: v.NextCursor}); e == nil || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestVirtualHostedCrossBucketCopyValidatesSourceEndpoint(t *testing.T) {
	calls := 0
	_, base := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "target-bucket.s3.ru-1.storage.selcloud.ru" || r.URL.Path != "/target" || r.Header.Get("X-Amz-Copy-Source") != "/source-bucket/source" {
			t.Fatal(r.URL)
		}
		return response(200, `<CopyObjectResult><ETag>"tag"</ETag></CopyObjectResult>`), nil
	})
	c := base.WithVirtualHosted()
	sourceBucket, _ := c.Bucket("source-bucket")
	targetBucket, _ := c.Bucket("target-bucket")
	source, _ := c.Object(sourceBucket, "source")
	target, _ := c.Object(targetBucket, "target")
	if _, e := c.Objects.Copy(context.Background(), source, target, s3.CopyOptions{}, types.CallOptions{}); e != nil || calls != 1 {
		t.Fatal(e, calls)
	}
	source.EndpointURL = "https://source-bucket.old.test"
	source.Parent.EndpointURL = "https://source-bucket.old.test"
	if v, e := c.Objects.Copy(context.Background(), source, target, s3.CopyOptions{}, types.CallOptions{}); types.ErrorCode(e) != types.EndpointUnavailable || v.Receipt.Outcome != types.NotSent || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestCopyEmbeddedErrorAndMalformedSuccessRetainAcknowledgement(t *testing.T) {
	for _, body := range []string{`<Error><Code>AccessDenied</Code><Message>private-provider-text</Message></Error>`, `<CopyObjectResult><ETag>`} {
		t.Run(body, func(t *testing.T) {
			_, c := clients(t, func(*http.Request) (*http.Response, error) { return response(200, body), nil })
			b, _ := c.Bucket("test-bucket")
			source, _ := c.Object(b, "source")
			target, _ := c.Object(b, "target")
			v, e := c.Objects.Copy(context.Background(), source, target, s3.CopyOptions{}, types.CallOptions{})
			if e == nil || v.Receipt.Outcome != types.Accepted || len(v.Receipt.ResourceRefs) != 1 || v.Receipt.ResourceRefs[0].ID != "target" || strings.Contains(fmt.Sprint(e), "private-provider-text") {
				t.Fatal(e)
			}
		})
	}
}
func TestBlockedErrorDiagnosticsHonorDeadlineAndKnownRejection(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		v := response(403, "")
		b := &blockingBody{closed: make(chan struct{}), first: true}
		v.Body = b
		time.AfterFunc(250*time.Millisecond, func() { _ = b.Close() })
		return v, nil
	})
	b, _ := c.Bucket("test-bucket")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	v, e := c.Buckets.Delete(ctx, b, types.CallOptions{})
	if types.ErrorCode(e) != types.DeadlineExceeded || v.Receipt.Outcome != types.Rejected || time.Since(started) > 200*time.Millisecond {
		t.Fatal(e, time.Since(started))
	}
}
func TestDeleteManyMissingResultsAreIncompleteAndControlKeysNotRewritten(t *testing.T) {
	calls := 0
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		calls++
		return response(200, `<DeleteResult><Deleted><Key>one</Key></Deleted></DeleteResult>`), nil
	})
	b, _ := c.Bucket("test-bucket")
	v, e := c.Objects.DeleteMany(context.Background(), b, s3.DeleteManyRequest{Objects: []s3.DeleteEntry{{Key: "one"}, {Key: "two"}}}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || v.Data.Complete || len(v.Data.Deleted) != 1 || v.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	v, e = c.Objects.DeleteMany(context.Background(), b, s3.DeleteManyRequest{Objects: []s3.DeleteEntry{{Key: "unsafe\x01key"}}}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || v.Receipt.Outcome != types.NotSent || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestShortDownloadBodyIsNotCompleteAndKeepsPrefix(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) {
		v := response(200, "data")
		v.Header.Set("Content-Length", "9")
		return v, nil
	})
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "key")
	var out strings.Builder
	v, e := c.Objects.Download(context.Background(), r, &out, s3.DownloadOptions{MaxBytes: 100}, types.CallOptions{})
	if types.ErrorCode(e) != types.ProtocolError || v.Data.Complete || v.Data.Bytes != 4 || out.String() != "data" {
		t.Fatal(e, v.Data.Complete, v.Data.Bytes)
	}
}
func TestZeroByteUploadHasKnownEmptyBodyAndForeignCreateLocationIsNotSent(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "PUT" || r.ContentLength != 0 || r.Body != http.NoBody {
			t.Fatal("zero body must not become unknown-length chunked upload")
		}
		return response(200, ""), nil
	})
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "empty")
	v, e := c.Objects.Upload(context.Background(), r, nil, 0, s3.UploadOptions{}, types.CallOptions{})
	if e != nil || !v.Data.Complete || v.Data.Bytes != 0 || calls != 1 {
		t.Fatal(e, calls)
	}
	foreign := "ru-7"
	created, e := c.Buckets.Create(context.Background(), b, s3.CreateBucketOptions{LocationConstraint: &foreign}, types.CallOptions{})
	if types.ErrorCode(e) != types.ScopeMismatch || created.Receipt.Outcome != types.NotSent || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestKeyEscapingMatchesNativeS3Serializer(t *testing.T) {
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/test-bucket/a%2Bb%26%3D%3A%3F/key%20space" {
			t.Fatal(r.URL.EscapedPath())
		}
		return response(200, ""), nil
	})
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "a+b&=:?/key space")
	if _, e := c.Objects.Head(context.Background(), r, s3.HeadObjectOptions{}, types.CallOptions{}); e != nil {
		t.Fatal(e)
	}
}
func TestInvalidMetadataHeadersAreNotSent(t *testing.T) {
	_, c := clients(t, func(*http.Request) (*http.Response, error) { t.Fatal("invalid header sent"); return nil, nil })
	b, _ := c.Bucket("test-bucket")
	r, _ := c.Object(b, "key")
	for _, meta := range []map[string]types.ProtectedValue{{"bad:name": types.Protect([]byte("value"))}, {"name": types.Protect([]byte("bad\r\nvalue"))}, {"name": types.Protect([]byte("bad\x01value"))}} {
		v, e := c.Objects.Upload(context.Background(), r, strings.NewReader("data"), 4, s3.UploadOptions{Metadata: meta}, types.CallOptions{})
		if types.ErrorCode(e) != types.InvalidArgument || v.Receipt.Outcome != types.NotSent {
			t.Fatal(e)
		}
	}
}
func TestExpiredS3MaterialNeverSignsRequestsOrURLs(t *testing.T) {
	scope := types.Scope{AccountID: "a", ProjectID: "p", Region: "spb", Pool: "ru-1"}
	root, e := sdk.New(sdk.Config{AccountID: "a", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		return credentials.Material{Identity: "s", Revision: "1", Scheme: credentials.S3, Binding: scope, Audience: "selectel", AccessKey: types.Protect([]byte("synthetic-access")), SecretKey: types.Protect([]byte("synthetic-secret")), ExpiresAt: time.Now().Add(-time.Minute)}, nil
	}), Transport: transport(func(*http.Request) (*http.Response, error) { t.Fatal("expired credentials HTTP"); return nil, nil })})
	if e != nil {
		t.Fatal(e)
	}
	p, _ := root.Project("p")
	r, _ := p.Location("spb", "ru-1")
	c := r.S3()
	b, _ := c.Bucket("test-bucket")
	key, _ := c.Object(b, "key")
	if _, e = c.Objects.Head(context.Background(), key, s3.HeadObjectOptions{}, types.CallOptions{}); types.ErrorCode(e) != types.Unauthenticated {
		t.Fatal(e)
	}
	if _, e = c.Objects.PresignGet(context.Background(), key, s3.PresignOptions{Expires: time.Minute}, types.CallOptions{}); types.ErrorCode(e) != types.Unauthenticated {
		t.Fatal(e)
	}
}

func TestListV1DelimiterCanFallbackToLastKeyWhenMarkerOmitted(t *testing.T) {
	calls := 0
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("delimiter") != "/" {
			t.Fatal(r.URL)
		}
		if calls == 1 {
			return response(200, `<ListBucketResult><Name>test-bucket</Name><IsTruncated>true</IsTruncated><Contents><Key>last</Key><Size>1</Size></Contents></ListBucketResult>`), nil
		}
		if r.URL.Query().Get("marker") != "last" {
			t.Fatal(r.URL)
		}
		return response(200, `<ListBucketResult><Name>test-bucket</Name><IsTruncated>false</IsTruncated></ListBucketResult>`), nil
	})
	ref, _ := c.Bucket("test-bucket")
	delimiter := "/"
	it, e := c.Objects.IterateV1(ref, s3.ObjectListOptions{Delimiter: &delimiter}, pagination.Options{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = it.Next(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = it.Next(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestConfirmedConditionalCopyDatesAndDeleteIfMatch(t *testing.T) {
	calls := 0
	after := time.Date(2026, 10, 1, 3, 4, 5, 0, time.FixedZone("offset", 3600))
	before := after.Add(time.Hour)
	etag := `"expected"`
	_, c := clients(t, func(r *http.Request) (*http.Response, error) {
		calls++
		switch r.Method {
		case "PUT":
			if r.Header.Get("X-Amz-Copy-Source-If-Modified-Since") != after.UTC().Format(http.TimeFormat) || r.Header.Get("X-Amz-Copy-Source-If-Unmodified-Since") != before.UTC().Format(http.TimeFormat) {
				t.Fatalf("missing source date conditions: %v", r.Header)
			}
			return response(412, `<Error><Code>PreconditionFailed</Code><Message>must discard</Message></Error>`), nil
		case "DELETE":
			if r.Header.Get("If-Match") != etag {
				t.Fatal("missing delete If-Match")
			}
			return response(412, `<Error><Code>PreconditionFailed</Code></Error>`), nil
		default:
			t.Fatal(r.Method)
			return nil, nil
		}
	})
	bucket, _ := c.Bucket("test-bucket")
	src, _ := c.Object(bucket, "source")
	dst, _ := c.Object(bucket, "target")
	var sdkerr *types.SDKError
	copied, e := c.Objects.Copy(context.Background(), src, dst, s3.CopyOptions{SourceIfModifiedSince: &after, SourceIfUnmodifiedSince: &before}, types.CallOptions{})
	if (!errors.As(e, &sdkerr) || sdkerr.Code != types.Conflict) || copied.Receipt == nil || copied.Receipt.Outcome != types.Rejected {
		t.Fatal(e, copied.Receipt)
	}
	deleted, e := c.Objects.Delete(context.Background(), dst, s3.DeleteObjectOptions{IfMatch: &etag}, types.CallOptions{})
	if (!errors.As(e, &sdkerr) || sdkerr.Code != types.Conflict) || deleted.Receipt == nil || deleted.Receipt.Outcome != types.Rejected {
		t.Fatal(e, deleted.Receipt)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
	bad := "etag\r\nInjected: yes"
	notSent, e := c.Objects.Delete(context.Background(), dst, s3.DeleteObjectOptions{IfMatch: &bad}, types.CallOptions{})
	if (!errors.As(e, &sdkerr) || sdkerr.Code != types.InvalidArgument) || notSent.Receipt == nil || notSent.Receipt.Outcome != types.NotSent || calls != 2 {
		t.Fatal(e, calls)
	}
}
