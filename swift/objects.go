package swift

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func relativeObject(ref types.ResourceRef, leading bool) string {
	if ref.Parent == nil {
		return ""
	}
	out := url.PathEscape(ref.Parent.ID) + "/" + url.PathEscape(ref.ID)
	if leading {
		out = "/" + out
	}
	return out
}
func objectEqual(a, b types.ResourceRef) bool {
	return a.ID == b.ID && a.Parent != nil && b.Parent != nil && a.Parent.ID == b.Parent.ID && a.Scope == b.Scope && a.EndpointID == b.EndpointID && a.EndpointURL == b.EndpointURL
}
func (c *Client) objectControl(ctx context.Context, id, method string, source *types.ResourceRef, target types.ResourceRef, extra []types.ResourceRef, h http.Header, body []byte, query url.Values, status []int, o types.CallOptions, validate func() error) (types.Result[ObjectInfo], error) {
	refs := []types.ResourceRef{target}
	if source != nil {
		refs = append(refs, *source)
	}
	refs = append(refs, extra...)
	return invoke(c, ctx, id, method, refPath(target), status, refs, h, body, query, o, func() error {
		if e := c.refError(target, "object"); e != nil {
			return e
		}
		if source != nil {
			if e := c.refError(*source, "object"); e != nil {
				return e
			}
		}
		if validate != nil {
			return validate()
		}
		return nil
	}, func(_ context.Context, r io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[ObjectInfo], error) {
		known := cloneRef(target)
		out, e := objectInfo(h, &known)
		if e == nil && source != nil {
			if copied := value(h, "X-Copied-From"); copied != nil {
				decoded, x := url.PathUnescape(strings.TrimPrefix(*copied, "/"))
				if x != nil {
					e = types.NewError(types.ProtocolError)
				} else if decoded != source.Parent.ID+"/"+source.ID {
					e = types.NewError(types.ScopeMismatch)
				}
			}
		}
		if e == nil {
			e = c.discard(r)
		}
		return core.Decoded[ObjectInfo]{Data: out}, e
	})
}

