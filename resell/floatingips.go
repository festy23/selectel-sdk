package resell

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

type FloatingIPsClient struct{ client *Client }
type FloatingIPListOptions struct{ Detailed *string }

func (c *FloatingIPsClient) Collect(ctx context.Context, filters FloatingIPListOptions, options pagination.Options) (pagination.Collection[FloatingIP], error) {
	cl := c.client
	op := allocationOperation("floatingips", "list", "GET")
	query := url.Values{}
	if filters.Detailed != nil {
		query.Set("detailed", *filters.Detailed)
	}
	validate := func(v *FloatingIP, endpoint types.EndpointBinding) error {
		if cl.scope.ProjectID != "" && v.ProjectID != nil && *v.ProjectID != cl.scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(endpoint, "projects", "resell_floatingip", *v.ID)
		}
		return nil
	}
	it, err := core.FixedIterator(cl.runtime, op, core.Request{Scope: cl.scope, Path: "/v2/floatingips", Query: query, Validate: cl.validateAccount}, filters, options, "floatingips", validate)
	if err != nil {
		return pagination.Collection[FloatingIP]{Items: []FloatingIP{}, StopReason: "error"}, err
	}
	return it.Collect(ctx)
}

func (c *FloatingIPsClient) Get(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[FloatingIP], error) {
	cl := c.client
	op := cl.allocationOperation("floatingips", "get", "GET")
	validate := func() error {
		if e := cl.validateAllocation(); e != nil {
			return e
		}
		if ref.Kind != "resell_floatingip" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[FloatingIP], error) {
		var envelope struct {
			FloatingIP *FloatingIP `json:"floatingip"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.FloatingIP == nil {
			return core.Decoded[FloatingIP]{}, types.NewError(types.ProtocolError)
		}
		v := envelope.FloatingIP
		if cl.scope.ProjectID != "" && v.ProjectID != nil && *v.ProjectID != cl.scope.ProjectID {
			return core.Decoded[FloatingIP]{}, types.NewError(types.ScopeMismatch)
		}
		if v.ID != nil && *v.ID != ref.ID {
			return core.Decoded[FloatingIP]{}, types.NewError(types.ScopeMismatch)
		}
		if e := v.Validate(); e != nil {
			return core.Decoded[FloatingIP]{}, e
		}
		v.Ref = core.Ref(endpoint, "projects", "resell_floatingip", ref.ID)
		return core.Decoded[FloatingIP]{Data: *v, Refs: []types.ResourceRef{*v.Ref}}, nil
	}
	pin := &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/floatingips/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}

func (c *FloatingIPsClient) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	cl := c.client
	op := cl.allocationOperation("floatingips", "delete", "DELETE")
	validate := func() error {
		if e := cl.validateAllocation(); e != nil {
			return e
		}
		if ref.Kind != "resell_floatingip" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		if len(bytes.TrimSpace(raw)) != 0 {
			return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[struct{}]{}, nil
	}
	pin := &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/floatingips/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}

// FloatingIPCreateRequest permits omitted body and quantity as published by Swagger
// and the official SDK respectively. No provider default is manufactured.
type FloatingIPCreateRequest struct {
	ProjectID   string
	FloatingIPs []FloatingIPSpec
}

func (c *FloatingIPsClient) Create(ctx context.Context, request FloatingIPCreateRequest, options types.CallOptions) (types.Result[[]FloatingIP], error) {
	cl := c.client
	op := cl.allocationOperation("floatingips_projects", "create", "POST")
	var body []byte
	if request.FloatingIPs != nil {
		body, _ = json.Marshal(struct {
			FloatingIPs []FloatingIPSpec `json:"floatingips"`
		}{request.FloatingIPs})
	}
	validate := func() error {
		if e := cl.validateAllocation(); e != nil {
			return e
		}
		if cl.scope.ProjectID != "" && request.ProjectID != cl.scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if request.ProjectID == "" {
			return types.NewError(types.InvalidArgument)
		}
		for _, spec := range request.FloatingIPs {
			if spec.Region == nil || *spec.Region == "" || (spec.Quantity != nil && !int32Value(spec.Quantity, -2147483648)) {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}
	decode := decodeAllocations("floatingip", "floatingips", "resell_floatingip", request.ProjectID, false, func(v *FloatingIP, ref *types.ResourceRef) { v.Ref = ref }, FloatingIP.Validate)
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/floatingips/projects/" + segment(request.ProjectID), Body: body, Options: options, Validate: validate}, decode)
}
