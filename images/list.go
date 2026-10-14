package images

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

// Comparison strings preserve documented in:/time comparison syntax verbatim.
type FilterField string

// Filters selects source base-attribute comparisons; link/control fields are invalid.
type ListOptions struct {
	SortKeys, SortDirections                                                                                   []string
	Filters                                                                                                    map[FilterField]types.ProtectedValue
	Name, ID, Visibility, MemberStatus, Owner, Status, Sort, ContainerFormat, DiskFormat, CreatedAt, UpdatedAt string
	Hidden, Protected                                                                                          *bool
	SizeMin, SizeMax                                                                                           *types.Integer
	Tags                                                                                                       []string
	Properties                                                                                                 map[string]types.ProtectedValue
}

var queryControls = map[string]bool{"marker": true, "limit": true, "sort": true, "sort_key": true, "sort_dir": true, "tag": true, "size_min": true, "size_max": true, "member_status": true, "changes-since": true}

func (f ListOptions) query() (url.Values, error) {
	q := url.Values{}
	for k, v := range map[string]string{"name": f.Name, "id": f.ID, "visibility": f.Visibility, "member_status": f.MemberStatus, "owner": f.Owner, "status": f.Status, "sort": f.Sort, "container_format": f.ContainerFormat, "disk_format": f.DiskFormat, "created_at": f.CreatedAt, "updated_at": f.UpdatedAt} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if f.Sort != "" && (len(f.SortKeys) > 0 || len(f.SortDirections) > 0) {
		return nil, invalid()
	}
	for _, key := range f.SortKeys {
		q.Add("sort_key", key)
	}
	for _, direction := range f.SortDirections {
		q.Add("sort_dir", direction)
	}
	for k, v := range map[string]*bool{"os_hidden": f.Hidden, "protected": f.Protected} {
		if v != nil {
			q.Set(k, strconv.FormatBool(*v))
		}
	}
	for k, v := range map[string]*types.Integer{"size_min": f.SizeMin, "size_max": f.SizeMax} {
		if v != nil {
			if !nonnegative(v) {
				return nil, invalid()
			}
			q.Set(k, v.String())
		}
	}
	for _, tag := range f.Tags {
		// The server trims tags but still distinguishes tag= from omission.
		q.Add("tag", strings.TrimSpace(tag))
	}
	for field, v := range f.Filters {
		k := string(field)
		if !baseKeys[k] || queryControls[k] || k == "self" || k == "file" || k == "schema" || k == "direct_url" || k == "locations" || k == "stores" || k == "tags" {
			return nil, invalid()
		}
		if _, exists := q[k]; exists {
			return nil, invalid()
		}
		q.Set(k, string(v.Reveal()))
	}
	for k, v := range f.Properties {
		if !customKey(k) || queryControls[k] {
			return nil, invalid()
		}
		q.Set(k, string(v.Reveal()))
	}
	return q, nil
}
func (c *Client) Iterate(f ListOptions, o pagination.Options) (*pagination.Iterator[Image], error) {
	query, e := f.query()
	if e != nil {
		return nil, e
	}
	plan, e := c.runtime.PagePlan("images.v2.images.list", "v2", c.scope, "", query)
	if e != nil {
		return nil, e
	}
	plan.DynamicEndpoint = plan.EndpointID == ""
	expected := plan.EndpointID
	if o.Cursor != nil {
		expected = o.Cursor.EndpointID
	}
	return pagination.New(plan, o, func(ctx context.Context, marker string, size uint) (pagination.SourcePage[Image], error) {
		q := url.Values{}
		for k, v := range query {
			q[k] = append([]string(nil), v...)
		}
		q.Set("limit", strconv.FormatUint(uint64(size), 10))
		if marker != "" {
			q.Set("marker", marker)
		}
		next := ""
		result, e := core.DoJSON(c.runtime, ctx, operation("images.list", "GET"), core.Request{Scope: c.scope, Path: "/v2/images", Query: q, ExpectedEndpointID: expected}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]Image], error) {
			var body struct {
				Images []json.RawMessage `json:"images"`
				Next   string            `json:"next"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Images == nil {
				return core.Decoded[[]Image]{}, types.NewError(types.ProtocolError)
			}
			items := []Image{}
			for _, v := range body.Images {
				got, err := decodeImage("")(v, ep)
				if err != nil {
					return core.Decoded[[]Image]{}, err
				}
				items = append(items, got.Data)
			}
			if body.Next != "" {
				base, _ := url.Parse(strings.TrimSuffix(strings.TrimRight(ep.URL, "/"), "/v2") + "/v2/images")
				u, err := url.Parse(body.Next)
				if err != nil {
					return core.Decoded[[]Image]{}, types.NewError(types.ProtocolError)
				}
				u = base.ResolveReference(u)
				// Glance emits the API-root next URI even behind a catalog proxy
				// prefix. Reanchor only this known operation path, never a host.
				if u.Path == "/v2/images" && u.Scheme == base.Scheme && u.Host == base.Host {
					u.Path = base.Path
					u.RawPath = ""
				}
				if u.Scheme != base.Scheme || u.Host != base.Host || u.EscapedPath() != base.EscapedPath() || u.User != nil || u.Fragment != "" || len(u.Query()["marker"]) != 1 {
					return core.Decoded[[]Image]{}, types.NewError(types.ProtocolError)
				}
				nq := u.Query()
				next = nq.Get("marker")
				nq.Del("marker")
				nq.Del("limit")
				if next == "" || !reflect.DeepEqual(nq, query) {
					return core.Decoded[[]Image]{}, types.NewError(types.ProtocolError)
				}
			}
			return core.Decoded[[]Image]{Data: items}, nil
		})
		if e == nil {
			expected = result.Metadata.EndpointID
		}
		return pagination.SourcePage[Image]{Items: result.Data, Next: next, Done: next == "", Metadata: result.Metadata}, e
	})
}
func (c *Client) Collect(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Collection[Image], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Image]{Items: []Image{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Client) ListPage(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Page[Image], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[Image]{}, e
	}
	return it.Next(ctx)
}
