package dnslegacy

import (
	"context"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strconv"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type ExportOptions struct{ MaxBytes int64 }

// ExportProgress measures bytes accepted by the caller-owned writer. Complete
// means the complete response reached that writer, not durable storage or DNS readiness.
type ExportProgress struct {
	Bytes    int64 `json:"bytes"`
	Complete bool  `json:"complete"`
}
type exportReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r exportReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	return r.reader.Read(p)
}

type exportWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w exportWriter) Write(p []byte) (int, error) {
	if w.ctx.Err() != nil {
		return 0, w.ctx.Err()
	}
	return w.writer.Write(p)
}
func nilWriter(writer io.Writer) bool {
	if writer == nil {
		return true
	}
	v := reflect.ValueOf(writer)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return v.IsNil()
	}
	return false
}

// Export discloses BIND zone contents only into the explicitly supplied writer.
// The SDK closes the response body exactly once and never closes the writer.
func (c *Domains) Export(ctx context.Context, selector DomainSelector, writer io.Writer, spec ExportOptions, o types.CallOptions) (types.Result[ExportProgress], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.export.list", Service: "dns", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.StaticToken, Success: []int{200}, Headers: http.Header{"Accept": []string{"text/plain"}}}, c.scope)
	path, selectorErr := selector.path()
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if selectorErr != nil {
			return selectorErr
		}
		if spec.MaxBytes <= 0 || nilWriter(writer) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(call context.Context, reader io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[ExportProgress], error) {
		out := core.Decoded[ExportProgress]{PartialRead: true}
		values := h.Values("Content-Type")
		if len(values) > 1 {
			return out, types.NewError(types.ProtocolError)
		}
		contentType := h.Get("Content-Type")
		if uint64(len(contentType)) > c.runtime.StructuredLimit() {
			return out, types.NewError(types.ResponseLimit)
		}
		if contentType != "" {
			kind, _, e := mime.ParseMediaType(contentType)
			if e != nil || kind != "text/plain" {
				return out, types.NewError(types.ProtocolError)
			}
		}
		reader = exportReader{call, reader}
		n, e := io.Copy(exportWriter{call, writer}, io.LimitReader(reader, spec.MaxBytes))
		out.Data.Bytes = n
		if e != nil {
			return out, types.NewError(types.TransportError)
		}
		var probe [1]byte
		more, e := io.ReadFull(reader, probe[:])
		if more > 0 {
			return out, types.NewError(types.ResponseLimit)
		}
		if e != io.EOF {
			return out, types.NewError(types.TransportError)
		}
		if length := h.Get("Content-Length"); length != "" {
			if uint64(len(length)) > c.runtime.StructuredLimit() {
				return out, types.NewError(types.ResponseLimit)
			}
			expected, e := strconv.ParseInt(length, 10, 64)
			if e != nil || expected < 0 || expected != n {
				return out, types.NewError(types.ProtocolError)
			}
		}
		if call.Err() != nil {
			return out, types.ContextError(call.Err())
		}
		out.Data.Complete = true
		out.PartialRead = false
		return out, nil
	}
	return core.DoResponse(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/export", Options: o, Transfer: true, Validate: validate}, decode)
}
