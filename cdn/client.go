// Package cdn binds the current documented project-scoped CDN v3 API.
package cdn

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client { return &Client{r, s} }
func (c *Client) operation(id, method string) (core.Operation, error) {
	effect := "read"
	if method != "GET" {
		effect = "mutation"
	}
	return c.runtime.BindDocumented(core.Operation{ID: id, Service: "cdn", APIGeneration: "v3", Method: method, Effect: effect, Auth: types.IAMProject, RequireProject: true, Success: []int{200}, Rejected: operationRejections[id]}, c.scope)
}

var resourceID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (c *Client) Resource(id string) (types.ResourceRef, error) {
	if !resourceID.MatchString(id) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("cdn.v3.resources.get", "GET")
	if e != nil {
		return types.ResourceRef{}, e
	}
	return *core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, APIGeneration: "v3", Scope: c.scope}, "cdn", "resource", id), nil
}
func (c *Client) refError(ref types.ResourceRef, kind string) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Service != "cdn" || ref.APIGeneration != "v3" || ref.Scope != c.scope || ref.Kind != kind || ref.Parent != nil {
		return types.NewError(types.ScopeMismatch)
	}
	if kind == "resource" {
		if !resourceID.MatchString(ref.ID) {
			return types.NewError(types.InvalidArgument)
		}
	} else if kind == "cache_task" {
		if !opaqueID(ref.ID) {
			return types.NewError(types.InvalidArgument)
		}
	} else if kind == "certificate" {
		if _, e := types.ParseInteger(ref.ID); e != nil {
			return types.NewError(types.InvalidArgument)
		}
	} else {
		return types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("cdn.v3.resources.get", "GET")
	if e != nil {
		return e
	}
	expected := types.ResourceRef{EndpointID: op.EndpointID, EndpointURL: op.FixedEndpoint}
	if expected.EndpointID != ref.EndpointID || expected.EndpointURL != ref.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func pin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func (c *Client) GetResource(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Resource], error) {
	op, bindErr := c.operation("cdn.v3.resources.get", "GET")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/resources/" + url.PathEscape(ref.ID), PinnedEndpoint: pin(ref), Refs: []types.ResourceRef{ref}, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref, "resource")
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[Resource], error) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return core.Decoded[Resource]{}, types.NewError(types.ProtocolError)
		}
		if wire, ok := fields["id"]; ok {
			var id string
			if json.Unmarshal(wire, &id) != nil || id == "" {
				return core.Decoded[Resource]{}, types.NewError(types.ProtocolError)
			}
			if id != ref.ID {
				return core.Decoded[Resource]{}, types.NewError(types.ScopeMismatch)
			}
		}
		var out Resource
		if json.Unmarshal(raw, &out) != nil {
			return core.Decoded[Resource]{}, types.NewError(types.ProtocolError)
		}
		known := ref
		out.Ref = &known
		return core.Decoded[Resource]{Data: out}, out.Validate()
	})
}
