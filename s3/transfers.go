package s3

import (
	"context"
	"encoding/base64"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

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
func header(h http.Header, key, value string) error {
	if key == "" {
		return types.NewError(types.InvalidArgument)
	}
	for _, c := range []byte(key) {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c))) {
			return types.NewError(types.InvalidArgument)
		}
	}
	for _, c := range []byte(value) {
		if c < 32 && c != '\t' || c == 127 {
			return types.NewError(types.InvalidArgument)
		}
	}
	h.Set(key, value)
	return nil
}

func uploadHeaders(in UploadOptions) (http.Header, error) {
	h := http.Header{"Content-Type": {"application/octet-stream"}}
	if in.ContentType != nil {
		if e := header(h, "Content-Type", string(in.ContentType.Reveal())); e != nil {
			return h, e
		}
	}
	for k, v := range in.Metadata {
		if k == "" || strings.ContainsAny(k, " :\t") {
			return h, types.NewError(types.InvalidArgument)
		}
		if e := header(h, "x-amz-meta-"+k, string(v.Reveal())); e != nil {
			return h, e
		}
	}
	for _, v := range []struct {
		k string
		v *string
	}{{"Content-MD5", in.ContentMD5}, {"If-Match", in.IfMatch}, {"If-None-Match", in.IfNoneMatch}} {
		if v.v != nil {
			if e := header(h, v.k, *v.v); e != nil {
				return h, e
			}
		}
	}
	if in.ContentMD5 != nil {
		b, e := base64.StdEncoding.DecodeString(*in.ContentMD5)
		if e != nil || len(b) != 16 {
			return h, types.NewError(types.InvalidArgument)
		}
	}
	return h, nil
}
func objectHeaders(h http.Header, ref types.ResourceRef) ObjectInfo {
	out := ObjectInfo{Ref: &ref, Metadata: map[string]types.ProtectedValue{}}
	if raw := h.Get("Content-Length"); raw != "" {
		v, _ := types.ParseInteger(raw)
		out.ContentLength = &v
	}
	for _, field := range []struct {
		name string
		dest **types.ProtectedValue
	}{{"Cache-Control", &out.CacheControl}, {"Content-Disposition", &out.ContentDisposition}, {"Content-Encoding", &out.ContentEncoding}, {"Content-Language", &out.ContentLanguage}, {"Expires", &out.Expires}} {
		if value := h.Get(field.name); value != "" {
			p := types.Protect([]byte(value))
			*field.dest = &p
		}
	}
	for _, field := range []struct {
		name string
		dest **string
	}{{"Content-Range", &out.ContentRange}, {"Accept-Ranges", &out.AcceptRanges}, {"X-Amz-Storage-Class", &out.StorageClass}, {"Last-Modified", &out.LastModified}} {
		if value := h.Get(field.name); value != "" {
			*field.dest = &value
		}
	}
	if raw := h.Get("X-Amz-Delete-Marker"); raw != "" {
		v := raw == "true"
		out.DeleteMarker = &v
	}
	if raw := h.Get("X-Amz-Copy-Source-Version-Id"); raw != "" {
		p := types.Protect([]byte(raw))
		out.SourceVersionID = &p
	}

	if s := h.Get("ETag"); s != "" {
		out.ETag = &s
	}
	if s := h.Get("x-amz-version-id"); s != "" {
		v := types.Protect([]byte(s))
		out.VersionID = &v
	}
	if s := h.Get("Content-Type"); s != "" {
		v := types.Protect([]byte(s))
		out.ContentType = &v
	}
	for k, v := range h {
		if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") {
			out.Metadata[strings.TrimPrefix(strings.ToLower(k), "x-amz-meta-")] = types.Protect([]byte(strings.Join(v, ",")))
		}
	}
	return out
}

