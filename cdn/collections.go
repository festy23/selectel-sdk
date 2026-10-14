package cdn

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type ListOptions struct{ Active *bool }

func collectFixed[T any](c *Client, ctx context.Context, id, path string, query url.Values, options pagination.Options, decode core.Decoder[[]T]) (pagination.Collection[T], error) {
	empty := pagination.Collection[T]{Items: []T{}, StopReason: "error"}
	op, e := c.operation(id, "GET")
	if e != nil {
		return empty, e
	}
	plan, e := c.runtime.PagePlan(id, "v3", c.scope, op.EndpointID, struct{ Path, Query string }{path, query.Encode()})
	if e != nil {
		return empty, e
	}
	plan.FixedSize = true
	iterator, e := pagination.New(plan, options, func(ctx context.Context, _ string, _ uint) (pagination.SourcePage[T], error) {
		result, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Query: query}, decode)
		return pagination.SourcePage[T]{Items: result.Data, Metadata: result.Metadata, Done: e == nil}, e
	})
	if e != nil {
		return empty, e
	}
	return iterator.Collect(ctx)
}
func (c *Client) ListResources(ctx context.Context, spec ListOptions, options pagination.Options) (pagination.Collection[Resource], error) {
	q := url.Values{}
	if spec.Active != nil {
		q.Set("active", strconv.FormatBool(*spec.Active))
	}
	return collectFixed(c, ctx, "cdn.v3.resources.list", "/resources", q, options, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]Resource], error) {
		var items []Resource
		if json.Unmarshal(raw, &items) != nil || items == nil {
			return core.Decoded[[]Resource]{}, types.NewError(types.ProtocolError)
		}
		seen := map[string]bool{}
		for i := range items {
			if items[i].Validate() != nil {
				return core.Decoded[[]Resource]{}, types.NewError(types.ProtocolError)
			}
			if items[i].ID != nil {
				id := *items[i].ID
				if !resourceID.MatchString(id) || seen[id] {
					return core.Decoded[[]Resource]{}, types.NewError(types.ProtocolError)
				}
				seen[id] = true
				items[i].Ref = core.Ref(ep, "cdn", "resource", id)
			}
		}
		return core.Decoded[[]Resource]{Data: items}, nil
	})
}