// Copy uses the source-addressed COPY primitive and an explicit destination.
func (c *Client) Copy(ctx context.Context, source, target types.ResourceRef, spec CopyOptions, o types.CallOptions) (types.Result[ObjectInfo], error) {
	h, e := metadataRequest("X-Object-Meta-", spec.Metadata, nil)
	h.Set("Destination", relativeObject(target, true))
	setBoolean(h, "X-Fresh-Metadata", spec.FreshMetadata)
	// COPY addresses the source, unlike PUT X-Copy-From. Both refs are retained.
	refs := []types.ResourceRef{source, target}
	return invoke(c, ctx, "swift.v1.objects.copy", "COPY", refPath(source), []int{201}, refs, h, nil, nil, o, func() error {
		if e != nil {
			return e
		}
		if x := c.refError(source, "object"); x != nil {
			return x
		}
		return c.refError(target, "object")
	}, func(_ context.Context, r io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[ObjectInfo], error) {
		known := cloneRef(target)
		out, e := objectInfo(h, &known)
		if e == nil {
			if from := value(h, "X-Copied-From"); from != nil {
				decoded, x := url.PathUnescape(strings.TrimPrefix(*from, "/"))
				if x != nil {
					e = types.NewError(types.ProtocolError)
				} else if decoded != source.Parent.ID+"/"+source.ID {
					e = types.NewError(types.ScopeMismatch)
				}
			}
		}
		if e == nil {
			e = c.discard(r)
		}
		return core.Decoded[ObjectInfo]{Data: out}, e
	})
}
func (c *Client) CopyFrom(ctx context.Context, source, target types.ResourceRef, spec UploadOptions, o types.CallOptions) (types.Result[ObjectInfo], error) {
	h, e := uploadHeaders(spec)
	h.Set("X-Copy-From", relativeObject(source, true))
	return c.objectControl(ctx, "swift.v1.objects.put", "PUT", &source, target, nil, h, nil, nil, []int{201}, o, func() error { return e })
}
func (c *Client) PutDynamicManifest(ctx context.Context, target, segmentsBucket types.ResourceRef, prefix string, spec UploadOptions, o types.CallOptions) (types.Result[ObjectInfo], error) {
	h, e := uploadHeaders(spec)
	h.Set("X-Object-Manifest", url.PathEscape(segmentsBucket.ID)+"/"+url.PathEscape(prefix))
	return c.objectControl(ctx, "swift.v1.objects.put", "PUT", nil, target, []types.ResourceRef{segmentsBucket}, h, nil, nil, []int{201}, o, func() error {
		if e != nil {
			return e
		}
		if x := c.refError(segmentsBucket, "bucket"); x != nil {
			return x
		}
		if prefix == "" || len(prefix) > 1024 || !utf8.ValidString(prefix) || strings.ContainsAny(prefix, "\r\n\x00") {
			return types.NewError(types.InvalidArgument)
		}
		if target.Parent != nil && target.Parent.ID == segmentsBucket.ID && strings.HasPrefix(target.ID, prefix) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	})
}
func (c *Client) PutStaticManifest(ctx context.Context, target types.ResourceRef, segments []Segment, spec UploadOptions, o types.CallOptions) (types.Result[ObjectInfo], error) {
	h, encodeErr := uploadHeaders(spec)
	h.Set("X-Static-Large-Object", "True")
	h.Set("Content-Type", "application/json")
	type row struct {
		Path string         `json:"path"`
		ETag *string        `json:"etag,omitempty"`
		Size *types.Integer `json:"size_bytes,omitempty"`
	}
	wire := make([]row, 0, min(len(segments), 1000))
	refs := []types.ResourceRef{}
	var raw []byte
	validate := func() error {
		if encodeErr != nil {
			return encodeErr
		}
		if len(segments) < 1 || len(segments) > 1000 {
			return types.NewError(types.InvalidArgument)
		}
		for _, segment := range segments {
			if e := c.refError(segment.Object, "object"); e != nil {
				return e
			}
			if objectEqual(target, segment.Object) {
				return types.NewError(types.InvalidArgument)
			}
			if segment.SizeBytes != nil {
				n, e := types.ParseInteger(segment.SizeBytes.String())
				if e != nil || strings.HasPrefix(n.String(), "-") {
					return types.NewError(types.InvalidArgument)
				}
			}
			wire = append(wire, row{relativeObject(segment.Object, true), segment.ETag, segment.SizeBytes})
			refs = append(refs, segment.Object)
		}
		var e error
		raw, e = json.Marshal(wire)
		if e != nil {
			return types.NewError(types.InvalidArgument)
		}
		if uint64(len(raw)) > c.runtime.StructuredLimit() {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	// Prepare the typed control body before submission; preserve errors in finalizer.
	preflight := validate()
	return c.objectControl(ctx, "swift.v1.objects.put", "PUT", nil, target, refs, h, raw, url.Values{"multipart-manifest": {"put"}}, []int{201}, o, func() error { return preflight })
}
func (c *Client) PutSymlink(ctx context.Context, target, source types.ResourceRef, etag *string, o types.CallOptions) (types.Result[ObjectInfo], error) {
	h := http.Header{"X-Symlink-Target": {relativeObject(source, false)}}
	e := setText(h, "X-Symlink-Target-Etag", etag)
	return c.objectControl(ctx, "swift.v1.objects.put", "PUT", &source, target, nil, h, nil, nil, []int{201}, o, func() error {
		if e != nil {
			return e
		}
		if objectEqual(target, source) || etag != nil && *etag == "" {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	})
}
func (c *Client) SymlinkInfo(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[ObjectInfo], error) {
	return c.objectControl(ctx, "swift.v1.objects.head_symlink", "HEAD", nil, ref, nil, nil, nil, url.Values{"symlink": {"get"}}, []int{200}, o, nil)
}
func (c *Client) UpdateObject(ctx context.Context, ref types.ResourceRef, spec ObjectUpdate, o types.CallOptions) (types.Result[ObjectInfo], error) {
	h, e := metadataRequest("X-Object-Meta-", spec.Metadata, nil)
	for _, f := range []struct {
		k string
		v *types.ProtectedValue
	}{{"Cache-Control", spec.CacheControl}, {"Content-Encoding", spec.ContentEncoding}, {"Content-Type", spec.ContentType}, {"Content-Disposition", spec.ContentDisposition}, {"Link", spec.Link}} {
		if x := setProtected(h, f.k, f.v); x != nil {
			e = x
		}
	}
	for _, f := range []struct {
		k string
		v *types.Integer
	}{{"X-Delete-At", spec.DeleteAt}, {"X-Delete-After", spec.DeleteAfter}} {
		if x := setInteger(h, f.k, f.v); x != nil {
			e = x
		}
	}
	return c.objectControl(ctx, "swift.v1.objects.update", "POST", nil, ref, nil, h, nil, nil, []int{201, 202}, o, func() error { return e })
}
