package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"unicode/utf8"
)

func protectedBody(field string, value *types.ProtectedValue, extra map[string]types.ProtectedJSON) ([]byte, error) {
	if value == nil || len(extra) != 0 || !utf8.Valid(value.Reveal()) {
		return nil, types.NewError(types.InvalidArgument)
	}
	return json.Marshal(map[string]string{field: string(value.Reveal())})
}
func decodeResource(expected string, project string) core.Decoder[ResourceWithCredentials] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[ResourceWithCredentials], error) {
		out := core.Decoded[ResourceWithCredentials]{}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return out, types.NewError(types.ProtocolError)
		}
		row := ResourceWithCredentials{}
		for key, target := range map[string]**string{"id": &row.ID, "project_id": &row.ProjectID} {
			if wire, ok := fields[key]; ok {
				if json.Unmarshal(wire, target) != nil {
					return out, types.NewError(types.ProtocolError)
				}
			}
		}
		// A transfer may acknowledge the explicitly requested target project. It does
		// not adopt a new target-scoped reference; the receipt retains caller scope.
		identityEndpoint := ep
		identityEndpoint.Scope.ProjectID = project
		refs, e := identity(&row, expected, identityEndpoint)
		if len(refs) > 0 {
			refs[0].Scope = ep.Scope
			row.Ref = core.Ref(ep, "email", "resource", refs[0].ID)
		}
		out.Refs = refs
		if e != nil {
			return out, e
		}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = row.Ref
		return out, out.Data.Validate()
	}
}
func (c *Client) CreateResource(ctx context.Context, spec CreateResourceRequest, o types.CallOptions) (types.Result[ResourceWithCredentials], error) {
	op, bindErr := c.operation("createResource")
	body, bodyErr := protectedBody("name", spec.Name, spec.Extra)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/resources", Body: body, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return bodyErr
	}}, decodeResource("", c.scope.ProjectID))
}

func (c *Client) RenameResource(ctx context.Context, ref types.ResourceRef, spec UpdateResourceRequest, o types.CallOptions) (types.Result[ResourceWithCredentials], error) {
	op, err := c.operation("changeResourceName")
	req := c.request(ref, err)
	req.Options = o
	body, bodyErr := protectedBody("name", spec.Name, spec.Extra)
	req.Body = body
	validate := req.Validate
	req.Validate = func() error {
		if e := validate(); e != nil {
			return e
		}
		return bodyErr
	}
	return core.DoJSON(c.runtime, ctx, op, req, decodeResource(ref.ID, c.scope.ProjectID))
}
func (c *Client) RotateCredentials(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[ResourceWithCredentials], error) {
	op, err := c.operation("generateNewResourcePassword")
	req := c.request(ref, err)
	req.Path += "/password"
	req.Options = o
	return core.DoJSON(c.runtime, ctx, op, req, decodeResource(ref.ID, c.scope.ProjectID))
}
func (c *Client) TransferResource(ctx context.Context, ref types.ResourceRef, spec TransferResourceRequest, o types.CallOptions) (types.Result[ResourceWithCredentials], error) {
	op, err := c.operation("transferResourceToOtherProject")
	req := c.request(ref, err)
	req.Path += "/transfer"
	req.Options = o
	target := ""
	if spec.ProjectID != nil {
		target = *spec.ProjectID
	}
	req.Body, _ = json.Marshal(map[string]string{"project_id": target})
	validate := req.Validate
	req.Validate = func() error {
		if e := validate(); e != nil {
			return e
		}
		if spec.ProjectID == nil || target == "" || !utf8.ValidString(target) || len(spec.Extra) != 0 {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	return core.DoJSON(c.runtime, ctx, op, req, decodeResource(ref.ID, target))
}
func emptyDecode(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
	out := core.Decoded[struct{}]{}
	if len(bytes.TrimSpace(raw)) != 0 {
		return out, types.NewError(types.ProtocolError)
	}
	return out, nil
}
func (c *Client) DeleteResource(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	op, err := c.operation("deleteResource")
	op.NoContentSuccess = []int{204}
	req := c.request(ref, err)
	req.Options = o
	return core.DoJSON(c.runtime, ctx, op, req, emptyDecode)
}
