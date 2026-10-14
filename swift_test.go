package selectelsdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	sdk "github.com/festy23/selectel-sdk"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/swift"
	"github.com/festy23/selectel-sdk/types"
)

func TestSwiftCurrentAccountHeadersUseProjectIAMAndProtectMetadata(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "HEAD" || r.URL.Host != "swift.ru-1.storage.selcloud.ru" || r.URL.Path != "/v1/test-project" || r.Header.Get("X-Auth-Token") == "" || r.Header.Get("Authorization") != "" {
			t.Fatal("Swift auth/scope/wire incorrect")
		}
		out := inventoryReply(204, "")
		out.Header.Set("X-Account-Bytes-Used", "900719925474099312345")
		out.Header.Set("X-Account-Container-Count", "2")
		out.Header.Set("X-Account-Object-Count", "3")
		out.Header.Set("X-Timestamp", "1374058535.429270001")
		out.Header.Set("X-Account-Meta-Temp-Url-Key", "synthetic-private-tempurl")
		out.Header.Set("X-Trans-Id", "swift-request1")
		return out, nil
	}, 0)
	out, e := regionalView(t, c, "ru-1").Swift().Account(context.Background(), types.CallOptions{})
	if e != nil || out.Data.BytesUsed.String() != "900719925474099312345" || out.Data.Created.String() != "1374058535.429270001" || out.Data.Ref.Scope.ProjectID != "test-project" {
		t.Fatal("Swift header precision/scope lost", e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private-tempurl") {
		t.Fatal("temporary URL secret leaked")
	}
}

func TestSwiftCurrentBucketMetadataAndMutationReceipts(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("X-Auth-Token") == "" || r.URL.Path != "/v1/test-project/bucket" {
			t.Fatal("wrong scope")
		}
		switch r.Method {
		case "PUT":
			if r.Header.Get("X-Container-Meta-Type") != "private" || r.Header.Get("X-Versions-Enabled") != "false" {
				t.Fatal("create flags lost")
			}
			return inventoryReply(201, ""), nil
		case "POST":
			if r.Header.Get("X-Container-Meta-Quota-Bytes") != "0" || r.Header.Get("X-Container-Meta-Private") != "synthetic-private-metadata" || r.Header.Get("X-Remove-Container-Meta-Old") != "true" {
				t.Fatal("metadata/quota/removal changed")
			}
			return inventoryReply(202, "accepted"), nil
		case "DELETE":
			return inventoryReply(204, ""), nil
		}
		t.Fatal("unexpected effect")
		return nil, nil
	}, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	created, e := svc.CreateBucket(context.Background(), "bucket", swift.BucketCreate{Type: managementPtr("private"), VersionsEnabled: managementPtr(false)}, types.CallOptions{})
	if e != nil || created.Receipt == nil || created.Receipt.Outcome != types.Accepted || created.Data.Ref.Parent.ID != "test-project" {
		t.Fatal("create bucket", e)
	}
	n, _ := types.ParseInteger("0")
	out, e := svc.UpdateBucket(context.Background(), *created.Data.Ref, swift.BucketUpdate{QuotaBytes: &n, Metadata: swift.MetadataUpdate{Set: map[string]types.ProtectedValue{"Private": types.Protect([]byte("synthetic-private-metadata"))}, Remove: []string{"Old"}}}, types.CallOptions{})
	if e != nil || out.Receipt.Outcome != types.Accepted {
		t.Fatal("update bucket", e)
	}
	deleted, e := svc.DeleteBucket(context.Background(), *created.Data.Ref, types.CallOptions{})
	if e != nil || deleted.Receipt.Outcome != types.Accepted || calls != 3 {
		t.Fatal("delete bucket", e)
	}
}

