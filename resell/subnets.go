package resell

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
)

type SubnetsClient struct{ client *Client }
type AllocationListOptions struct{ Detailed *bool }

func (c *SubnetsClient) Collect(ctx context.Context, filters AllocationListOptions, options pagination.Options) (pagination.Collection[Subnet], error) {
	cl := c.client
	op := cl.allocationOperation("subnets", "list", "GET")
	query := url.Values{}
	if filters.Detailed != nil {
		query.Set("detailed", strconv.FormatBool(*filters.Detailed))
	}
	validate := func(v *Subnet, endpoint types.EndpointBinding) error {
		if cl.scope.ProjectID != "" && v.ProjectID != nil && *v.ProjectID != cl.scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(endpoint, "projects", "resell_subnet", v.ID.String())
		}
		return nil
	}
	it, err := core.FixedIterator(cl.runtime, op, core.Request{Scope: cl.scope, Path: "/v2/subnets", Query: query, Validate: cl.validateAllocation}, filters, options, "subnets", validate)
	if err != nil {
		return pagination.Collection[Subnet]{Items: []Subnet{}, StopReason: "error"}, err
	}
	return it.Collect(ctx)
}

func (c *SubnetsClient) Get(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[Subnet], error) {
	cl := c.client
	op := cl.allocationOperation("subnets", "get", "GET")
	validate := func() error {
		if e := cl.validateAllocation(); e != nil {
			return e
		}
		if ref.Kind != "resell_subnet" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Subnet], error) {
		var envelope struct {
			Subnet *Subnet `json:"subnet"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Subnet == nil {
			return core.Decoded[Subnet]{}, types.NewError(types.ProtocolError)
		}
		v := envelope.Subnet
		if cl.scope.ProjectID != "" && v.ProjectID != nil && *v.ProjectID != cl.scope.ProjectID {
			return core.Decoded[Subnet]{}, types.NewError(types.ScopeMismatch)
		}
		if v.ID != nil && v.ID.String() != ref.ID {
			return core.Decoded[Subnet]{}, types.NewError(types.ScopeMismatch)
		}
		if e := v.Validate(); e != nil {
			return core.Decoded[Subnet]{}, e
		}
		v.Ref = core.Ref(endpoint, "projects", "resell_subnet", ref.ID)
		return core.Decoded[Subnet]{Data: *v, Refs: []types.ResourceRef{*v.Ref}}, nil
	}
	pin := &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/subnets/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}

func (c *SubnetsClient) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	cl := c.client
	op := cl.allocationOperation("subnets", "delete", "DELETE")
	validate := func() error {
		if e := cl.validateAllocation(); e != nil {
			return e
		}
		if ref.Kind != "resell_subnet" || ref.Parent != nil {
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
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/subnets/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}

// SubnetCreateRequest permits omitted body and quantity as published by Swagger
// and the official SDK respectively. No provider default is manufactured.
type SubnetCreateRequest struct {
	ProjectID string
	Subnets   []SubnetSpec
}

func (c *SubnetsClient) Create(ctx context.Context, request SubnetCreateRequest, options types.CallOptions) (types.Result[[]Subnet], error) {
	cl := c.client
	op := cl.allocationOperation("subnets_projects", "create", "POST")
	var body []byte
	if request.Subnets != nil {
		body, _ = json.Marshal(struct {
			Subnets []SubnetSpec `json:"subnets"`
		}{request.Subnets})
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
		for _, spec := range request.Subnets {
			if spec.Region == nil || *spec.Region == "" || spec.Type == nil || *spec.Type == "" || !int32Value(spec.PrefixLength, -2147483648) || (spec.Quantity != nil && !int32Value(spec.Quantity, -2147483648)) {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}
	decode := decodeAllocations("subnet", "subnets", "resell_subnet", request.ProjectID, true, func(v *Subnet, ref *types.ResourceRef) { v.Ref = ref }, Subnet.Validate)
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/subnets/projects/" + segment(request.ProjectID), Body: body, Options: options, Validate: validate}, decode)
}
