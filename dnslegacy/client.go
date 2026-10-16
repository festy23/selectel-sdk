// Package dnslegacy implements the explicitly selected account-scoped DNS v1 API.
package dnslegacy

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
	Tags    *Tags
	Domains *Domains
	Records *Records
}
type Tags struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{runtime: r, scope: s, Tags: &Tags{runtime: r, scope: s}, Domains: &Domains{runtime: r, scope: s}, Records: &Records{runtime: r, scope: s}}
}

var decimalID = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)

func (c *Tags) Get(ctx context.Context, id string, o types.CallOptions) (types.Result[Tag], error) {
	return c.get(ctx, id, nil, o)
}
func (c *Tags) GetRef(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Tag], error) {
	return c.get(ctx, ref.ID, &ref, o)
}
func (c *Tags) get(ctx context.Context, id string, expected *types.ResourceRef, o types.CallOptions) (types.Result[Tag], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.tags.get", Service: "dns", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.StaticToken, Success: []int{200}}, c.scope)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if expected != nil {
			if expected.Validate() != nil {
				return types.NewError(types.InvalidArgument)
			}
			if expected.Scope != c.scope || expected.Service != "dns" || expected.APIGeneration != "v1" || expected.Kind != "tag" || expected.Parent != nil || expected.EndpointID != op.EndpointID || expected.EndpointURL != op.FixedEndpoint {
				return types.NewError(types.ScopeMismatch)
			}
		}
		if !decimalID.MatchString(id) {
			return types.NewError(types.InvalidArgument)
		}
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}
	decode := func(raw []byte, ep types.EndpointBinding) (core.Decoded[Tag], error) {
		var row Tag
		if json.Unmarshal(raw, &row) != nil || row.Validate() != nil || row.ID == nil || !decimalID.MatchString(row.ID.String()) {
			return core.Decoded[Tag]{}, types.NewError(types.ProtocolError)
		}
		if row.ID.String() != id {
			return core.Decoded[Tag]{}, types.NewError(types.ScopeMismatch)
		}
		ref := types.ResourceRef{Scope: c.scope, Service: "dns", APIGeneration: "v1", Kind: "tag", ID: id, EndpointID: ep.ID, EndpointURL: ep.URL}
		row.Ref = &ref
		return core.Decoded[Tag]{Data: row, Refs: []types.ResourceRef{ref}}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/tags/" + id, Options: o, Validate: validate}, decode)
}

func (c *Tags) Update(ctx context.Context, ref types.ResourceRef, spec TagUpdateSpec, o types.CallOptions) (types.Result[Tag], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.tags.replace", Service: "dns", APIGeneration: "v1", Method: "PUT", Effect: "mutation", Auth: types.StaticToken, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if ref.Validate() != nil || !decimalID.MatchString(ref.ID) {
			return types.NewError(types.InvalidArgument)
		}
		if ref.Scope != c.scope || ref.Service != "dns" || ref.APIGeneration != "v1" || ref.Kind != "tag" || ref.Parent != nil || ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
			return types.NewError(types.ScopeMismatch)
		}
		if spec.Name == nil || !utf8.Valid(spec.Name.Reveal()) || utf8.RuneCount(spec.Name.Reveal()) < 1 || utf8.RuneCount(spec.Name.Reveal()) > 50 || len(spec.Domains) > 30 {
			return types.NewError(types.InvalidArgument)
		}
		for _, id := range spec.Domains {
			if _, e := types.ParseInteger(id.String()); e != nil {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}
	var wire struct {
		Name    string          `json:"name"`
		Domains []types.Integer `json:"domains,omitzero"`
	}
	if spec.Name != nil {
		wire.Name = string(spec.Name.Reveal())
	}
	wire.Domains = spec.Domains
	body, bodyErr := json.Marshal(wire)
	decode := func(raw []byte, ep types.EndpointBinding) (core.Decoded[Tag], error) {
		var row Tag
		if json.Unmarshal(raw, &row) != nil || row.Validate() != nil {
			return core.Decoded[Tag]{}, types.NewError(types.ProtocolError)
		}
		if row.ID.String() != ref.ID {
			return core.Decoded[Tag]{}, types.NewError(types.ScopeMismatch)
		}
		known := ref
		row.Ref = &known
		return core.Decoded[Tag]{Data: row, Refs: []types.ResourceRef{known}}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/tags/" + ref.ID, Refs: []types.ResourceRef{ref}, Body: body, Options: o, Validate: func() error {
		if e := validate(); e != nil {
			return e
		}
		return bodyErr
	}}, decode)
}

// Acknowledgement preserves a bounded untyped success representation. It does
// not establish deletion completion or an undocumented response field contract.
type Acknowledgement struct {
	Representation *types.ProtectedJSON `json:"representation,omitempty"`
}

func (c *Tags) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Acknowledgement], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.tags.delete", Service: "dns", APIGeneration: "v1", Method: "DELETE", Effect: "mutation", Auth: types.StaticToken, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if ref.Validate() != nil || !decimalID.MatchString(ref.ID) {
			return types.NewError(types.InvalidArgument)
		}
		if ref.Scope != c.scope || ref.Service != "dns" || ref.APIGeneration != "v1" || ref.Kind != "tag" || ref.Parent != nil || ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[Acknowledgement], error) {
		ack := Acknowledgement{}
		if len(bytes.TrimSpace(raw)) > 0 {
			var value types.ProtectedJSON
			if json.Unmarshal(raw, &value) != nil {
				return core.Decoded[Acknowledgement]{}, types.NewError(types.ProtocolError)
			}
			ack.Representation = &value
		}
		return core.Decoded[Acknowledgement]{Data: ack}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/tags/" + ref.ID, Refs: []types.ResourceRef{ref}, Options: o, Validate: validate}, decode)
}