func TestSwiftCurrentTransfersStreamExplicitBytesAndNeverReplay(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("X-Auth-Token") == "" || r.URL.EscapedPath() != "/v1/test-project/bucket/folder%2Fobject" {
			t.Fatal("transfer scope/path")
		}
		if r.Method == "PUT" {
			raw, e := io.ReadAll(r.Body)
			if e != nil || string(raw) != "synthetic-private-binary" || r.ContentLength != 24 {
				t.Fatal("upload bytes/length")
			}
			return inventoryReply(201, ""), nil
		}
		out := inventoryReply(200, "abcdef")
		out.Header.Set("Content-Length", "6")
		return out, nil
	}, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	bucket, e := svc.Bucket("bucket")
	if e != nil {
		t.Fatal(e)
	}
	object, e := svc.Object(bucket, "folder/object")
	if e != nil {
		t.Fatal(e)
	}
	input := &swiftOwnedReader{Reader: strings.NewReader("synthetic-private-binary")}
	uploaded, e := svc.Upload(context.Background(), object, input, 24, swift.UploadOptions{}, types.CallOptions{})
	if e != nil || uploaded.Receipt == nil || uploaded.Receipt.Outcome != types.Accepted || !uploaded.Data.Complete || input.closed {
		t.Fatal("upload receipt/reader ownership", e)
	}
	var dst bytes.Buffer
	download, e := svc.Download(context.Background(), object, &dst, swift.DownloadOptions{MaxBytes: 3}, types.CallOptions{})
	if types.ErrorCode(e) != types.ResponseLimit || download.Data.Complete || download.Data.Bytes != 3 || dst.String() != "abc" || calls != 2 {
		t.Fatal("bounded download", e)
	}
}

type swiftOwnedReader struct {
	*strings.Reader
	closed bool
}

func (r *swiftOwnedReader) Close() error { r.closed = true; return nil }

func TestSwiftCurrentListsContinueAfterClampedPagesAndBindCursor(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("format") != "json" || r.URL.Query().Get("limit") != "2" {
			t.Fatal("wrong listing bounds/format")
		}
		body := `[]`
		switch r.URL.Query().Get("marker") {
		case "":
			body = `[{"name":"bucket-a","count":1,"bytes":900719925474099312345,"last_modified":"2016-04-29T16:23:50.46023012345","ref":{"id":"foreign","service":"compute"}}]`
		case "bucket-a":
			body = `[{"name":"bucket-b","count":0,"bytes":0}]`
		}
		out := inventoryReply(200, body)
		out.Header.Set("X-Account-Container-Count", "2")
		out.Header.Set("X-Account-Meta-Temp-Url-Key", "synthetic-private-tempurl")
		return out, nil
	}, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	first, e := svc.Buckets(context.Background(), swift.ListOptions{}, pagination.Options{PageSize: 2, MaxPages: 1})
	if types.ErrorCode(e) != types.Incomplete || first.Complete || len(first.Items) != 1 || first.NextCursor == nil || first.Items[0].Ref.Service != "swift" || first.Items[0].Bytes.String() != "900719925474099312345" || *first.Items[0].LastModified != "2016-04-29T16:23:50.46023012345" {
		t.Fatal("clamped list/cursor lost", e)
	}
	resumed, e := svc.Buckets(context.Background(), swift.ListOptions{}, pagination.Options{PageSize: 2, Cursor: first.NextCursor})
	if e != nil || !resumed.Complete || len(resumed.Items) != 1 || *resumed.Items[0].Name != "bucket-b" || len(resumed.HeaderPages) != 2 || resumed.HeaderPages[0].ContainerCount.String() != "2" || calls != 3 {
		t.Fatal("continuation ended too early", e)
	}
	prefix := "changed"
	_, e = svc.Buckets(context.Background(), swift.ListOptions{Prefix: &prefix}, pagination.Options{PageSize: 2, Cursor: first.NextCursor})
	if types.ErrorCode(e) != types.ScopeMismatch || calls != 3 {
		t.Fatal("cursor filter binding bypassed", e)
	}
	raw, _ := json.Marshal(resumed)
	if strings.Contains(string(raw), "synthetic-private-tempurl") {
		t.Fatal("listing metadata leaked")
	}
}

