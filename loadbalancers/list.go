package loadbalancers

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// ListOptions selects documented Octavia fields and filters, preserving repeated values.
// Page size and marker belong to pagination.Options and cannot be overridden here.
type ListOptions struct {
	Filters          map[string][]string
	Fields           []string
	Sort             []string
	SortKey, SortDir []string
	PageReverse      *bool
}

func query(resource string, f ListOptions) (url.Values, error) {
	q := url.Values{}
	for key, values := range f.Filters {
		if !filters[resource][key] || len(values) == 0 {
			return nil, types.NewError(types.InvalidArgument)
		}
		q[key] = append([]string(nil), values...)
	}
	for _, key := range f.Fields {
		if !projections[resource][key] {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Add("fields", key)
	}
	for _, sort := range f.Sort {
		for _, pair := range strings.Split(sort, ",") {
			p := strings.Split(pair, ":")
			if !filters[resource][p[0]] || len(p) > 2 || len(p) == 2 && p[1] != "asc" && p[1] != "desc" {
				return nil, types.NewError(types.InvalidArgument)
			}
		}
		q.Add("sort", sort)
	}
	if len(f.SortKey) != len(f.SortDir) {
		return nil, types.NewError(types.InvalidArgument)
	}
	for i, k := range f.SortKey {
		if !filters[resource][k] || f.SortDir[i] != "asc" && f.SortDir[i] != "desc" {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Add("sort_key", k)
		q.Add("sort_dir", f.SortDir[i])
	}
	if f.PageReverse != nil {
		q.Set("page_reverse", strconv.FormatBool(*f.PageReverse))
	}
	return q, nil
}
func iterate[T any](s service, resource, kind string, parentRef *types.ResourceRef, f ListOptions, o pagination.Options) (*pagination.Iterator[T], error) {
	q, e := query(resource, f)
	if e != nil {
		return nil, e
	}
	path := "/v2/lbaas/" + resource
	if resource == "members" {
		if parentRef == nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		if e := s.valid(*parentRef, "pool"); e != nil {
			return nil, e
		}
		path = "/v2/lbaas/pools/" + segment(parentRef.ID) + "/members"
	}
	op := operation(resource, "list", "GET", 200)
	plan, e := s.runtime.PagePlan(op.ID, "v2", s.scope, "", struct {
		Options ListOptions
		Parent  *types.ResourceRef
	}{f, parentRef})
	if e != nil {
		return nil, e
	}
	plan.DynamicEndpoint = true
	expected := ""
	if o.Cursor != nil {
		expected = o.Cursor.EndpointID
	}
	fetch := func(ctx context.Context, marker string, size uint) (pagination.SourcePage[T], error) {
		cq := url.Values{}
		for k, v := range q {
			cq[k] = append([]string(nil), v...)
		}
		cq.Set("limit", strconv.FormatUint(uint64(size), 10))
		if marker != "" {
			cq.Set("marker", marker)
		}
		refs := []types.ResourceRef{}
		if parentRef != nil {
			refs = append(refs, *parentRef)
		}
		result, e := core.DoJSON(s.runtime, ctx, op, core.Request{Scope: s.scope, Path: path, Query: cq, Refs: refs, ExpectedEndpointID: expected}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[page[T]], error) {
			var env map[string]json.RawMessage
			var data page[T]
			if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env[resource], &data.Items) != nil || data.Items == nil {
				return core.Decoded[page[T]]{}, types.NewError(types.ProtocolError)
			}
			for i := range data.Items {
				if _, e := attach(&data.Items[i], ep, kind, nil, parentRef, len(f.Fields) > 0); e != nil {
					return core.Decoded[page[T]]{}, e
				}
			}
			var links []struct{ Rel, Href string }
			if b := env[resource+"_links"]; b != nil && json.Unmarshal(b, &links) != nil {
				return core.Decoded[page[T]]{}, types.NewError(types.ProtocolError)
			}
			for _, link := range links {
				if link.Rel != "next" {
					continue
				}
				if data.Next != "" {
					return core.Decoded[page[T]]{}, types.NewError(types.ProtocolError)
				}
				basePath := path
				for _, suffix := range []string{"/v2", "/v2.0"} {
					if strings.HasSuffix(strings.TrimRight(ep.URL, "/"), suffix) {
						basePath = strings.TrimPrefix(basePath, "/v2")
						break
					}
				}
				base, _ := url.Parse(strings.TrimRight(ep.URL, "/") + basePath)
				next, e := url.Parse(link.Href)
				if e != nil {
					return core.Decoded[page[T]]{}, types.NewError(types.ProtocolError)
				}
				next = base.ResolveReference(next)
				if next.Scheme != base.Scheme || next.Host != base.Host || next.User != nil || next.Fragment != "" || next.Path != base.Path && next.Path != base.Path+".json" {
					return core.Decoded[page[T]]{}, types.NewError(types.ProtocolError)
				}
				nq := next.Query()
				for key, values := range nq {
					if key == "marker" {
						continue
					}
					if !reflect.DeepEqual(values, cq[key]) {
						return core.Decoded[page[T]]{}, types.NewError(types.ProtocolError)
					}
				}
				data.Next = nq.Get("marker")
				if data.Next == "" || data.Next == marker || len(nq["marker"]) != 1 {
					return core.Decoded[page[T]]{}, types.NewError(types.ProtocolError)
				}
			}
			return core.Decoded[page[T]]{Data: data}, nil
		})
		if result.Metadata.EndpointID != "" {
			expected = result.Metadata.EndpointID
		}
		return pagination.SourcePage[T]{Items: result.Data.Items, Next: result.Data.Next, Done: result.Data.Next == "", Metadata: result.Metadata}, e
	}
	return pagination.New(plan, o, fetch)
}

type page[T any] struct {
	Items []T
	Next  string
}

func (c *Client) Iterate(f ListOptions, o pagination.Options) (*pagination.Iterator[LoadBalancer], error) {
	return iterate[LoadBalancer](service{c.runtime, c.scope}, "loadbalancers", "loadbalancer", nil, f, o)
}
func (c *Client) ListPage(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Page[LoadBalancer], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[LoadBalancer]{}, e
	}
	return it.Next(ctx)
}
func (c *Client) Collect(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Collection[LoadBalancer], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[LoadBalancer]{Complete: false}, e
	}
	return it.Collect(ctx)
}
