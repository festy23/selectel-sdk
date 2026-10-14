// Package images implements source-bound Glance v2 metadata and membership APIs.
package images

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
	Members *Members
}
type Members struct{ client *Client }
type Empty struct{}

func Bind(r *core.Runtime, s types.Scope) *Client {
	c := &Client{runtime: r, scope: s}
	c.Members = &Members{c}
	return c
}
func invalid() error { return types.NewError(types.InvalidArgument) }
func operation(action, method string) core.Operation {
	effect := "read"
	success := 200
	if method != "GET" {
		effect = "mutation"
		if action == "images.create" {
			success = 201
		}
		if method == "DELETE" {
			success = 204
		}
	}
	op := core.Operation{ID: "images.v2." + action, Service: "images", APIGeneration: "v2", Method: method, Effect: effect, Auth: types.IAMProject, ServiceType: "image", EndpointVersionPrefix: "/v2", RequireProject: true, RequireLocation: true, Success: []int{success}, Rejected: []int{400, 401, 403, 404, 405, 406, 409, 413, 415, 422, 429}, ErrorCodes: map[int]types.Code{400: types.InvalidArgument, 405: types.InvalidArgument, 406: types.InvalidArgument, 413: types.QuotaExceeded, 415: types.InvalidArgument, 422: types.InvalidArgument}}
	if method == "PATCH" {
		op.Headers = http.Header{"Content-Type": {"application/openstack-images-v2.1-json-patch"}}
	}
	if success == 204 {
		op.NoContentSuccess = []int{204}
	}
	return op
}
func validateRef(r types.ResourceRef, kind string) error {
	if r.Validate() != nil || r.Service != "images" || r.APIGeneration != "v2" || r.Kind != kind {
		return invalid()
	}
	if kind == "image" && r.Parent != nil {
		return invalid()
	}
	if kind == "image_member" {
		if r.Parent == nil || validateRef(*r.Parent, "image") != nil {
			return invalid()
		}
		if r.Parent.Scope != r.Scope || r.Parent.EndpointID != r.EndpointID || r.Parent.EndpointURL != r.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}
func pin(r types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: r.EndpointID, URL: r.EndpointURL, Scope: r.Scope, APIGeneration: r.APIGeneration, Audience: "selectel"}
}
func decodeImage(expected string) core.Decoder[Image] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[Image], error) {
		out := core.Decoded[Image]{}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return out, types.NewError(types.ProtocolError)
		}
		var id string
		if json.Unmarshal(fields["id"], &id) == nil && id != "" {
			if expected != "" && expected != id {
				return out, types.NewError(types.ScopeMismatch)
			}
			out.Refs = []types.ResourceRef{*core.Ref(ep, "images", "image", id)}
		}
		base := map[string]json.RawMessage{}
		properties := map[string]json.RawMessage{}
		for k, v := range fields {
			if baseKeys[k] {
				base[k] = v
			} else {
				properties[k] = v
			}
		}
		body, _ := json.Marshal(base)
		var v Image
		if json.Unmarshal(body, &v) != nil || v.ID == "" {
			return out, types.NewError(types.ProtocolError)
		}
		extension, _ := json.Marshal(properties)
		_ = json.Unmarshal(extension, &v.Properties)
		v.Ref = core.Ref(ep, "images", "image", v.ID)
		out.Data = v
		return out, nil
	}
}