func TestSwiftCurrentCopyAndManifestVariantsKeepExplicitSourceRefs(t *testing.T) {
	actions := []string{}
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Auth-Token") == "" {
			t.Fatal("Swift auth missing")
		}
		action := r.Method + ":" + r.URL.Query().Get("multipart-manifest")
		actions = append(actions, action)
		switch {
		case r.Method == "COPY":
			if r.Header.Get("Destination") != "/dest/copied" || r.Header.Get("X-Fresh-Metadata") != "false" {
				t.Fatal("copy destination/false lost")
			}
		case r.Header.Get("X-Copy-From") != "":
			if r.Header.Get("X-Copy-From") != "/source/original" {
				t.Fatal("copy source lost")
			}
		case r.Header.Get("X-Object-Manifest") != "":
			if r.Header.Get("X-Object-Manifest") != "source/chunks%2F" {
				t.Fatal("DLO prefix changed")
			}
		case r.Header.Get("X-Symlink-Target") != "":
			if r.Header.Get("X-Symlink-Target") != "source/original" || r.Header.Get("X-Symlink-Target-Etag") != "tag1" {
				t.Fatal("symlink target lost")
			}
		case r.URL.Query().Get("multipart-manifest") == "put":
			raw, _ := io.ReadAll(r.Body)
			if string(raw) != `[{"path":"/source/original","etag":"tag1","size_bytes":9007199254740993}]` {
				t.Fatal("SLO explicit path/checksum/precision lost")
			}
		default:
			t.Fatal("unknown put variant")
		}
		return inventoryReply(201, ""), nil
	}, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	srcBucket, _ := svc.Bucket("source")
	dstBucket, _ := svc.Bucket("dest")
	source, _ := svc.Object(srcBucket, "original")
	target, _ := svc.Object(dstBucket, "copied")
	copied, e := svc.Copy(context.Background(), source, target, swift.CopyOptions{FreshMetadata: managementPtr(false)}, types.CallOptions{})
	if e != nil || copied.Receipt.Outcome != types.Accepted || len(copied.Receipt.ResourceRefs) != 2 || copied.Data.Ref.Parent.ID != "dest" {
		t.Fatal("COPY refs", e)
	}
	_, e = svc.CopyFrom(context.Background(), source, target, swift.UploadOptions{}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = svc.PutDynamicManifest(context.Background(), target, srcBucket, "chunks/", swift.UploadOptions{}, types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = svc.PutSymlink(context.Background(), target, source, managementPtr("tag1"), types.CallOptions{})
	if e != nil {
		t.Fatal(e)
	}
	n, _ := types.ParseInteger("9007199254740993")
	_, e = svc.PutStaticManifest(context.Background(), target, []swift.Segment{{Object: source, ETag: managementPtr("tag1"), SizeBytes: &n}}, swift.UploadOptions{}, types.CallOptions{})
	if e != nil || len(actions) != 5 {
		t.Fatal("SLO refs", e)
	}
}

func TestSwiftCurrentBulkDeleteRetainsPartialErrorsAndExactRefs(t *testing.T) {
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v1/test-project" || r.URL.Query().Get("bulk-delete") != "true" || r.URL.Query().Get("format") != "json" {
			t.Fatal("bulk selector wrong")
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != "bucket/one\nbucket/two" {
			t.Fatal("bulk exact targets changed")
		}
		return inventoryReply(200, `{"Number Deleted":1,"Number Not Found":0,"Response Status":"400 Bad Request","Response Body":"synthetic-private-response","Errors":[["bucket/two","synthetic-private-failure"]]}`), nil
	}, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	bucket, _ := svc.Bucket("bucket")
	a, _ := svc.Object(bucket, "one")
	b, _ := svc.Object(bucket, "two")
	out, e := svc.BulkDelete(context.Background(), []types.ResourceRef{a, b}, types.CallOptions{})
	if types.ErrorCode(e) != types.OperationFailed || out.Receipt == nil || out.Receipt.Outcome != types.Accepted || len(out.Receipt.ResourceRefs) != 2 || out.Data.NumberDeleted.String() != "1" || len(out.Data.Errors) != 1 {
		t.Fatal("partial delete lost summary/receipt", e)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("bulk provider payload leaked")
	}
}

