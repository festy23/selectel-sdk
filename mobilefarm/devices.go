package mobilefarm

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

func (c *Client) validateRef(ref types.ResourceRef, kind string, op core.Operation) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Scope != c.scope || ref.Service != "mobile_farm" || ref.APIGeneration != op.APIGeneration {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != kind || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.EndpointUnavailable)
	}
	return nil
}
func (c *Client) GetDevice(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[Device], error) {
	op := core.Operation{ID: "mobile_farm.v3.api_devices.get", Service: "mobile_farm", APIGeneration: "v3", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, Success: []int{200}, ErrorCodes: map[int]types.Code{400: types.InvalidArgument, 401: types.Unauthenticated, 403: types.PermissionDenied}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/devices/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRef(ref, "device", op)
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[Device], error) {
		var v Device
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Serial == nil || *v.Serial == "" {
			return core.Decoded[Device]{}, types.NewError(types.ProtocolError)
		}
		if *v.Serial != ref.ID {
			return core.Decoded[Device]{}, types.NewError(types.ScopeMismatch)
		}
		v.Ref = core.Ref(ep, "mobile_farm", "device", *v.Serial)
		return core.Decoded[Device]{Data: v}, nil
	})
}

func (c *Client) ListDevices(ctx context.Context, options pagination.Options) (pagination.Collection[SmallDevice], error) {
	op := core.Operation{ID: "mobile_farm.v3.api_devices.list", Service: "mobile_farm", APIGeneration: "v3", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, Success: []int{200}, ErrorCodes: map[int]types.Code{401: types.Unauthenticated, 403: types.PermissionDenied}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[SmallDevice]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/api/v3/devices"}, nil, options, "", func(v *SmallDevice, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.Serial == nil || *v.Serial == "" {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "mobile_farm", "device", *v.Serial)
		return nil
	})
	if e != nil {
		return pagination.Collection[SmallDevice]{}, e
	}
	return iterator.Collect(ctx)
}
