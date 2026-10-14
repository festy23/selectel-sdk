package cdn

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) GetResourcesStatus(ctx context.Context, refs []types.ResourceRef, o types.CallOptions) (types.Result[[]ResourceStatus], error) {
	op, bindErr := c.operation("cdn.v3.resource_status.list", "GET")
	q := url.Values{}
	ids := []string{}
	known := map[string]types.ResourceRef{}
	for _, ref := range refs {
		ids = append(ids, ref.ID)
		known[ref.ID] = ref
	}
	if len(ids) > 0 {
		q.Set("resource_id", strings.Join(ids, ","))
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/resources/status", Query: q, Refs: refs, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if len(refs) > 50 || len(known) != len(refs) {
			return types.NewError(types.InvalidArgument)
		}
		for _, ref := range refs {
			if e := c.refError(ref, "resource"); e != nil {
				return e
			}
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[[]ResourceStatus], error) {
		items := []ResourceStatus{}
		if len(refs) == 0 && len(bytes.TrimSpace(raw)) == 0 {
			return core.Decoded[[]ResourceStatus]{Data: items}, nil
		}
		if json.Unmarshal(raw, &items) != nil || items == nil {
			return core.Decoded[[]ResourceStatus]{}, types.NewError(types.ProtocolError)
		}
		seen := map[string]bool{}
		for i := range items {
			if items[i].Resource == nil {
				return core.Decoded[[]ResourceStatus]{}, types.NewError(types.ProtocolError)
			}
			id := *items[i].Resource
			ref, ok := known[id]
			if !ok {
				return core.Decoded[[]ResourceStatus]{}, types.NewError(types.ScopeMismatch)
			}
			if seen[id] {
				return core.Decoded[[]ResourceStatus]{}, types.NewError(types.ProtocolError)
			}
			seen[id] = true
			items[i].Ref = &ref
		}
		return core.Decoded[[]ResourceStatus]{Data: items}, nil
	})
}
func (c *Client) GetResourceStatus(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[ResourceStatus], error) {
	op, bindErr := c.operation("cdn.v3.resource_status.get", "GET")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/resources/" + url.PathEscape(ref.ID) + "/status", Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref, "resource")
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[ResourceStatus], error) {
		var out ResourceStatus
		if json.Unmarshal(raw, &out) != nil {
			return core.Decoded[ResourceStatus]{}, types.NewError(types.ProtocolError)
		}
		if out.Resource != nil && *out.Resource != ref.ID {
			return core.Decoded[ResourceStatus]{}, types.NewError(types.ScopeMismatch)
		}
		known := ref
		out.Ref = &known
		return core.Decoded[ResourceStatus]{Data: out}, nil
	})
}