func TestSwiftCurrentDeleteSegmentsIsExplicit(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "DELETE" || (calls == 1 && r.URL.RawQuery != "") || (calls == 2 && r.URL.Query().Get("multipart-manifest") != "delete") {
			t.Fatal("implicit or missing cascade")
		}
		return inventoryReply(204, ""), nil
	}, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	b, _ := svc.Bucket("bucket")
	obj, _ := svc.Object(b, "manifest")
	a, e := svc.DeleteObject(context.Background(), obj, types.CallOptions{})
	if e != nil || a.Receipt.Outcome != types.Accepted {
		t.Fatal(e)
	}
	a, e = svc.DeleteSegments(context.Background(), obj, types.CallOptions{})
	if e != nil || a.Receipt.Outcome != types.Accepted || len(a.Receipt.ResourceRefs) != 1 {
		t.Fatal(e)
	}
}

type swiftCancelBody struct {
	first  bool
	closed chan struct{}
	closes atomic.Int32
}

func (r *swiftCancelBody) Read(p []byte) (int, error) {
	if !r.first {
		r.first = true
		return copy(p, "prefix"), nil
	}
	<-r.closed
	return 0, errors.New("synthetic-private-read")
}
func (r *swiftCancelBody) Close() error {
	if r.closes.Add(1) == 1 {
		close(r.closed)
	}
	return nil
}

type swiftCancelWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
	closed bool
}

func (w *swiftCancelWriter) Write(p []byte) (int, error) {
	n, e := w.Buffer.Write(p)
	w.cancel()
	return n, e
}
func (w *swiftCancelWriter) Close() error { w.closed = true; return nil }
func TestSwiftCurrentCancellationClosesResponseOnceAndRetainsPrefix(t *testing.T) {
	body := &swiftCancelBody{closed: make(chan struct{})}
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		out := inventoryReply(200, "")
		out.Body = body
		return out, nil
	}, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	b, _ := svc.Bucket("bucket")
	ref, _ := svc.Object(b, "object")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dst := swiftCancelWriter{cancel: cancel}
	out, e := svc.Download(ctx, ref, &dst, swift.DownloadOptions{MaxBytes: 100}, types.CallOptions{})
	if types.ErrorCode(e) != types.Canceled || out.Data.Bytes != 6 || out.Data.Complete || dst.String() != "prefix" || calls != 1 || body.closes.Load() != 1 || dst.closed {
		t.Fatal("cancellation ownership/progress", e)
	}
}
func TestSwiftCurrentUnknownUploadNeverReplays(t *testing.T) {
	calls := 0
	c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
		calls++
		_, _ = io.Copy(io.Discard, r.Body)
		return nil, errors.New("synthetic-private-network")
	}, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	b, _ := svc.Bucket("bucket")
	ref, _ := svc.Object(b, "object")
	input := &swiftOwnedReader{Reader: strings.NewReader("data")}
	out, e := svc.Upload(context.Background(), ref, input, 4, swift.UploadOptions{}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || out.Receipt == nil || out.Receipt.Outcome != types.Unknown || out.Receipt.Attempts != 1 || len(out.Receipt.ResourceRefs) != 1 || calls != 1 || input.closed || out.Data.Bytes != 4 || out.Data.Complete || !reflect.DeepEqual(out.Data.Object.Ref, &ref) {
		t.Fatal("upload replay/ownership/progress", e)
	}
}
func TestSwiftCurrentTransferUsesTransferBudget(t *testing.T) {
	var remaining time.Duration
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), CallTimeout: time.Second, TransferTimeout: 5 * time.Second, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatal("no transfer deadline")
		}
		remaining = time.Until(deadline)
		return inventoryReply(200, "abc"), nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	svc := regionalView(t, c, "ru-1").Swift()
	b, _ := svc.Bucket("bucket")
	ref, _ := svc.Object(b, "object")
	var dst bytes.Buffer
	out, e := svc.Download(context.Background(), ref, &dst, swift.DownloadOptions{MaxBytes: 3}, types.CallOptions{})
	if e != nil || !out.Data.Complete || remaining < 3*time.Second || remaining > 5*time.Second {
		t.Fatal("wrong transfer budget", e)
	}
}

