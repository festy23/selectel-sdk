// Package s3 binds the confirmed Selectel S3 protocol subset with explicit scope.
package s3

import (
	"context"
	"encoding/xml"
	"github.com/aws/smithy-go/encoding/httpbinding"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/internal/endpoints"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

const generation = "v2006_03_01"

type Client struct {
	virtual bool
	runtime *core.Runtime
	scope   types.Scope
	Buckets *Buckets
	Objects *Objects
}
type Buckets struct{ client *Client }
type Objects struct{ client *Client }

func Bind(r *core.Runtime, s types.Scope) *Client {
	c := &Client{runtime: r, scope: s}
	c.Buckets = &Buckets{c}
	c.Objects = &Objects{c}
	return c
}
func operation(action, method string, status ...int) core.Operation {
	effect := "read"
	if method != "GET" && method != "HEAD" {
		effect = "mutation"
	}
	return core.Operation{ID: "s3." + generation + "." + action, Service: "s3", APIGeneration: generation, Method: method, Effect: effect, Auth: types.S3SigV4, SigV4Service: "s3", ErrorProtocol: "s3", RequireProject: true, RequireLocation: true, Success: status, Rejected: []int{400, 401, 403, 404, 409, 412, 416}, ErrorCodes: map[int]types.Code{400: types.InvalidArgument, 412: types.Conflict, 416: types.InvalidArgument}, Headers: http.Header{"Accept-Encoding": {"identity"}}}
}
func (c *Client) scopeValid() error {
	if c.scope.Validate() != nil || c.scope.ProjectID == "" || c.scope.Pool == "" {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func bucketName(s string) bool {
	if len(s) < 3 || len(s) > 63 || strings.Contains(s, "..") || s[0] == '-' || s[len(s)-1] == '-' || strings.ContainsAny(s, "/?#\\") {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789-.", c) {
			return false
		}
	}
	return true
}
func (c *Client) Bucket(name string) (types.ResourceRef, error) {
	if !bucketName(name) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	o, e := c.runtime.BindDocumented(operation("buckets.create", "PUT", 200), c.scope)
	if e != nil {
		return types.ResourceRef{}, e
	}
	ep := types.EndpointBinding{ID: o.EndpointID, URL: o.FixedEndpoint, Scope: c.scope, APIGeneration: generation, Audience: "selectel"}
	return c.bucketRef(ep, name)
}
func (c *Client) valid(r types.ResourceRef, kind string) error {
	if e := c.scopeValid(); e != nil {
		return e
	}
	if r.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if r.Scope != c.scope || r.Service != "s3" || r.APIGeneration != generation || !kindMatches(r.Kind, kind) {
		return types.NewError(types.ScopeMismatch)
	}
	if kind == "bucket_name" && (!bucketName(r.ID) || r.Parent != nil) {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func pin(r types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: r.EndpointID, URL: r.EndpointURL, Scope: r.Scope, APIGeneration: r.APIGeneration, Audience: "selectel"}
}
func (c *Client) callOp(id, method string, status ...int) (core.Operation, error) {
	return c.runtime.BindDocumented(operation(id, method, status...), c.scope)
}
func (c *Buckets) Create(ctx context.Context, r types.ResourceRef, in CreateBucketOptions, o types.CallOptions) (types.Result[BucketInfo], error) {
	s := c.client
	op, bind := s.callOp("buckets.create", "PUT", 200)
	op = addressOperation(op, r)
	var body []byte
	var encode error
	if in.LocationConstraint != nil {
		body, encode = xml.Marshal(struct {
			XMLName  xml.Name `xml:"CreateBucketConfiguration"`
			XMLNS    string   `xml:"xmlns,attr"`
			Location string   `xml:"LocationConstraint"`
		}{XMLNS: "http://s3.amazonaws.com/doc/2006-03-01/", Location: *in.LocationConstraint})
		op.Headers.Set("Content-Type", "application/xml")
	}
	if in.ObjectLockEnabled != nil {
		if *in.ObjectLockEnabled {
			op.Headers.Set("x-amz-bucket-object-lock-enabled", "true")
		} else {
			op.Headers.Set("x-amz-bucket-object-lock-enabled", "false")
		}
	}
	return core.DoResponse(s.runtime, ctx, op, core.Request{Scope: s.scope, Path: bucketPath(r), Body: body, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if encode != nil {
			return types.NewError(types.InvalidArgument)
		}
		if in.LocationConstraint != nil && *in.LocationConstraint != s.scope.Pool {
			return types.NewError(types.ScopeMismatch)
		}
		return s.valid(r, "bucket_name")
	}}, func(_ context.Context, body io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[BucketInfo], error) {
		out := core.Decoded[BucketInfo]{Data: BucketInfo{Ref: &r}}
		if location := h.Get("Location"); location != "" {
			protected := types.Protect([]byte(location))
			out.Data.CreationLocation = &protected
		}
		var p [1]byte
		if n, e := body.Read(p[:]); n != 0 || (e != nil && e != io.EOF) {
			return out, types.NewError(types.ProtocolError)
		}
		return out, nil
	})
}

func (c *Client) Object(bucket types.ResourceRef, key string) (types.ResourceRef, error) {
	if e := c.valid(bucket, "bucket_name"); e != nil {
		return types.ResourceRef{}, e
	}
	if key == "" || len(key) > 1024 || !utf8.ValidString(key) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	return types.ResourceRef{Scope: c.scope, Service: "s3", APIGeneration: generation, Kind: objectKind(bucket), ID: key, Parent: &bucket, EndpointID: bucket.EndpointID, EndpointURL: bucket.EndpointURL}, nil
}
func objectPath(r types.ResourceRef) string {
	if r.Parent == nil {
		return "/"
	}
	return "/" + url.PathEscape(r.Parent.ID) + "/" + httpbinding.EscapePath(r.ID, false)
}
func (c *Client) validObject(r types.ResourceRef) error {
	if e := c.valid(r, "object_key"); e != nil {
		return e
	}
	if r.Parent == nil {
		return types.NewError(types.InvalidArgument)
	}
	if e := c.valid(*r.Parent, "bucket_name"); e != nil {
		return e
	}
	if virtualRef(r) != virtualRef(*r.Parent) || r.Parent.EndpointID != r.EndpointID || r.Parent.EndpointURL != r.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	if r.ID == "" || len(r.ID) > 1024 || !utf8.ValidString(r.ID) {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}

func (c *Client) WithVirtualHosted() *Client {
	out := Bind(c.runtime, c.scope)
	out.virtual = true
	return out
}
func virtualRef(r types.ResourceRef) bool {
	return r.Kind == "bucket_vhost_name" || r.Kind == "object_vhost_key"
}
func kindMatches(actual, want string) bool {
	return actual == want || want == "bucket_name" && actual == "bucket_vhost_name" || want == "object_key" && actual == "object_vhost_key"
}
func objectKind(bucket types.ResourceRef) string {
	if virtualRef(bucket) {
		return "object_vhost_key"
	}
	return "object_key"
}
func (c *Client) bucketRef(ep types.EndpointBinding, name string) (types.ResourceRef, error) {
	kind := "bucket_name"
	if c.virtual {
		var e error
		ep, e = endpoints.S3VirtualHosted(ep, name)
		if e != nil {
			return types.ResourceRef{}, e
		}
		kind = "bucket_vhost_name"
	}
	return *core.Ref(ep, "s3", kind, name), nil
}
func bucketPath(r types.ResourceRef) string {
	if virtualRef(r) {
		return "/"
	}
	return "/" + url.PathEscape(r.ID)
}
func objectWirePath(r types.ResourceRef) string {
	path := objectPath(r)
	if virtualRef(r) && r.Parent != nil {
		return strings.TrimPrefix(path, "/"+url.PathEscape(r.Parent.ID))
	}
	return path
}
func addressOperation(op core.Operation, r types.ResourceRef) core.Operation {
	if virtualRef(r) {
		name := r.ID
		if r.Parent != nil {
			name = r.Parent.ID
		}
		op.S3BucketHost = name
	}
	return op
}

func (c *Client) currentEndpoint(r types.ResourceRef) error {
	op, e := c.callOp("objects.copy", "PUT", 200)
	if e != nil {
		return e
	}
	ep := types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.scope, APIGeneration: generation, Audience: "selectel"}
	if virtualRef(r) {
		name := r.ID
		if r.Parent != nil {
			name = r.Parent.ID
		}
		ep, e = endpoints.S3VirtualHosted(ep, name)
		if e != nil {
			return e
		}
	}
	if ep.ID != r.EndpointID || ep.URL != r.EndpointURL {
		return types.NewError(types.EndpointUnavailable)
	}
	return nil
}
