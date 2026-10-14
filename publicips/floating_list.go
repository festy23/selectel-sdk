package publicips

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
	"strings"
)

type FloatingIPGetOptions struct{ Fields []FloatingIPField }
type FloatingIPListOptions struct {
	Filters     map[FloatingIPFilter][]string
	Fields      []FloatingIPField
	SortKeys    []FloatingIPField
	SortDirs    []string
	PageReverse *bool
}

func floatingFields(fields []FloatingIPField) (url.Values, error) {
	q := url.Values{}
	for _, f := range fields {
		if !validFloatingIPField(f) {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Add("fields", string(f))
	}
	return q, nil
}
func (c *FloatingIPs) Get(ctx context.Context, ref types.ResourceRef, options FloatingIPGetOptions, o types.CallOptions) (types.Result[FloatingIP], error) {
	q, e := floatingFields(options.Fields)
	in := core.Request{Scope: c.scope, Path: "/v2.0/floatingips/" + url.PathEscape(ref.ID), Query: q, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pinned(ref), Validate: func() error {
		if e != nil {
			return e
		}
		return validateNetworkRef(ref, "floating_ip")
	}}
	return core.DoJSON(c.runtime, ctx, floatingOp("get", "GET"), in, c.decodeFloating(&ref, nil, "", len(options.Fields) > 0))
}
func (c *FloatingIPs) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(c.runtime, ctx, floatingOp("delete", "DELETE"), core.Request{Scope: c.scope, Path: "/v2.0/floatingips/" + url.PathEscape(ref.ID), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pinned(ref), Validate: func() error { return validateNetworkRef(ref, "floating_ip") }}, func([]byte, types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
func (c *FloatingIPs) Iterate(f FloatingIPListOptions, o pagination.Options) (*pagination.Iterator[FloatingIP], error) {
	q, e := floatingFields(f.Fields)
	if e != nil {
		return nil, e
	}
	for field, values := range f.Filters {
		if !validFloatingIPFilter(field) || len(values) == 0 {
			return nil, types.NewError(types.InvalidArgument)
		}
		q[string(field)] = append([]string(nil), values...)
	}
	if len(f.SortKeys) != len(f.SortDirs) {
		return nil, types.NewError(types.InvalidArgument)
	}
	for i, key := range f.SortKeys {
		if !validFloatingIPField(key) || (f.SortDirs[i] != "asc" && f.SortDirs[i] != "desc") {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Add("sort_key", string(key))
		q.Add("sort_dir", f.SortDirs[i])
	}
	rel := "next"
	if f.PageReverse != nil {
		q.Set("page_reverse", strconv.FormatBool(*f.PageReverse))
		if *f.PageReverse {
			rel = "previous"
		}
	}
	op := floatingOp("list", "GET")
	plan, e := c.runtime.PagePlan(op.ID, "v2", c.scope, "", q)
	if e != nil {
		return nil, e
	}
	plan.DynamicEndpoint = plan.EndpointID == ""
	expected := plan.EndpointID
	if o.Cursor != nil {
		expected = o.Cursor.EndpointID
	}
	projection := len(f.Fields) > 0
	fetch := func(ctx context.Context, marker string, size uint) (pagination.SourcePage[FloatingIP], error) {
		query := url.Values{}
		for k, v := range q {
			query[k] = append([]string(nil), v...)
		}
		query.Set("limit", strconv.FormatUint(uint64(size), 10))
		if marker != "" {
			query.Set("marker", marker)
		}
		next := ""
		result, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2.0/floatingips", Query: query, ExpectedEndpointID: expected}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]FloatingIP], error) {
			out := core.Decoded[[]FloatingIP]{Data: []FloatingIP{}}
			var body struct {
				Items []json.RawMessage            `json:"floatingips"`
				Links []struct{ Rel, Href string } `json:"floatingips_links"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Items == nil {
				return out, types.NewError(types.ProtocolError)
			}
			for _, item := range body.Items {
				env, _ := json.Marshal(map[string]json.RawMessage{"floatingip": item})
				v, e := c.decodeFloating(nil, nil, "", projection)(env, ep)
				if e != nil {
					return out, e
				}
				out.Data = append(out.Data, v.Data)
			}
			for _, link := range body.Links {
				if link.Rel != rel {
					continue
				}
				if next != "" {
					return out, types.NewError(types.ProtocolError)
				}
				target, e := url.Parse(link.Href)
				base, _ := url.Parse(strings.TrimRight(ep.URL, "/") + "/v2.0/floatingips")
				if e != nil {
					return out, types.NewError(types.ProtocolError)
				}
				target = base.ResolveReference(target)
				if target.Scheme != base.Scheme || target.Host != base.Host || target.EscapedPath() != base.EscapedPath() || target.User != nil || target.Fragment != "" {
					return out, types.NewError(types.ProtocolError)
				}
				nextQuery, e := url.ParseQuery(target.RawQuery)
				if e != nil || len(nextQuery["marker"]) != 1 || nextQuery.Get("marker") == "" {
					return out, types.NewError(types.ProtocolError)
				}
				next = nextQuery.Get("marker")
			}
			return out, nil
		})
		if e == nil {
			expected = result.Metadata.EndpointID
		}
		return pagination.SourcePage[FloatingIP]{Items: result.Data, Next: next, Done: next == "", Metadata: result.Metadata}, e
	}
	return pagination.New(plan, o, fetch)
}
func (c *FloatingIPs) Collect(ctx context.Context, f FloatingIPListOptions, o pagination.Options) (pagination.Collection[FloatingIP], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[FloatingIP]{Items: []FloatingIP{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *FloatingIPs) ListPage(ctx context.Context, f FloatingIPListOptions, o pagination.Options) (pagination.Page[FloatingIP], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[FloatingIP]{}, e
	}
	return it.Next(ctx)
}