// Upload consumes at most size caller bytes; Bytes reports input consumed, not persistence.
func (c *Objects) Upload(ctx context.Context, r types.ResourceRef, reader io.Reader, size int64, in UploadOptions, o types.CallOptions) (types.Result[Transfer], error) {
	s := c.client
	op, bind := s.callOp("objects.put", "PUT", 200)
	op = addressOperation(op, r)
	h, encode := uploadHeaders(in)
	op.Headers = h
	op.S3UnsignedPayload = true
	if nilValue(reader) && size == 0 {
		reader = strings.NewReader("")
	}
	count := &countedReader{reader: io.LimitReader(reader, max(size, 0)), ctx: ctx}
	v, e := core.DoResponse(s.runtime, ctx, op, core.Request{Scope: s.scope, Path: objectWirePath(r), BodyStream: count, BodySize: size, Transfer: true, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if encode != nil {
			return encode
		}
		if nilValue(reader) || size < 0 {
			return types.NewError(types.InvalidArgument)
		}
		return s.validObject(r)
	}}, func(call context.Context, b io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[Transfer], error) {
		out := Transfer{Object: ObjectInfo{Ref: &r}, Bytes: count.n.Load()}
		if e := s.headerLimit(h); e != nil {
			return core.Decoded[Transfer]{Data: out}, e
		}
		out.Object = mutationHeaders(h, r)
		var probe [1]byte
		n, e := b.Read(probe[:])
		if n != 0 || (e != nil && e != io.EOF) || out.Bytes != size {
			return core.Decoded[Transfer]{Data: out}, types.NewError(types.ProtocolError)
		}
		if call.Err() != nil {
			return core.Decoded[Transfer]{Data: out}, types.ContextError(call.Err())
		}
		out.Complete = true
		return core.Decoded[Transfer]{Data: out}, nil
	})
	v.Data.Bytes = count.n.Load()
	if v.Data.Object.Ref == nil && s.validObject(r) == nil {
		v.Data.Object.Ref = &r
	}
	if e != nil {
		v.Data.Complete = false
	}
	return v, e
}
func (s *Client) headerLimit(h http.Header) error {
	for _, key := range []string{"Content-Length", "X-Amz-Delete-Marker"} {
		values := h.Values(key)
		if len(values) > 1 {
			return types.NewError(types.ProtocolError)
		}
		if len(values) == 1 {
			if key == "Content-Length" {
				if _, e := types.ParseInteger(values[0]); e != nil || strings.HasPrefix(values[0], "-") {
					return types.NewError(types.ProtocolError)
				}
			} else if values[0] != "true" && values[0] != "false" {
				return types.NewError(types.ProtocolError)
			}
		}
	}

	var n uint64
	for k, values := range h {
		n += uint64(len(k))
		for _, v := range values {
			n += uint64(len(v))
		}
		if n > s.runtime.StructuredLimit() {
			return types.NewError(types.ResponseLimit)
		}
	}
	return nil
}

type directedWriter struct {
	writer io.Writer
	ctx    context.Context
}

func (w directedWriter) Write(p []byte) (int, error) {
	if w.ctx.Err() != nil {
		return 0, w.ctx.Err()
	}
	return w.writer.Write(p)
}
func (c *Objects) Download(ctx context.Context, r types.ResourceRef, writer io.Writer, in DownloadOptions, o types.CallOptions) (types.Result[Transfer], error) {
	s := c.client
	op, bind := s.callOp("objects.get", "GET", 200, 206, 304)
	op = addressOperation(op, r)
	q := url.Values{}
	if in.VersionID != nil {
		q.Set("versionId", string(in.VersionID.Reveal()))
	}
	var encode error
	for _, v := range []struct {
		k string
		v *string
	}{{"Range", in.Range}, {"If-Match", in.IfMatch}, {"If-None-Match", in.IfNoneMatch}} {
		if v.v != nil {
			if e := header(op.Headers, v.k, *v.v); e != nil {
				encode = e
			}
		}
	}
	return core.DoResponse(s.runtime, ctx, op, core.Request{Scope: s.scope, Path: objectWirePath(r), Query: q, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Transfer: true, Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if encode != nil {
			return encode
		}
		if in.MaxBytes <= 0 || nilValue(writer) {
			return types.NewError(types.InvalidArgument)
		}
		return s.validObject(r)
	}}, func(call context.Context, b io.Reader, _ types.EndpointBinding, status int, h http.Header) (core.Decoded[Transfer], error) {
		out := Transfer{Object: ObjectInfo{Ref: &r}}
		d := core.Decoded[Transfer]{Data: out, PartialRead: true}
		if e := s.headerLimit(h); e != nil {
			return d, e
		}
		d.Data.Object = objectHeaders(h, r)
		if status == 304 {
			d.Data.Complete = true
			d.Data.NotModified = true
			return d, nil
		}
		n, e := io.Copy(directedWriter{writer, call}, io.LimitReader(b, in.MaxBytes))
		d.Data.Bytes = n
		if e != nil {
			return d, types.NewError(types.TransportError)
		}
		var probe [1]byte
		more, e := b.Read(probe[:])
		if more > 0 {
			return d, types.NewError(types.ResponseLimit)
		}
		if e != nil && e != io.EOF {
			return d, types.NewError(types.TransportError)
		}
		if length := d.Data.Object.ContentLength; length != nil && length.String() != strconv.FormatInt(n, 10) {
			return d, types.NewError(types.ProtocolError)
		}
		d.Data.Complete = true
		return d, nil
	})
}
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

func mutationHeaders(h http.Header, r types.ResourceRef) ObjectInfo {
	full := objectHeaders(h, r)
	return ObjectInfo{Ref: &r, ETag: full.ETag, VersionID: full.VersionID, DeleteMarker: full.DeleteMarker, SourceVersionID: full.SourceVersionID}
}
