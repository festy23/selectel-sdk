package swift

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func nilValue(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return r.IsNil()
	}
	return false
}

type countedReader struct {
	reader io.Reader
	n      atomic.Int64
	ctx    context.Context
}

func (r *countedReader) Read(p []byte) (int, error) {
	if r.ctx != nil && r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	n, e := r.reader.Read(p)
	r.n.Add(int64(n))
	return n, e
}

type directedWriter struct {
	writer io.Writer
	ctx    context.Context
}

func (w directedWriter) Write(p []byte) (int, error) {
	if w.ctx != nil && w.ctx.Err() != nil {
		return 0, w.ctx.Err()
	}
	return w.writer.Write(p)
}
func uploadHeaders(spec UploadOptions) (http.Header, error) {
	h, e := metadataRequest("X-Object-Meta-", spec.Metadata, nil)
	for _, f := range []struct {
		k string
		v *types.Integer
	}{{"X-Delete-At", spec.DeleteAt}, {"X-Delete-After", spec.DeleteAfter}} {
		if x := setInteger(h, f.k, f.v); x != nil {
			e = x
		}
	}
	if x := setProtected(h, "Content-Type", spec.ContentType); x != nil {
		e = x
	}
	if x := setText(h, "ETag", spec.ETag); x != nil {
		e = x
	}
	return h, e
}

// Upload consumes exactly size bytes; it does not close or probe beyond the
// caller-owned reader. A segment is uploaded with the same method and exact ref.
func (c *Client) Upload(ctx context.Context, ref types.ResourceRef, reader io.Reader, size int64, spec UploadOptions, o types.CallOptions) (types.Result[Transfer], error) {
	op, bindErr := c.operation("swift.v1.objects.put", "PUT", []int{201})
	h, encodeErr := uploadHeaders(spec)
	op.Headers = h
	if nilValue(reader) {
		reader = strings.NewReader("")
		if size != 0 {
			encodeErr = types.NewError(types.InvalidArgument)
		}
	}
	count := &countedReader{reader: io.LimitReader(reader, max(size, 0)), ctx: ctx}
	return core.DoResponse(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: refPath(ref), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Transfer: true, BodyStream: count, BodySize: size, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if encodeErr != nil {
			return encodeErr
		}
		if size < 0 {
			return types.NewError(types.InvalidArgument)
		}
		return c.refError(ref, "object")
	}}, func(call context.Context, r io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[Transfer], error) {
		known := cloneRef(ref)
		if e := c.headerLimit(h); e != nil {
			return core.Decoded[Transfer]{Data: Transfer{Object: ObjectInfo{Ref: &known}, Bytes: count.n.Load()}}, e
		}
		info, e := objectInfo(h, &known)
		if e == nil {
			e = c.headerLimit(h)
		}
		if e == nil {
			e = c.discard(r)
		}
		out := Transfer{Object: info, Bytes: count.n.Load()}
		if e == nil && out.Bytes != size {
			e = types.NewError(types.ProtocolError)
		}
		if e == nil && call.Err() != nil {
			e = types.ContextError(call.Err())
		}
		out.Complete = e == nil
		return core.Decoded[Transfer]{Data: out}, e
	})
}

// Download reveals bytes only to the caller-directed writer and never closes it.
// Bytes counts only the emitted prefix. No retry follows a started body copy.
func (c *Client) Download(ctx context.Context, ref types.ResourceRef, writer io.Writer, spec DownloadOptions, o types.CallOptions) (types.Result[Transfer], error) {
	op, bindErr := c.operation("swift.v1.objects.get", "GET", []int{200})
	op.Headers = http.Header{"Accept-Encoding": {"identity"}}
	query := url.Values{}
	if spec.Manifest {
		query.Set("multipart-manifest", "get")
	}
	if spec.Symlink {
		query.Set("symlink", "get")
	}
	return core.DoResponse(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: refPath(ref), Options: o, Query: query, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Transfer: true, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if spec.MaxBytes <= 0 || spec.MaxBytes == int64(^uint64(0)>>1) || nilValue(writer) || (spec.Manifest && spec.Symlink) {
			return types.NewError(types.InvalidArgument)
		}
		return c.refError(ref, "object")
	}}, func(call context.Context, r io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[Transfer], error) {
		known := cloneRef(ref)
		if e := c.headerLimit(h); e != nil {
			return core.Decoded[Transfer]{Data: Transfer{Object: ObjectInfo{Ref: &known}}, PartialRead: true}, e
		}
		info, e := objectInfo(h, &known)
		out := Transfer{Object: info}
		if e == nil {
			e = c.headerLimit(h)
		}
		if e != nil {
			return core.Decoded[Transfer]{Data: out, PartialRead: true}, e
		}
		n, copyErr := io.Copy(directedWriter{writer, call}, io.LimitReader(r, spec.MaxBytes))
		out.Bytes = n
		if copyErr != nil {
			return core.Decoded[Transfer]{Data: out, PartialRead: true}, types.NewError(types.TransportError)
		}
		var probe [1]byte
		more, readErr := r.Read(probe[:])
		if more > 0 {
			return core.Decoded[Transfer]{Data: out, PartialRead: true}, types.NewError(types.ResponseLimit)
		}
		if readErr != nil && readErr != io.EOF {
			return core.Decoded[Transfer]{Data: out, PartialRead: true}, types.NewError(types.TransportError)
		}
		if readErr == nil {
			return core.Decoded[Transfer]{Data: out, PartialRead: true}, types.NewError(types.ProtocolError)
		}
		if info.ContentLength != nil {
			expected, e := strconv.ParseInt(info.ContentLength.String(), 10, 64)
			if e != nil || expected < 0 || expected != n {
				return core.Decoded[Transfer]{Data: out, PartialRead: true}, types.NewError(types.ProtocolError)
			}
		}
		if call.Err() != nil {
			return core.Decoded[Transfer]{Data: out, PartialRead: true}, types.ContextError(call.Err())
		}
		out.Complete = true
		return core.Decoded[Transfer]{Data: out}, nil
	})
}
