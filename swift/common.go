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

func validName(v string) bool {
	return v != "" && len(v) <= 1024 && utf8.ValidString(v) && !strings.ContainsAny(v, "\r\n\x00") && v != "." && v != ".."
}
func cloneRef(ref types.ResourceRef) types.ResourceRef {
	if ref.Parent != nil {
		p := cloneRef(*ref.Parent)
		ref.Parent = &p
	}
	return ref
}
func (c *Client) refError(ref types.ResourceRef, kind string) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Scope != c.scope || ref.Service != "swift" || ref.APIGeneration != "v1" || ref.Kind != kind {
		return types.NewError(types.ScopeMismatch)
	}
	op, e := c.operation("swift.v1.account.get", "HEAD", []int{204})
	if e != nil {
		return e
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.ScopeMismatch)
	}
	if !validName(ref.ID) {
		return types.NewError(types.InvalidArgument)
	}
	switch kind {
	case "account":
		if ref.ID != c.scope.ProjectID || ref.Parent != nil {
			return types.NewError(types.ScopeMismatch)
		}
	case "bucket":
		if strings.Contains(ref.ID, "/") {
			return types.NewError(types.InvalidArgument)
		}
		if ref.Parent == nil {
			return types.NewError(types.InvalidArgument)
		}
		return c.refError(*ref.Parent, "account")
	case "object":
		if ref.Parent == nil {
			return types.NewError(types.InvalidArgument)
		}
		return c.refError(*ref.Parent, "bucket")
	default:
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func accountRef(ep types.EndpointBinding) *types.ResourceRef {
	return core.Ref(ep, "swift", "account", ep.Scope.ProjectID)
}
func bucketRef(ep types.EndpointBinding, name string) *types.ResourceRef {
	r := core.Ref(ep, "swift", "bucket", name)
	r.Parent = accountRef(ep)
	return r
}
func objectRef(bucket types.ResourceRef, name string) *types.ResourceRef {
	r := bucket
	r.Kind = "object"
	r.ID = name
	p := cloneRef(bucket)
	r.Parent = &p
	return &r
}

// Bucket and Object bind exact caller-selected names without I/O or ownership discovery.
func (c *Client) Bucket(name string) (types.ResourceRef, error) {
	op, e := c.operation("swift.v1.account.get", "HEAD", []int{204})
	if e != nil {
		return types.ResourceRef{}, e
	}
	if !validName(name) || strings.Contains(name, "/") {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	return *bucketRef(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, APIGeneration: "v1", Scope: c.scope, Audience: "selectel"}, name), nil
}
func (c *Client) Object(bucket types.ResourceRef, name string) (types.ResourceRef, error) {
	if e := c.refError(bucket, "bucket"); e != nil {
		return types.ResourceRef{}, e
	}
	if !validName(name) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	return *objectRef(bucket, name), nil
}
func accountPath(scope types.Scope) string { return "/" + url.PathEscape(scope.ProjectID) }
func refPath(ref types.ResourceRef) string {
	if ref.Kind == "account" {
		return accountPath(ref.Scope)
	}
	if ref.Kind == "bucket" {
		return accountPath(ref.Scope) + "/" + url.PathEscape(ref.ID)
	}
	if ref.Parent == nil {
		return accountPath(ref.Scope)
	}
	return accountPath(ref.Scope) + "/" + url.PathEscape(ref.Parent.ID) + "/" + url.PathEscape(ref.ID)
}
func pin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func invoke[T any](c *Client, ctx context.Context, id, method, path string, statuses []int, refs []types.ResourceRef, headers http.Header, body []byte, query url.Values, o types.CallOptions, validate func() error, decode core.ResponseDecoder[T]) (types.Result[T], error) {
	op, bindErr := c.operation(id, method, statuses)
	op.Headers = headers
	request := core.Request{Scope: c.scope, Path: path, Body: body, Query: query, Options: o, Refs: refs, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		for _, ref := range refs {
			if e := c.refError(ref, ref.Kind); e != nil {
				return e
			}
		}
		if validate != nil {
			return validate()
		}
		return nil
	}}
	if len(refs) > 0 {
		request.PinnedEndpoint = pin(refs[0])
	}
	return core.DoResponse(c.runtime, ctx, op, request, func(call context.Context, r io.Reader, ep types.EndpointBinding, status int, h http.Header) (core.Decoded[T], error) {
		if e := c.headerLimit(h); e != nil {
			return core.Decoded[T]{}, e
		}
		return decode(call, r, ep, status, h)
	})
}
func (c *Client) headerLimit(h http.Header) error {
	var n uint64
	for k, values := range h {
		n += uint64(len(k))
		for _, v := range values {
			n += uint64(len(v))
		}
		if n > c.runtime.StructuredLimit() {
			return types.NewError(types.ResponseLimit)
		}
	}
	return nil
}
func (c *Client) control(r io.Reader) ([]byte, error) {
	limit := c.runtime.StructuredLimit()
	if limit >= uint64(^uint64(0)>>1) {
		return nil, types.NewError(types.ResponseLimit)
	}
	raw, e := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if e != nil {
		return nil, types.NewError(types.TransportError)
	}
	if uint64(len(raw)) > limit {
		return nil, types.NewError(types.ResponseLimit)
	}
	return raw, nil
}
func (c *Client) discard(r io.Reader) error { _, e := c.control(r); return e }
func value(h http.Header, key string) *string {
	if _, ok := h[http.CanonicalHeaderKey(key)]; !ok {
		return nil
	}
	v := h.Get(key)
	return &v
}
func protected(h http.Header, key string) *types.ProtectedValue {
	v := value(h, key)
	if v == nil {
		return nil
	}
	p := types.Protect([]byte(*v))
	return &p
}
func boolean(h http.Header, key string) (*bool, error) {
	v := value(h, key)
	if v == nil {
		return nil, nil
	}
	var out bool
	switch strings.ToLower(*v) {
	case "true":
		out = true
	case "false":
		out = false
	default:
		return nil, types.NewError(types.ProtocolError)
	}
	return &out, nil
}
func decodeJSON(raw []byte, value any) error {
	if json.Unmarshal(raw, value) != nil {
		return types.NewError(types.ProtocolError)
	}
	return nil
}