var imageID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func nonnegative(n *types.Integer) bool {
	if n == nil {
		return true
	}
	v, ok := new(big.Int).SetString(n.String(), 10)
	return ok && v.Sign() >= 0
}
func customKey(k string) bool {
	return k != "" && utf8.RuneCountInString(k) <= 255 && !baseKeys[k] && k != "location" && k != "deleted" && k != "deleted_at" && !strings.HasPrefix(k, "os_glance")
}
func (c *Client) Create(ctx context.Context, s CreateSpec, o types.CallOptions) (types.Result[Image], error) {
	raw, e := json.Marshal(s)
	body := map[string]json.RawMessage{}
	if e == nil {
		e = json.Unmarshal(raw, &body)
	}
	delete(body, "properties")
	for k, v := range s.Properties {
		if !customKey(k) {
			e = invalid()
			continue
		}
		body[k], _ = json.Marshal(string(v.Reveal()))
	}
	if !nonnegative(s.MinDisk) || !nonnegative(s.MinRAM) {
		e = invalid()
	}
	if s.ID != nil && !imageID.MatchString(*s.ID) {
		e = invalid()
	}
	raw, _ = json.Marshal(body)
	expected := ""
	if s.ID != nil {
		expected = *s.ID
	}
	var targets func(types.EndpointBinding) []types.ResourceRef
	if expected != "" {
		targets = func(ep types.EndpointBinding) []types.ResourceRef {
			return []types.ResourceRef{*core.Ref(ep, "images", "image", expected)}
		}
	}
	return core.DoJSON(c.runtime, ctx, operation("images.create", "POST"), core.Request{TargetRefs: targets, Scope: c.scope, Path: "/v2/images", Body: raw, Options: o, Validate: func() error { return e }}, decodeImage(expected))
}
func (c *Client) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Image], error) {
	return core.DoJSON(c.runtime, ctx, operation("images.get", "GET"), core.Request{Scope: c.scope, Path: "/v2/images/" + url.PathEscape(r.ID), Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error { return validateRef(r, "image") }}, decodeImage(r.ID))
}
func (c *Client) Delete(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return core.DoJSON(c.runtime, ctx, operation("images.delete", "DELETE"), core.Request{Scope: c.scope, Path: "/v2/images/" + url.PathEscape(r.ID), Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error { return validateRef(r, "image") }}, func([]byte, types.EndpointBinding) (core.Decoded[Empty], error) { return core.Decoded[Empty]{}, nil })
}

// Patch accepts one top-level JSON pointer per operation. Values remain protected
// until this explicit mutation boundary; owner and reserved fields are unavailable.
type Patch struct {
	Op    string
	Path  string
	Value *types.ProtectedJSON
}

var writable = map[string]string{"name": "nullable_string", "visibility": "string", "protected": "bool", "os_hidden": "bool", "container_format": "nullable_string", "disk_format": "nullable_string", "min_disk": "integer", "min_ram": "integer", "tags": "strings"}

func pointerKey(path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		return "", invalid()
	}
	key := strings.TrimPrefix(path, "/")
	if key == "" || strings.Contains(key, "/") {
		return "", invalid()
	}
	for i := 0; i < len(key); i++ {
		if key[i] == '~' {
			if i+1 >= len(key) || (key[i+1] != '0' && key[i+1] != '1') {
				return "", invalid()
			}
			i++
		}
	}
	return strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~"), nil
}
func patchBody(ops []Patch) ([]byte, error) {
	if len(ops) == 0 {
		return nil, invalid()
	}
	rows := []map[string]json.RawMessage{}
	for _, p := range ops {
		if p.Op != "add" && p.Op != "replace" && p.Op != "remove" {
			return nil, invalid()
		}
		key, e := pointerKey(p.Path)
		if e != nil {
			return nil, e
		}
		kind, mutable := writable[key]
		if !mutable && !customKey(key) {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		row := map[string]json.RawMessage{}
		row["op"], _ = json.Marshal(p.Op)
		row["path"], _ = json.Marshal(p.Path)
		if p.Op == "remove" {
			if p.Value != nil {
				return nil, invalid()
			}
		} else {
			if p.Value == nil || !json.Valid(p.Value.Reveal()) {
				return nil, invalid()
			}
			raw := p.Value.Reveal()
			if !mutable {
				var v string
				if json.Unmarshal(raw, &v) != nil || string(raw) == "null" {
					return nil, invalid()
				}
			} else {
				switch kind {
				case "string", "nullable_string":
					var v *string
					if json.Unmarshal(raw, &v) != nil || (v == nil && kind == "string") {
						return nil, invalid()
					}
				case "bool":
					var v *bool
					if json.Unmarshal(raw, &v) != nil || v == nil {
						return nil, invalid()
					}
				case "integer":
					var v types.Integer
					if json.Unmarshal(raw, &v) != nil || !nonnegative(&v) {
						return nil, invalid()
					}
				case "strings":
					var v []string
					if json.Unmarshal(raw, &v) != nil || v == nil {
						return nil, invalid()
					}
				}
			}
			row["value"] = json.RawMessage(raw)
		}
		rows = append(rows, row)
	}
	return json.Marshal(rows)
}
func (c *Client) Update(ctx context.Context, r types.ResourceRef, p []Patch, o types.CallOptions) (types.Result[Image], error) {
	raw, e := patchBody(p)
	return core.DoJSON(c.runtime, ctx, operation("images.update", "PATCH"), core.Request{Scope: c.scope, Path: "/v2/images/" + url.PathEscape(r.ID), Body: raw, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error {
		if e != nil {
			return e
		}
		return validateRef(r, "image")
	}}, decodeImage(r.ID))
}
