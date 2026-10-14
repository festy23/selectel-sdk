// Package mail exposes the documented Email Service management API, not SMTP sending.
package mail

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"regexp"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client { return &Client{runtime: r, scope: s} }

var resourceID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (c *Client) operation(id string) (core.Operation, error) {
	spec := operationSpecs[id]
	effect := "mutation"
	if spec.Method == "GET" {
		effect = "read"
	}
	op := core.Operation{ID: operationIDs[id], Service: "email", APIGeneration: "v1", Method: spec.Method, Effect: effect, Auth: types.IAMProject, RequireProject: true, Success: spec.Success, Rejected: spec.Rejected}
	return c.runtime.BindDocumented(op, c.scope)
}
func (c *Client) Resource(id string) (types.ResourceRef, error) {
	if !resourceID.MatchString(id) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("getResourceById")
	if e != nil {
		return types.ResourceRef{}, e
	}
	return *core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.scope, APIGeneration: "v1"}, "email", "resource", id), nil
}
func (c *Client) refError(ref types.ResourceRef) error {
	if ref.Validate() != nil || !resourceID.MatchString(ref.ID) || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Service != "email" || ref.Kind != "resource" || ref.APIGeneration != "v1" || ref.Scope != c.scope {
		return types.NewError(types.ScopeMismatch)
	}
	op, e := c.operation("getResourceById")
	if e != nil {
		return e
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (c *Client) request(ref types.ResourceRef, err error) core.Request {
	return core.Request{Scope: c.scope, Path: "/resources/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if err != nil {
			return err
		}
		return c.refError(ref)
	}}
}
func identity(row *ResourceWithCredentials, expected string, ep types.EndpointBinding) ([]types.ResourceRef, error) {
	if row == nil {
		return nil, types.NewError(types.ProtocolError)
	}
	if row.ProjectID != nil && *row.ProjectID != ep.Scope.ProjectID {
		return nil, types.NewError(types.ScopeMismatch)
	}
	id := expected
	if row.ID != nil {
		if !resourceID.MatchString(*row.ID) {
			return nil, types.NewError(types.ProtocolError)
		}
		if id != "" && *row.ID != id {
			return nil, types.NewError(types.ScopeMismatch)
		}
		id = *row.ID
	}
	if id == "" {
		return nil, types.NewError(types.ProtocolError)
	}
	row.Ref = core.Ref(ep, "email", "resource", id)
	return []types.ResourceRef{*row.Ref}, nil
}
func (c *Client) GetResource(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[GetResourceByID], error) {
	op, err := c.operation("getResourceById")
	req := c.request(ref, err)
	req.Options = o
	return core.DoJSON(c.runtime, ctx, op, req, func(raw []byte, ep types.EndpointBinding) (core.Decoded[GetResourceByID], error) {
		out := core.Decoded[GetResourceByID]{}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		refs, e := identity(out.Data.Resource, ref.ID, ep)
		out.Refs = refs
		return out, e
	})
}
