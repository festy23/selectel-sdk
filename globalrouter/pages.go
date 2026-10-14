package globalrouter

import (
	"context"
	"encoding/json"
	"math/big"
	"net/url"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

// Page contains exactly the requested provider page, without inferred completion.
type Page[T any] struct {
	Items []T `json:"items"`
}
type PageOptions struct {
	Page, Limit                      *types.Integer
	SortKeys, SortDirection, Filters *string
	ShowGroups, ShowZones            *bool
}

func pageQuery(o PageOptions, resource string) (url.Values, error) {
	q := url.Values{}
	for _, n := range []struct {
		key   string
		value *types.Integer
	}{{"page", o.Page}, {"limit", o.Limit}} {
		if n.value == nil {
			continue
		}
		v, ok := new(big.Int).SetString(n.value.String(), 10)
		if !ok || v.Sign() < 1 || (n.key == "limit" && v.Cmp(big.NewInt(200)) > 0) {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Set(n.key, n.value.String())
	}
	for _, s := range []struct {
		key   string
		value *string
	}{{"sort_keys", o.SortKeys}, {"sort_direction", o.SortDirection}, {"filters", o.Filters}} {
		if s.value != nil {
			q.Set(s.key, *s.value)
		}
	}
	if o.SortDirection != nil && *o.SortDirection != "asc" && *o.SortDirection != "desc" {
		return nil, types.NewError(types.InvalidArgument)
	}
	for _, b := range []struct {
		key, resource string
		value         *bool
	}{{"show_groups", "zones", o.ShowGroups}, {"show_zones", "zone_groups", o.ShowZones}} {
		if b.value == nil {
			continue
		}
		if b.resource != resource {
			return nil, types.NewError(types.InvalidArgument)
		}
		if *b.value {
			q.Set(b.key, "true")
		} else {
			q.Set(b.key, "false")
		}
	}
	return q, nil
}
func listPage[T any](c *Client, ctx context.Context, resource, kind string, o PageOptions, call types.CallOptions) (types.Result[Page[T]], error) {
	op, bindErr := c.operation(resource, "list", "GET")
	q, qErr := pageQuery(o, resource)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		return qErr
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (out core.Decoded[Page[T]], err error) {
		var rows []json.RawMessage
		if json.Unmarshal(raw, &rows) != nil || rows == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Items = []T{}
		for _, row := range rows {
			var value T
			if json.Unmarshal(row, &value) != nil {
				return out, types.NewError(types.ProtocolError)
			}
			if e := c.bindTree(&value, endpoint, kind, nil); e != nil {
				return out, e
			}
			out.Data.Items = append(out.Data.Items, value)
		}
		return out, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/" + resource, Query: q, Options: call, Validate: validate}, decode)
}
func (c *RoutersClient) ListPage(ctx context.Context, o PageOptions, call types.CallOptions) (types.Result[Page[Router]], error) {
	return listPage[Router](c.client, ctx, "routers", "router", o, call)
}
func (c *NetworksClient) ListPage(ctx context.Context, o PageOptions, call types.CallOptions) (types.Result[Page[Network]], error) {
	return listPage[Network](c.client, ctx, "networks", "network", o, call)
}
func (c *SubnetsClient) ListPage(ctx context.Context, o PageOptions, call types.CallOptions) (types.Result[Page[Subnet]], error) {
	return listPage[Subnet](c.client, ctx, "subnets", "subnet", o, call)
}
func (c *StaticRoutesClient) ListPage(ctx context.Context, o PageOptions, call types.CallOptions) (types.Result[Page[StaticRoute]], error) {
	return listPage[StaticRoute](c.client, ctx, "static_routes", "static_route", o, call)
}
func (c *Client) ZonesPage(ctx context.Context, o PageOptions, call types.CallOptions) (types.Result[Page[Zone]], error) {
	return listPage[Zone](c, ctx, "zones", "zone", o, call)
}
func (c *Client) ServicesPage(ctx context.Context, o PageOptions, call types.CallOptions) (types.Result[Page[Service]], error) {
	return listPage[Service](c, ctx, "services", "service", o, call)
}
func (c *Client) ZoneGroupsPage(ctx context.Context, o PageOptions, call types.CallOptions) (types.Result[Page[ZoneGroup]], error) {
	return listPage[ZoneGroup](c, ctx, "zone_groups", "zone_group", o, call)
}
func (c *Client) QuotasPage(ctx context.Context, o PageOptions, call types.CallOptions) (types.Result[Page[Quota]], error) {
	return listPage[Quota](c, ctx, "quotas", "quota", o, call)
}
