package objectstorage

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

func (c *LogExports) Ref(id string) (types.ResourceRef, error) {
	if id == "" {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.client.runtime.BindDocumented(operationBindings["log_exports.get"], c.client.scope)
	if e != nil {
		return types.ResourceRef{}, e
	}
	return *core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.client.scope, APIGeneration: "v2", Audience: "selectel"}, "object_storage", "log_export", id), nil
}
func exportBody(in CreateExportOptions) ([]byte, error) {
	if e := in.validate(); e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	body := map[string]any{}
	if in.Type != nil {
		body["type"] = *in.Type
	}
	if in.Data != nil {
		raw, e := json.Marshal(in.Data)
		if e != nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		var data map[string]json.RawMessage
		_ = json.Unmarshal(raw, &data)
		if in.Data.Filters != nil {
			filter := in.Data.Filters.Reveal()
			var fields map[string]json.RawMessage
			if json.Unmarshal(filter, &fields) != nil || fields == nil {
				return nil, types.NewError(types.InvalidArgument)
			}
			data["filters"] = json.RawMessage(filter)
		}
		body["data"] = data
	}
	return json.Marshal(body)
}
func exportCall(ctx context.Context, c *Client, id, path string, ref *types.ResourceRef, body []byte, encode error, o types.CallOptions) (types.Result[ExportResponse], error) {
	op, bind := c.runtime.BindDocumented(operationBindings[id], c.scope)
	request := core.Request{Scope: c.scope, Path: path, Body: body, Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if encode != nil {
			return encode
		}
		if ref != nil {
			return c.validateRef(*ref, "log_export")
		}
		return nil
	}}
	if ref != nil {
		request.Refs = []types.ResourceRef{*ref}
		request.PinnedEndpoint = &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: "v2", Audience: "selectel"}
	}
	return core.DoJSON(c.runtime, ctx, op, request, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[ExportResponse], error) {
		var identity struct {
			Task *struct {
				ID *string `json:"id"`
			} `json:"task"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Task == nil {
			return core.Decoded[ExportResponse]{}, types.NewError(types.ProtocolError)
		}
		known := ref
		if identity.Task.ID != nil {
			if *identity.Task.ID == "" {
				return core.Decoded[ExportResponse]{}, types.NewError(types.ProtocolError)
			}
			if ref != nil && *identity.Task.ID != ref.ID {
				return core.Decoded[ExportResponse]{}, types.NewError(types.ScopeMismatch)
			}
			if ref == nil {
				known = core.Ref(endpoint, "object_storage", "log_export", *identity.Task.ID)
			}
		}
		var out ExportResponse
		decoded := core.Decoded[ExportResponse]{}
		if known != nil {
			decoded.Refs = []types.ResourceRef{*known}
			decoded.Data.Ref = known
		}
		if json.Unmarshal(raw, &out) != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		if e := out.validate(); e != nil {
			return decoded, e
		}
		out.Ref = known
		decoded.Data = out
		return decoded, nil
	})
}
func (c *LogExports) create(ctx context.Context, in CreateExportOptions, o types.CallOptions) (types.Result[ExportResponse], error) {
	body, e := exportBody(in)
	return exportCall(ctx, c.client, "log_exports.create", "/v2/logs", nil, body, e, o)
}
func (c *LogExports) get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[ExportResponse], error) {
	return exportCall(ctx, c.client, "log_exports.get", "/v2/logs/"+url.PathEscape(r.ID), &r, nil, nil, o)
}
