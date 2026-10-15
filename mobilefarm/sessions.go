package mobilefarm

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

func (c *Client) AssignDevice(ctx context.Context, ref types.ResourceRef, input AddUserDeviceRequest, options types.CallOptions) (types.Result[UnmodeledResponse], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_users_devices.create", http.MethodPost, "mutation", projectOnly, []int{200}, []int{400, 401, 403, 404}, nil)
	body, bodyErr := wireBody(input)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/users/devices", Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if input.Serial == nil || *input.Serial != ref.ID {
			return types.NewError(types.ScopeMismatch)
		}
		return c.validateRef(ref, "device", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[UnmodeledResponse], error) {
		return core.Decoded[UnmodeledResponse]{Data: UnmodeledResponse{HTTPStatus: 200, Body: types.Protect(raw)}}, types.NewError(types.Incomplete)
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) ReleaseDevice(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[UnmodeledResponse], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_users_devices.delete", http.MethodDelete, "mutation", projectOnly, []int{200}, []int{401, 403, 404}, nil)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/users/devices/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRef(ref, "device", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[UnmodeledResponse], error) {
		return core.Decoded[UnmodeledResponse]{Data: UnmodeledResponse{HTTPStatus: 200, Body: types.Protect(raw)}}, types.NewError(types.Incomplete)
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) StopRemoteConnect(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[UnmodeledResponse], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_users_devices_remote_connect.delete", http.MethodDelete, "mutation", projectOnly, []int{200}, []int{400, 401, 403, 404}, nil)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/users/devices/" + url.PathEscape(ref.ID) + "/remote-connect", Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRef(ref, "device", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[UnmodeledResponse], error) {
		return core.Decoded[UnmodeledResponse]{Data: UnmodeledResponse{HTTPStatus: 200, Body: types.Protect(raw)}}, types.NewError(types.Incomplete)
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) StartRemoteConnect(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[StartRemoteConnectResponse], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_users_devices_remote_connect.create", http.MethodPost, "mutation", projectOnly, []int{200}, []int{400, 401, 403, 404}, nil)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/users/devices/" + url.PathEscape(ref.ID) + "/remote-connect", Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRef(ref, "device", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[StartRemoteConnectResponse], error) {
		var decoded core.Decoded[StartRemoteConnectResponse]
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		return decoded, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
