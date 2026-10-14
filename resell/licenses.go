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

type LicensesClient struct{ client *Client }

func (c *LicensesClient) Collect(ctx context.Context, filters AllocationListOptions, options pagination.Options) (pagination.Collection[License], error) {
	cl := c.client
	op := cl.allocationOperation("licenses", "list", "GET")
	query := url.Values{}
	if filters.Detailed != nil {
		query.Set("detailed", strconv.FormatBool(*filters.Detailed))
	}
	validate := func(v *License, endpoint types.EndpointBinding) error {
		if cl.scope.ProjectID != "" && v.ProjectID != nil && *v.ProjectID != cl.scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(endpoint, "projects", "resell_license", v.ID.String())
		}
		return nil
	}
	it, err := core.FixedIterator(cl.runtime, op, core.Request{Scope: cl.scope, Path: "/v2/licenses", Query: query, Validate: cl.validateAllocation}, filters, options, "licenses", validate)
	if err != nil {
		return pagination.Collection[License]{Items: []License{}, StopReason: "error"}, err
	}
	return it.Collect(ctx)
}

func (c *LicensesClient) Get(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[License], error) {
	cl := c.client
	op := cl.allocationOperation("licenses", "get", "GET")
	validate := func() error {
		if e := cl.validateAllocation(); e != nil {
			return e
		}
		if ref.Kind != "resell_license" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[License], error) {
		var envelope struct {
			License *License `json:"license"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.License == nil {
			return core.Decoded[License]{}, types.NewError(types.ProtocolError)
		}
		v := envelope.License
		if cl.scope.ProjectID != "" && v.ProjectID != nil && *v.ProjectID != cl.scope.ProjectID {
			return core.Decoded[License]{}, types.NewError(types.ScopeMismatch)
		}
		if v.ID != nil && v.ID.String() != ref.ID {
			return core.Decoded[License]{}, types.NewError(types.ScopeMismatch)
		}
		if e := v.Validate(); e != nil {
			return core.Decoded[License]{}, e
		}
		v.Ref = core.Ref(endpoint, "projects", "resell_license", ref.ID)
		return core.Decoded[License]{Data: *v, Refs: []types.ResourceRef{*v.Ref}}, nil
	}
	pin := &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/licenses/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}

func (c *LicensesClient) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	cl := c.client
	op := cl.allocationOperation("licenses", "delete", "DELETE")
	validate := func() error {
		if e := cl.validateAllocation(); e != nil {
			return e
		}
		if ref.Kind != "resell_license" || ref.Parent != nil {
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
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/licenses/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}

// LicenseCreateRequest permits omitted body and quantity as published by Swagger
// and the official SDK respectively. No provider default is manufactured.
type LicenseCreateRequest struct {
	ProjectID string
	Licenses  []LicenseSpec
}

func (c *LicensesClient) Create(ctx context.Context, request LicenseCreateRequest, options types.CallOptions) (types.Result[[]License], error) {
	cl := c.client
	op := cl.allocationOperation("licenses_projects", "create", "POST")
	var body []byte
	if request.Licenses != nil {
		body, _ = json.Marshal(struct {
			Licenses []LicenseSpec `json:"licenses"`
		}{request.Licenses})
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
		for _, spec := range request.Licenses {
			if spec.Region == nil || *spec.Region == "" || spec.Type == nil || *spec.Type == "" || (spec.Quantity != nil && !int32Value(spec.Quantity, 1)) {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}
	decode := decodeAllocations("licenses", "licenses", "resell_license", request.ProjectID, true, func(v *License, ref *types.ResourceRef) { v.Ref = ref }, License.Validate)
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: "/v2/licenses/projects/" + segment(request.ProjectID), Body: body, Options: options, Validate: validate}, decode)
}