func TestSwiftCurrentUnknownAccountMetadataRetainsExactAccount(t *testing.T) {
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }, 0)
	out, e := regionalView(t, c, "ru-1").Swift().UpdateAccount(context.Background(), swift.AccountUpdate{}, types.CallOptions{})
	if types.ErrorCode(e) != types.UnknownOutcome || out.Receipt == nil || len(out.Receipt.ResourceRefs) != 1 || out.Receipt.ResourceRefs[0].Kind != "account" || out.Receipt.ResourceRefs[0].ID != "test-project" {
		t.Fatal("known account locator lost", e)
	}
}

func TestSwiftCurrentMutationBindingFailuresAlwaysFinalizeReceipt(t *testing.T) {
	calls, lookups := 0, 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		lookups++
		return credentials.Material{}, errors.New("unexpected credentials")
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected I/O") })})
	if e != nil {
		t.Fatal(e)
	}
	good := regionalView(t, c, "ru-1").Swift()
	b, _ := good.Bucket("bucket")
	obj, _ := good.Object(b, "object")
	other, _ := good.Object(b, "other")
	for _, pool := range []string{"", "unsupported-pool"} {
		view, e := projectView(t, c).Location("ru-1", pool)
		if e != nil {
			t.Fatal(e)
		}
		s := view.Swift()
		cases := []func() (*types.Receipt, error){
			func() (*types.Receipt, error) {
				r, e := s.UpdateAccount(context.Background(), swift.AccountUpdate{}, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.CreateBucket(context.Background(), "bucket", swift.BucketCreate{}, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.UpdateBucket(context.Background(), b, swift.BucketUpdate{}, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.DeleteBucket(context.Background(), b, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.Upload(context.Background(), obj, nil, 0, swift.UploadOptions{}, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.UpdateObject(context.Background(), obj, swift.ObjectUpdate{}, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.Copy(context.Background(), obj, other, swift.CopyOptions{}, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.CopyFrom(context.Background(), obj, other, swift.UploadOptions{}, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.PutDynamicManifest(context.Background(), obj, b, "chunks/", swift.UploadOptions{}, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.PutStaticManifest(context.Background(), obj, []swift.Segment{{Object: other}}, swift.UploadOptions{}, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.PutSymlink(context.Background(), obj, other, nil, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.DeleteObject(context.Background(), obj, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.DeleteSegments(context.Background(), obj, types.CallOptions{})
				return r.Receipt, e
			},
			func() (*types.Receipt, error) {
				r, e := s.BulkDelete(context.Background(), []types.ResourceRef{obj}, types.CallOptions{})
				return r.Receipt, e
			},
		}
		for i, f := range cases {
			r, e := f()
			if e == nil || r == nil || r.Outcome != types.NotSent || r.Attempts != 0 {
				t.Fatalf("case %d pool %q receipt failure: %v", i, pool, e)
			}
		}
	}
	if calls != 0 || lookups != 0 {
		t.Fatal("binding failure reached credentials/HTTP")
	}
}

func TestSwiftCurrentForgedContainerCannotBecomeObjectParent(t *testing.T) {
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) { t.Fatal("invalid ref reached HTTP"); return nil, nil }, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	b, _ := svc.Bucket("bucket")
	b.ID = "other/container"
	if _, e := svc.Object(b, "object"); types.ErrorCode(e) != types.InvalidArgument {
		t.Fatal("malformed container accepted", e)
	}
}

func TestSwiftCurrentTypedQuotaCannotCollideWithRemoval(t *testing.T) {
	c := inventoryClient(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("conflicting quota reached HTTP")
		return nil, nil
	}, 0)
	svc := regionalView(t, c, "ru-1").Swift()
	zero, _ := types.ParseInteger("0")
	out, e := svc.UpdateAccount(context.Background(), swift.AccountUpdate{QuotaBytes: &zero, Metadata: swift.MetadataUpdate{Remove: []string{"Quota-Bytes"}}}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent {
		t.Fatal("quota removal collision", e)
	}
}

func TestSwiftCurrentProtectedHeaderRejectsControlBeforeCredentials(t *testing.T) {
	lookups := 0
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: credentials.SourceFunc(func(context.Context, types.AuthRequest) (credentials.Material, error) {
		lookups++
		return credentials.Material{}, errors.New("unexpected credential read")
	}), Transport: transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("invalid header reached HTTP"); return nil, nil })})
	if e != nil {
		t.Fatal(e)
	}
	svc := regionalView(t, c, "ru-1").Swift()
	out, e := svc.UpdateAccount(context.Background(), swift.AccountUpdate{Metadata: swift.MetadataUpdate{Set: map[string]types.ProtectedValue{"key": types.Protect([]byte("value\x01suffix"))}}}, types.CallOptions{})
	if types.ErrorCode(e) != types.InvalidArgument || out.Receipt == nil || out.Receipt.Outcome != types.NotSent || lookups != 0 {
		t.Fatal("invalid header passed preflight", e)
	}
}

type swiftLateBody struct {
	closed chan struct{}
	once   atomic.Bool
	read   bool
}

func (r *swiftLateBody) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	<-r.closed
	r.read = true
	return copy(p, "late"), nil
}
func (r *swiftLateBody) Close() error {
	if r.once.CompareAndSwap(false, true) {
		close(r.closed)
	}
	return nil
}
func TestSwiftCurrentTransferDeadlinePreventsLateWriterEmission(t *testing.T) {
	body := &swiftLateBody{closed: make(chan struct{})}
	c, e := sdk.New(sdk.Config{AccountID: "test-account", Credentials: source(), TransferTimeout: 50 * time.Millisecond, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/identity/v3/auth/tokens" {
			return response(requestedScope(r), time.Now().Add(time.Hour), 201), nil
		}
		out := inventoryReply(200, "")
		out.Body = body
		return out, nil
	})})
	if e != nil {
		t.Fatal(e)
	}
	svc := regionalView(t, c, "ru-1").Swift()
	b, _ := svc.Bucket("bucket")
	ref, _ := svc.Object(b, "object")
	var dst bytes.Buffer
	out, e := svc.Download(context.Background(), ref, &dst, swift.DownloadOptions{MaxBytes: 10}, types.CallOptions{})
	if types.ErrorCode(e) != types.DeadlineExceeded || dst.Len() != 0 || out.Data.Bytes != 0 || out.Data.Complete {
		t.Fatal("expired transfer emitted late bytes", e)
	}
}

func TestSwiftCurrentUploadProgressSurvivesCancellationAndRejection(t *testing.T) {
	for _, which := range []string{"cancel", "rejected"} {
		t.Run(which, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			c := inventoryClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				buf := make([]byte, 4)
				if _, e := io.ReadFull(r.Body, buf); e != nil {
					t.Fatal(e)
				}
				if which == "cancel" {
					cancel()
					return nil, context.Canceled
				}
				return inventoryReply(403, "synthetic-private-rejection"), nil
			}, 0)
			svc := regionalView(t, c, "ru-1").Swift()
			b, _ := svc.Bucket("bucket")
			ref, _ := svc.Object(b, "object")
			input := &swiftOwnedReader{Reader: strings.NewReader("fourmore")}
			out, e := svc.Upload(ctx, ref, input, 8, swift.UploadOptions{}, types.CallOptions{})
			wantCode, wantOutcome := types.UnknownOutcome, types.Unknown
			if which == "rejected" {
				wantCode, wantOutcome = types.PermissionDenied, types.Rejected
			}
			if types.ErrorCode(e) != wantCode || out.Receipt == nil || out.Receipt.Outcome != wantOutcome || out.Receipt.Attempts != 1 || calls != 1 || input.closed || out.Data.Bytes != 4 || out.Data.Complete || !reflect.DeepEqual(out.Data.Object.Ref, &ref) || len(out.Receipt.ResourceRefs) != 1 || !reflect.DeepEqual(out.Receipt.ResourceRefs[0], ref) {
				t.Fatal("upload failure progress/receipt", e)
			}
			var detail *types.SDKError
			if !errors.As(e, &detail) || detail.Receipt != out.Receipt {
				t.Fatal("receipt detached from original error")
			}
			if which == "cancel" && detail.CauseCode != types.Canceled {
				t.Fatal("cancel cause lost")
			}
		})
	}
}
