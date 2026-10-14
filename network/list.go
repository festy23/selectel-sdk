package network

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

// Selection preserves repeated filters (OR) and distinct fields (AND). Owner
// filters select provider-visible resources; they never change authorization.
type Selection[F ~string] struct {
	Filters     map[F][]string
	Fields      []F
	SortKeys    []F
	SortDirs    []string
	PageReverse *bool
}
type NetworkFilters = Selection[NetworkField]
type SubnetFilters = Selection[SubnetField]
type PortFilters = Selection[PortField]

func selectionQuery[F ~string](s Selection[F], valid func(F) bool) (url.Values, error) {
	q, e := fieldQuery(s.Fields, valid)
	if e != nil {
		return nil, e
	}
	for f, values := range s.Filters {
		if !valid(f) || len(values) == 0 {
			return nil, types.NewError(types.InvalidArgument)
		}
		q[string(f)] = append([]string(nil), values...)
	}
	if len(s.SortKeys) != len(s.SortDirs) {
		return nil, types.NewError(types.InvalidArgument)
	}
	for i, f := range s.SortKeys {
		if !valid(f) || (s.SortDirs[i] != "asc" && s.SortDirs[i] != "desc") {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Add("sort_key", string(f))
		q.Add("sort_dir", s.SortDirs[i])
	}
	if s.PageReverse != nil {
		q.Set("page_reverse", strconv.FormatBool(*s.PageReverse))
	}
	return q, nil
}
func iterate[T model, F ~string](runtime *core.Runtime, scope types.Scope, kind string, s Selection[F], valid func(F) bool, setRef func(*T, *types.ResourceRef), options pagination.Options) (*pagination.Iterator[T], error) {
	values, e := selectionQuery(s, valid)
	if e != nil {
		return nil, e
	}
	op := operation(kind, "list", "GET")
	plan, e := runtime.PagePlan(op.ID, "v2", scope, "", values)
	if e != nil {
		return nil, e
	}
	plan.DynamicEndpoint = plan.EndpointID == ""
	expectedEndpoint := plan.EndpointID
	if options.Cursor != nil {
		expectedEndpoint = options.Cursor.EndpointID
	}
	path := "/v2.0/" + kind + "s"
	projection := len(s.Fields) > 0
	rel := "next"
	if values.Get("page_reverse") == "true" {
		rel = "previous"
	}
	fetch := func(ctx context.Context, marker string, size uint) (pagination.SourcePage[T], error) {
		q := url.Values{}
		for k, v := range values {
			q[k] = append([]string(nil), v...)
		}
		q.Set("limit", strconv.FormatUint(uint64(size), 10))
		if marker != "" {
			q.Set("marker", marker)
		}
		next := ""
		decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[[]T], error) {
			var body map[string]json.RawMessage
			var items []json.RawMessage
			var links []struct{ Rel, Href string }
			out := core.Decoded[[]T]{Data: []T{}}
			if json.Unmarshal(raw, &body) != nil || json.Unmarshal(body[kind+"s"], &items) != nil || items == nil {
				return out, types.NewError(types.ProtocolError)
			}
			if rawLinks, ok := body[kind+"s_links"]; ok && json.Unmarshal(rawLinks, &links) != nil {
				return out, types.NewError(types.ProtocolError)
			}
			for _, item := range items {
				v, e := decodeItem(item, endpoint, decodeOptions[T]{Kind: kind, Projection: projection, SetRef: setRef})
				if e != nil {
					return out, e
				}
				out.Data = append(out.Data, v.Data)
			}
			for _, link := range links {
				if link.Rel != rel {
					continue
				}
				if next != "" {
					return out, types.NewError(types.ProtocolError)
				}
				target, e := url.Parse(link.Href)
				base, _ := url.Parse(strings.TrimRight(endpoint.URL, "/") + path)
				if e != nil {
					return out, types.NewError(types.ProtocolError)
				}
				target = base.ResolveReference(target)
				if target.Scheme != base.Scheme || target.Host != base.Host || target.EscapedPath() != base.EscapedPath() || target.User != nil || target.Fragment != "" {
					return out, types.NewError(types.ProtocolError)
				}
				query, e := url.ParseQuery(target.RawQuery)
				if e != nil || len(query["marker"]) != 1 || query.Get("marker") == "" {
					return out, types.NewError(types.ProtocolError)
				}
				next = query.Get("marker")
			}
			return out, nil
		}
		result, e := core.DoJSON(runtime, ctx, op, core.Request{Scope: scope, Path: path, Query: q, ExpectedEndpointID: expectedEndpoint}, decode)
		if e == nil {
			expectedEndpoint = result.Metadata.EndpointID
		}
		return pagination.SourcePage[T]{Items: result.Data, Next: next, Done: next == "", Metadata: result.Metadata}, e
	}
	return pagination.New(plan, options, fetch)
}

func (n *Networks) Iterate(f NetworkFilters, o pagination.Options) (*pagination.Iterator[Network], error) {
	return iterate(n.runtime, n.scope, "network", f, validNetworkField, func(v *Network, r *types.ResourceRef) { v.Ref = r }, o)
}
func (n *Networks) Collect(ctx context.Context, f NetworkFilters, o pagination.Options) (pagination.Collection[Network], error) {
	it, e := n.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Network]{Items: []Network{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (n *Networks) ListPage(ctx context.Context, f NetworkFilters, o pagination.Options) (pagination.Page[Network], error) {
	it, e := n.Iterate(f, o)
	if e != nil {
		return pagination.Page[Network]{}, e
	}
	return it.Next(ctx)
}

func (n *Subnets) Iterate(f SubnetFilters, o pagination.Options) (*pagination.Iterator[Subnet], error) {
	return iterate(n.runtime, n.scope, "subnet", f, validSubnetField, func(v *Subnet, r *types.ResourceRef) { v.Ref = r }, o)
}
func (n *Subnets) Collect(ctx context.Context, f SubnetFilters, o pagination.Options) (pagination.Collection[Subnet], error) {
	it, e := n.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Subnet]{Items: []Subnet{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (n *Subnets) ListPage(ctx context.Context, f SubnetFilters, o pagination.Options) (pagination.Page[Subnet], error) {
	it, e := n.Iterate(f, o)
	if e != nil {
		return pagination.Page[Subnet]{}, e
	}
	return it.Next(ctx)
}

func (n *Ports) Iterate(f PortFilters, o pagination.Options) (*pagination.Iterator[Port], error) {
	return iterate(n.runtime, n.scope, "port", f, validPortField, func(v *Port, r *types.ResourceRef) { v.Ref = r }, o)
}
func (n *Ports) Collect(ctx context.Context, f PortFilters, o pagination.Options) (pagination.Collection[Port], error) {
	it, e := n.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Port]{Items: []Port{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (n *Ports) ListPage(ctx context.Context, f PortFilters, o pagination.Options) (pagination.Page[Port], error) {
	it, e := n.Iterate(f, o)
	if e != nil {
		return pagination.Page[Port]{}, e
	}
	return it.Next(ctx)
}
