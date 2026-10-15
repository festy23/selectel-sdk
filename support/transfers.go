package support

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"reflect"
	"sync/atomic"
	"unicode/utf8"
)

type UploadOptions struct {
	Filename, ContentType *types.ProtectedValue
	ActorUUID             *string
}
type UploadResult struct {
	File     AttachmentUploadFileResponse `json:"file"`
	Bytes    int64                        `json:"bytes"`
	Complete bool                         `json:"complete"`
}
type uploadReader struct {
	reader io.Reader
	bytes  atomic.Int64
	ctx    context.Context
}

func (r *uploadReader) Read(p []byte) (int, error) {
	if r.ctx != nil && r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	n, e := r.reader.Read(p)
	r.bytes.Add(int64(n))
	return n, e
}
func missingReader(r io.Reader) bool {
	if r == nil {
		return true
	}
	v := reflect.ValueOf(r)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return v.IsNil()
	}
	return false
}
func multipartParts(spec UploadOptions) ([]byte, []byte, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	params := map[string]string{"name": "data"}
	if spec.Filename != nil {
		value := spec.Filename.Reveal()
		if !utf8.Valid(value) || len(value) > 4096 {
			return nil, nil, "", types.NewError(types.InvalidArgument)
		}
		params["filename"] = string(value)
	}
	header := textproto.MIMEHeader{"Content-Disposition": {mime.FormatMediaType("form-data", params)}}
	if spec.ContentType != nil {
		value := spec.ContentType.Reveal()
		if !utf8.Valid(value) || len(value) > 4096 {
			return nil, nil, "", types.NewError(types.InvalidArgument)
		}
		if _, _, e := mime.ParseMediaType(string(value)); e != nil {
			return nil, nil, "", types.NewError(types.InvalidArgument)
		}
		header.Set("Content-Type", string(value))
	}
	if _, e := writer.CreatePart(header); e != nil {
		return nil, nil, "", types.NewError(types.InvalidArgument)
	}
	prefix := append([]byte(nil), buffer.Bytes()...)
	if e := writer.Close(); e != nil {
		return nil, nil, "", types.NewError(types.InvalidArgument)
	}
	return prefix, append([]byte(nil), buffer.Bytes()[len(prefix):]...), writer.FormDataContentType(), nil
}

// Upload consumes at most size payload bytes and never closes or probes beyond
// the caller-owned reader. Bytes reports input consumption, not server storage.
func (c *AttachmentsClient) Upload(ctx context.Context, reader io.Reader, size int64, spec UploadOptions, o types.CallOptions) (types.Result[UploadResult], error) {
	op, bindErr := c.operation("tickets.attachments_v1.files.upload", "POST")
	op.Success = []int{201}
	prefix, suffix, contentType, encodeErr := multipartParts(spec)
	op.Headers = http.Header{"Content-Type": {contentType}}
	query := url.Values{}
	if spec.ActorUUID != nil {
		query.Set("actor_uuid", *spec.ActorUUID)
	}
	if missingReader(reader) {
		reader = bytes.NewReader(nil)
		if size != 0 {
			encodeErr = types.NewError(types.InvalidArgument)
		}
	}
	count := &uploadReader{reader: io.LimitReader(reader, max(size, 0)), ctx: ctx}
	body := io.MultiReader(bytes.NewReader(prefix), count, bytes.NewReader(suffix))
	metadataSize := int64(len(prefix) + len(suffix))
	bodySize := max(size, 0) + metadataSize
	out, e := core.DoResponse(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/public/files/", Query: query, Options: o, Transfer: true, BodyStream: body, BodySize: bodySize, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if encodeErr != nil {
			return encodeErr
		}
		if size < 0 || size > math.MaxInt64-metadataSize {
			return types.NewError(types.InvalidArgument)
		}
		if spec.ActorUUID != nil && !fileUUID.MatchString(*spec.ActorUUID) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(call context.Context, body io.Reader, ep types.EndpointBinding, _ int, _ http.Header) (core.Decoded[UploadResult], error) {
		result := core.Decoded[UploadResult]{}
		limit := min(c.runtime.StructuredLimit(), uint64(math.MaxInt64-1))
		raw, e := io.ReadAll(io.LimitReader(body, int64(limit)+1))
		if e != nil {
			return result, types.NewError(types.TransportError)
		}
		if uint64(len(raw)) > limit {
			return result, types.NewError(types.ResponseLimit)
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return result, types.NewError(types.ProtocolError)
		}
		var id string
		if json.Unmarshal(fields["file_uuid"], &id) != nil || !fileUUID.MatchString(id) {
			return result, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(ep, "tickets", "file", id)
		result.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &result.Data.File) != nil {
			return result, types.NewError(types.ProtocolError)
		}
		result.Data.File.Ref = ref
		if count.bytes.Load() != size {
			return result, types.NewError(types.ProtocolError)
		}
		if call.Err() != nil {
			return result, types.ContextError(call.Err())
		}
		result.Data.Complete = true
		return result, nil
	})
	out.Data.Bytes = count.bytes.Load()
	if e != nil {
		out.Data.Complete = false
	}
	return out, e
}

type DownloadResult struct {
	Ref      *types.ResourceRef `json:"ref,omitempty"`
	Bytes    int64              `json:"bytes"`
	Complete bool               `json:"complete"`
}

// Download refuses I/O until a primary source supplies the successful response
// status/body/redirect contract. The published operation currently defines 422 only.
func (c *AttachmentsClient) Download(ctx context.Context, ref types.ResourceRef, destination io.Writer, maxBytes int64, o types.CallOptions) (types.Result[DownloadResult], error) {
	out := types.Result[DownloadResult]{Metadata: types.Metadata{Scope: c.scope, APIGeneration: "attachments_v1", EndpointID: ref.EndpointID}}
	if ctx == nil || destination == nil || maxBytes < 0 || o.IdempotencyKey != "" {
		return out, types.NewError(types.InvalidArgument)
	}
	if ctx.Err() != nil {
		return out, types.ContextError(ctx.Err())
	}
	if e := c.refError(ref); e != nil {
		return out, e
	}
	known := ref
	out.Data.Ref = &known
	e := types.NewError(types.CapabilityUnavailable)
	e.Reason = "success_contract_unconfirmed"
	return out, e
}
