package mobilefarm

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

func (c *Client) RemoveDevice(ctx context.Context, ref types.ResourceRef, input DeviceRemoveRequest, options types.CallOptions) (types.Result[DeviceRemoval], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_devices.delete", http.MethodDelete, "mutation", projectOnly, []int{200, 204}, []int{400, 401, 403, 404, 409, 429}, []int{204})
	body, bodyErr := wireBody(input)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSONStatus(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/devices/" + url.PathEscape(ref.ID), Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return c.validateRef(ref, "device", op)
	}}, func(raw []byte, _ types.EndpointBinding, status int) (core.Decoded[DeviceRemoval], error) {
		decoded := core.Decoded[DeviceRemoval]{Data: DeviceRemoval{HTTPStatus: status}}
		if status == 204 {
			return decoded, nil
		}
		var identity struct {
			Serial *string `json:"serial"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Serial == nil || *identity.Serial == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		if *identity.Serial != ref.ID {
			return decoded, types.NewError(types.ScopeMismatch)
		}
		var device SmallDevice
		if json.Unmarshal(raw, &device) != nil || device.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		copy := ref
		device.Ref = &copy
		decoded.Data.Device = &device
		return decoded, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

// ChangeDeviceSettings has a confirmed bodyless204; the202 response schema is
// missing and is retained as protected acceptance-only data with Incomplete.
func (c *Client) ChangeDeviceSettings(ctx context.Context, ref types.ResourceRef, input DeviceSettingsRequest, options types.CallOptions) (types.Result[UnmodeledResponse], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_devices_settings.update", http.MethodPatch, "mutation", projectOnly, []int{202, 204}, []int{400, 401, 403, 404, 405, 409}, nil)
	body, bodyErr := wireBody(input)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSONStatus(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/devices/" + url.PathEscape(ref.ID) + "/settings", Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil || input.Proxy == nil && input.Wifi == nil {
			return types.NewError(types.InvalidArgument)
		}
		if input.Proxy != nil && input.Proxy.Enable != nil && *input.Proxy.Enable && (input.Proxy.TokenFingerprint == nil || *input.Proxy.TokenFingerprint == "") {
			return types.NewError(types.InvalidArgument)
		}
		return c.validateRef(ref, "device", op)
	}}, func(raw []byte, _ types.EndpointBinding, status int) (core.Decoded[UnmodeledResponse], error) {
		decoded := core.Decoded[UnmodeledResponse]{Data: UnmodeledResponse{HTTPStatus: status}}
		if status == 204 {
			if len(bytes.TrimSpace(raw)) != 0 {
				return decoded, types.NewError(types.ProtocolError)
			}
			return decoded, nil
		}
		decoded.Data.Body = types.Protect(raw)
		return decoded, types.NewError(types.Incomplete)
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) ChangeDeviceTariff(ctx context.Context, ref types.ResourceRef, input Billing, options types.CallOptions) (types.Result[SmallDevice], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_devices_tariff.update", http.MethodPatch, "mutation", projectOnly, []int{200}, []int{400, 401, 403, 404, 409, 429}, nil)
	body, bodyErr := wireBody(input)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/devices/" + url.PathEscape(ref.ID) + "/tariff", Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return c.validateRef(ref, "device", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[SmallDevice], error) {
		var identity struct {
			Serial *string `json:"serial"`
		}
		decoded := core.Decoded[SmallDevice]{}
		if json.Unmarshal(raw, &identity) != nil || identity.Serial == nil || *identity.Serial == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		if *identity.Serial != ref.ID {
			return decoded, types.NewError(types.ScopeMismatch)
		}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		copy := ref
		decoded.Data.Ref = &copy
		return decoded, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) ReplaceDevice(ctx context.Context, ref types.ResourceRef, input DeviceReplaceRequest, options types.CallOptions) (types.Result[SmallDevice], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_devices.replace", http.MethodPut, "mutation", projectOnly, []int{200}, []int{400, 401, 403, 404, 409, 429}, nil)
	body, bodyErr := wireBody(input)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/devices/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Body: body, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return c.validateRef(ref, "device", op)
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[SmallDevice], error) {
		var identity struct {
			Serial *string `json:"serial"`
		}
		decoded := core.Decoded[SmallDevice]{}
		if json.Unmarshal(raw, &identity) != nil || identity.Serial == nil || *identity.Serial == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		returned := core.Ref(ep, "mobile_farm", "device", *identity.Serial)
		decoded.Refs = []types.ResourceRef{*returned}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.Ref = returned
		return decoded, nil
	})
	// No causal replay proof exists for a replacement with an unknown outcome.
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

// OrderDevices preserves native billing/filter fields and accepted identities;
// acceptance is not readiness and an unknown result is never replayed.
func (c *Client) OrderDevices(ctx context.Context, input DeviceOrder, options types.CallOptions) (types.Result[CreateDevicesResponse], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_devices.create", http.MethodPost, "mutation", projectOnly, []int{200}, []int{400, 401, 402, 403, 409}, nil)
	body, bodyErr := wireBody(input)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/devices", Body: body, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[CreateDevicesResponse], error) {
		decoded := core.Decoded[CreateDevicesResponse]{}
		var identity struct {
			Devices []json.RawMessage `json:"devices"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Devices == nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		seen := map[string]*types.ResourceRef{}
		var firstErr error
		for _, row := range identity.Devices {
			var device struct {
				Serial *string `json:"serial"`
			}
			if json.Unmarshal(row, &device) != nil || device.Serial == nil || *device.Serial == "" {
				if firstErr == nil {
					firstErr = types.NewError(types.ProtocolError)
				}
				continue
			}
			if seen[*device.Serial] != nil {
				if firstErr == nil {
					firstErr = types.NewError(types.ProtocolError)
				}
				continue
			}
			ref := core.Ref(ep, "mobile_farm", "device", *device.Serial)
			seen[*device.Serial] = ref
			decoded.Refs = append(decoded.Refs, *ref)
		}
		if firstErr != nil {
			return decoded, firstErr
		}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		for i := range decoded.Data.Devices {
			device := &decoded.Data.Devices[i]
			device.Ref = seen[*device.Serial]
		}
		return decoded, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
