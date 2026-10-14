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
	op := core.Operation{ID: "projects.v2.subnets.list", Service: "projects", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}}
	query := url.Values{}
	if filters.Detailed != nil {
		query.Set("detailed", strconv.FormatBool(*filters.Detailed))
	}
	validate := func(v *Subnet, endpoint types.EndpointBinding) error {
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(endpoint, "projects", "resell_subnet", v.ID.String())
		}
		return nil
	}
	it, err := core.FixedIterator(cl.runtime, op, core.Request{Scope: cl.scope, Path: "/v2/subnets", Query: query}, filters, options, "subnets", validate)
	if err != nil {
		return pagination.Collection[Subnet]{Items: []Subnet{}, StopReason: "error"}, err
	}
	return it.Collect(ctx)
}

func (c *SubnetsClient) Get(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[Subnet], error) {
	cl := c.client
	op := core.Operation{ID: "projects.v2.subnets.get", Service: "projects", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}, ErrorCodes: map[int]types.Code{400: types.NotFound}}
	validate := func() error {
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
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/subnets/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}

func (c *SubnetsClient) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	cl := c.client
	op := core.Operation{ID: "projects.v2.subnets.delete", Service: "projects", APIGeneration: "v2", Method: "DELETE", Effect: "mutation", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{204}, Rejected: []int{400, 401}, ErrorCodes: map[int]types.Code{400: types.NotFound}}
	validate := func() error {
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
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/subnets/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}
