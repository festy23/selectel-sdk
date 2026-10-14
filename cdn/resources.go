package cdn

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) CreateResource(ctx context.Context, spec ResourceSpec, o types.CallOptions) (types.Result[CreateUpdateResponse], error) {
	op, bindErr := c.operation("cdn.v3.resources.create", "POST")
	body, encodeErr := wireBody(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/resources", Body: body, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return encodeErr
	}}, decodeResourceChange(""))
}
func decodeResourceChange(expected string) core.Decoder[CreateUpdateResponse] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[CreateUpdateResponse], error) {
		out := core.Decoded[CreateUpdateResponse]{}
		// Identity is scanned before any independent settings or status is decoded.
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return out, types.NewError(types.ProtocolError)
		}
		id := expected
		if wire, exists := fields["resource_id"]; exists {
			var observed string
			if json.Unmarshal(wire, &observed) != nil || !resourceID.MatchString(observed) {
				return out, types.NewError(types.ProtocolError)
			}
			if expected != "" && observed != expected {
				return out, types.NewError(types.ScopeMismatch)
			}
			id = observed
		}
		var known *types.ResourceRef
		if id != "" {
			known = core.Ref(ep, "cdn", "resource", id)
			out.Refs = []types.ResourceRef{*known}
		}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = known
		if out.Data.Status == nil {
			return out, types.NewError(types.ProtocolError)
		}
		switch *out.Data.Status {
		case "accept":
			if id == "" {
				return out, types.NewError(types.ProtocolError)
			}
		case "error":
			return out, types.NewError(types.OperationFailed)
		default:
			return out, types.NewError(types.StateUnknown)
		}
		// task_id here is configuration metadata, not a cache-task reference.
		return out, nil
	}
}

func (c *Client) PatchResource(ctx context.Context, ref types.ResourceRef, spec ResourceSpec, o types.CallOptions) (types.Result[CreateUpdateResponse], error) {
	return c.changeResource(ctx, ref, spec, "PATCH", "cdn.v3.resources.update", o)
}
func (c *Client) ReplaceResource(ctx context.Context, ref types.ResourceRef, spec ResourceSpec, o types.CallOptions) (types.Result[CreateUpdateResponse], error) {
	return c.changeResource(ctx, ref, spec, "PUT", "cdn.v3.resources.replace", o)
}
func (c *Client) changeResource(ctx context.Context, ref types.ResourceRef, spec ResourceSpec, method, id string, o types.CallOptions) (types.Result[CreateUpdateResponse], error) {
	op, bindErr := c.operation(id, method)
	body, encodeErr := wireBody(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/resources/" + url.PathEscape(ref.ID), Body: body, PinnedEndpoint: pin(ref), Refs: []types.ResourceRef{ref}, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if encodeErr != nil {
			return encodeErr
		}
		return c.refError(ref, "resource")
	}}, decodeResourceChange(ref.ID))
}
