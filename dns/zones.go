package dns

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"regexp"
	"unicode/utf8"
)

type Empty struct{}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func decodeZone(raw []byte, ep types.EndpointBinding, expected string) (core.Decoded[Zone], error) {
	var id struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &id) != nil || id.ID == "" || expected != "" && id.ID != expected {
		return core.Decoded[Zone]{}, types.NewError(types.ProtocolError)
	}
	ref := core.Ref(ep, "dns", "zone", id.ID)
	out := core.Decoded[Zone]{Refs: []types.ResourceRef{*ref}}
	var v Zone
	if json.Unmarshal(raw, &v) != nil {
		return out, types.NewError(types.ProtocolError)
	}
	v.Ref = ref
	out.Data = v
	return out, nil
}
func (c *Zones) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Zone], error) {
	return core.DoJSON(c.runtime, ctx, op("zones", "get", "GET", 200), core.Request{Scope: c.scope, Path: "/zones/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o, Validate: func() error { return c.valid(ref, "zone") }}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[Zone], error) {
		return decodeZone(raw, ep, ref.ID)
	})
}
func (c *Zones) change(ctx context.Context, ref types.ResourceRef, action, method, suffix string, body any, o types.CallOptions, validate func() error) (types.Result[Empty], error) {
	var raw []byte
	var e error
	if body != nil {
		raw, e = json.Marshal(body)
	}
	return core.DoJSON(c.runtime, ctx, op("zones", action, method, 204), core.Request{Scope: c.scope, Path: "/zones/" + segment(ref.ID) + suffix, Body: raw, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o, Validate: func() error {
		if e != nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := c.valid(ref, "zone"); e != nil {
			return e
		}
		if validate != nil {
			return validate()
		}
		return nil
	}}, func([]byte, types.EndpointBinding) (core.Decoded[Empty], error) { return core.Decoded[Empty]{}, nil })
}
func (c *Zones) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return c.change(ctx, ref, "delete", "DELETE", "", nil, o, nil)
}
func (c *Zones) UpdateComment(ctx context.Context, ref types.ResourceRef, comment string, o types.CallOptions) (types.Result[Empty], error) {
	return c.change(ctx, ref, "comment", "PATCH", "", map[string]string{"comment": comment}, o, func() error {
		if !utf8.ValidString(comment) || utf8.RuneCountInString(comment) > 255 {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	})
}
func (c *Zones) SetDisabled(ctx context.Context, ref types.ResourceRef, disabled bool, o types.CallOptions) (types.Result[Empty], error) {
	return c.change(ctx, ref, "state", "PATCH", "/state", map[string]bool{"disabled": disabled}, o, nil)
}
func (c *Zones) SetProtected(ctx context.Context, ref types.ResourceRef, protected bool, o types.CallOptions) (types.Result[Empty], error) {
	return c.change(ctx, ref, "protection", "PATCH", "/protection", map[string]bool{"protected": protected}, o, nil)
}
func (c *Zones) TransferOwner(ctx context.Context, ref types.ResourceRef, projectID string, o types.CallOptions) (types.Result[Empty], error) {
	return c.change(ctx, ref, "owner", "PUT", "/owner", map[string]string{"project_id": projectID}, o, func() error {
		if !uuidPattern.MatchString(projectID) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	})
}
