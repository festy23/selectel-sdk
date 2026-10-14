package dns

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
	"unicode/utf8"
)

type RRSets struct{ service }

func segment(id string) string {
	if id == "." || id == ".." {
		return strings.ReplaceAll(id, ".", "%2E")
	}
	return url.PathEscape(id)
}
func pin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func (s service) valid(ref types.ResourceRef, kind string) error {
	if e := s.scopeValid(); e != nil {
		return e
	}
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Scope != s.scope || ref.Service != "dns" || ref.APIGeneration != "v2" || ref.Kind != kind {
		return types.NewError(types.ScopeMismatch)
	}
	if kind == "zone" && ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	if kind == "rrset" {
		if ref.Parent == nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := s.valid(*ref.Parent, "zone"); e != nil {
			return e
		}
		if ref.Parent.EndpointID != ref.EndpointID || ref.Parent.EndpointURL != ref.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}
func recordsWire(rows []Records) ([]map[string]any, error) {
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		raw := row.Content.Reveal()
		if !utf8.Valid(raw) {
			return nil, types.NewError(types.InvalidArgument)
		}
		m := map[string]any{"content": string(raw)}
		if row.Disabled != nil {
			m["disabled"] = *row.Disabled
		}
		out[i] = m
	}
	return out, nil
}
func rrBody(spec RRSetCreate) ([]byte, error) {
	rows, e := recordsWire(spec.Records)
	if e != nil {
		return nil, e
	}
	m := map[string]any{"name": spec.Name, "ttl": spec.TTL, "type": spec.Type, "records": rows}
	if spec.Comment != nil {
		m["comment"] = spec.Comment
	}
	return json.Marshal(m)
}
func decodeRR(raw []byte, ep types.EndpointBinding, parent types.ResourceRef, expected string) (core.Decoded[RRSet], error) {
	var id struct {
		ID     string `json:"id"`
		ZoneID string `json:"zone_id"`
	}
	if json.Unmarshal(raw, &id) != nil || id.ID == "" || id.ZoneID != parent.ID || expected != "" && id.ID != expected {
		return core.Decoded[RRSet]{}, types.NewError(types.ProtocolError)
	}
	ref := core.Ref(ep, "dns", "rrset", id.ID)
	ref.Parent = &parent
	out := core.Decoded[RRSet]{Refs: []types.ResourceRef{*ref}}
	var v RRSet
	if json.Unmarshal(raw, &v) != nil {
		return out, types.NewError(types.ProtocolError)
	}
	v.Ref = ref
	out.Data = v
	return out, nil
}
func (c *RRSets) Create(ctx context.Context, parent types.ResourceRef, spec RRSetCreate, o types.CallOptions) (types.Result[RRSet], error) {
	return c.create(ctx, parent, spec, "", o)
}
func (c *RRSets) CreateNative(ctx context.Context, parent types.ResourceRef, spec RRSetCreate, managedBy string, o types.CallOptions) (types.Result[RRSet], error) {
	return c.create(ctx, parent, spec, managedBy, o)
}
func (c *RRSets) create(ctx context.Context, parent types.ResourceRef, spec RRSetCreate, managedBy string, o types.CallOptions) (types.Result[RRSet], error) {
	body, e := rrBody(spec)
	if e == nil && managedBy != "" {
		body, e = withNativeManagedBy(body, managedBy)
	}
	return core.DoJSON(c.runtime, ctx, op("rrsets", "create", "POST", 200), core.Request{Scope: c.scope, Path: "/zones/" + segment(parent.ID) + "/rrset", Body: body, Refs: []types.ResourceRef{parent}, PinnedEndpoint: pin(parent), Options: o, Validate: func() error {
		if e != nil {
			return types.NewError(types.InvalidArgument)
		}
		if !fqdn(spec.Name) || !requestRecordTypes[spec.Type] {
			return types.NewError(types.InvalidArgument)
		}
		if e := validateRR(spec.TTL, spec.Records, spec.Comment); e != nil {
			return e
		}
		return c.valid(parent, "zone")
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[RRSet], error) {
		return decodeRR(raw, ep, parent, "")
	})
}

// RRSetOptions preserves the current API's optional management header.
// Nil means absent; OpenAPI null on an HTTP header has the same wire absence.
type RRSetOptions struct {
	types.CallOptions
	ManagedBy *string
}

func (c *RRSets) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[RRSet], error) {
	path := "/invalid"
	if ref.Parent != nil {
		path = "/zones/" + segment(ref.Parent.ID) + "/rrset/" + segment(ref.ID)
	}
	return core.DoJSON(c.runtime, ctx, op("rrsets", "get", "GET", 200), core.Request{Scope: c.scope, Path: path, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o, Validate: func() error { return c.valid(ref, "rrset") }}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[RRSet], error) {
		return decodeRR(raw, ep, *ref.Parent, ref.ID)
	})
}
func (c *RRSets) Update(ctx context.Context, ref types.ResourceRef, spec RRSetUpdate, o RRSetOptions) (types.Result[Empty], error) {
	rows, e := recordsWire(spec.Records)
	m := map[string]any{"ttl": spec.TTL, "records": rows}
	if spec.Comment != nil {
		m["comment"] = spec.Comment
	}
	body, encodeErr := json.Marshal(m)
	if e == nil {
		e = encodeErr
	}
	return c.change(ctx, ref, "update", "PATCH", body, o, func() error {
		if e != nil {
			return types.NewError(types.InvalidArgument)
		}
		return validateRR(spec.TTL, spec.Records, spec.Comment)
	})
}
func (c *RRSets) Delete(ctx context.Context, ref types.ResourceRef, o RRSetOptions) (types.Result[Empty], error) {
	return c.change(ctx, ref, "delete", "DELETE", nil, o, nil)
}
func (c *RRSets) change(ctx context.Context, ref types.ResourceRef, action, method string, body []byte, o RRSetOptions, validate func() error) (types.Result[Empty], error) {
	operation := op("rrsets", action, method, 204)
	if o.ManagedBy != nil {
		operation.Headers = map[string][]string{"X-Managed-By": {*o.ManagedBy}}
	}
	path := "/invalid"
	if ref.Parent != nil {
		path = "/zones/" + segment(ref.Parent.ID) + "/rrset/" + segment(ref.ID)
	}
	return core.DoJSON(c.runtime, ctx, operation, core.Request{Scope: c.scope, Path: path, Body: body, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o.CallOptions, Validate: func() error {
		if e := c.valid(ref, "rrset"); e != nil {
			return e
		}
		if o.ManagedBy != nil && !uuidPattern.MatchString(*o.ManagedBy) {
			return types.NewError(types.InvalidArgument)
		}
		if validate != nil {
			return validate()
		}
		return nil
	}}, func([]byte, types.EndpointBinding) (core.Decoded[Empty], error) { return core.Decoded[Empty]{}, nil })
}

// Native management-field variants reproduce the published domains-go v2 body
// field. They do not infer current server acceptance or fall back from headers.
func withNativeManagedBy(body []byte, managedBy string) ([]byte, error) {
	if !uuidPattern.MatchString(managedBy) {
		return nil, types.NewError(types.InvalidArgument)
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	m["managed_by"], _ = json.Marshal(managedBy)
	return json.Marshal(m)
}
func (c *RRSets) UpdateNative(ctx context.Context, ref types.ResourceRef, spec RRSetUpdate, managedBy string, o types.CallOptions) (types.Result[Empty], error) {
	rows, e := recordsWire(spec.Records)
	m := map[string]any{"ttl": spec.TTL, "records": rows}
	if spec.Comment != nil {
		m["comment"] = spec.Comment
	}
	body, encodeErr := json.Marshal(m)
	if e == nil {
		e = encodeErr
	}
	if e == nil && managedBy != "" {
		body, e = withNativeManagedBy(body, managedBy)
	}
	return c.change(ctx, ref, "update", "PATCH", body, RRSetOptions{CallOptions: o}, func() error {
		if e != nil {
			return types.NewError(types.InvalidArgument)
		}
		return validateRR(spec.TTL, spec.Records, spec.Comment)
	})
}
