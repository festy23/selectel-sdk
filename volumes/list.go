package volumes

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func iterator[T any](c *Client, action, path, key, linksKey string, o pagination.Options, decode func(json.RawMessage, types.EndpointBinding) (T, error)) (*pagination.Iterator[T], error) {
	plan, e := c.runtime.PagePlan("volumes.v3."+action, "v3", c.scope, "", nil)
	if e != nil {
		return nil, e
	}
	plan.DynamicEndpoint = plan.EndpointID == ""
	expected := plan.EndpointID
	if o.Cursor != nil {
		expected = o.Cursor.EndpointID
	}
	return pagination.New(plan, o, func(ctx context.Context, marker string, size uint) (pagination.SourcePage[T], error) {
		q := url.Values{"limit": {strconv.FormatUint(uint64(size), 10)}}
		if marker != "" {
			q.Set("marker", marker)
		}
		next := ""
		result, e := core.DoJSON(c.runtime, ctx, operation(action, "GET"), core.Request{Scope: c.scope, Path: path, Query: q, ExpectedEndpointID: expected, PinnedEndpoint: c.listPin}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]T], error) {
			var body map[string]json.RawMessage
			if json.Unmarshal(raw, &body) != nil {
				return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			var rows []json.RawMessage
			if json.Unmarshal(body[key], &rows) != nil || rows == nil {
				return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			items := make([]T, 0, len(rows))
			for _, row := range rows {
				v, err := decode(row, ep)
				if err != nil {
					return core.Decoded[[]T]{}, err
				}
				items = append(items, v)
			}
			var links []struct{ Rel, Href string }
			if b, ok := body[linksKey]; ok && json.Unmarshal(b, &links) != nil {
				return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			for _, l := range links {
				if l.Rel != "next" {
					continue
				}
				base, _ := url.Parse(strings.TrimRight(ep.URL, "/") + path)
				target, err := url.Parse(l.Href)
				if err != nil {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				target = base.ResolveReference(target)
				if next != "" || target.Scheme != base.Scheme || target.Host != base.Host || target.EscapedPath() != base.EscapedPath() || target.User != nil || target.Fragment != "" || len(target.Query()["marker"]) != 1 {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				for k := range target.Query() {
					if k != "marker" && k != "limit" {
						return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
					}
				}
				next = target.Query().Get("marker")
				if next == "" {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
			}
			return core.Decoded[[]T]{Data: items}, nil
		})
		if e == nil {
			expected = result.Metadata.EndpointID
		}
		return pagination.SourcePage[T]{Items: result.Data, Next: next, Done: next == "", Metadata: result.Metadata}, e
	})
}
func (c *Client) Iterate(o pagination.Options) (*pagination.Iterator[Volume], error) {
	return iterator(c, "volumes.list", "/volumes/detail", "volumes", "volumes_links", o, func(raw json.RawMessage, ep types.EndpointBinding) (Volume, error) {
		body, _ := json.Marshal(struct {
			Volume json.RawMessage `json:"volume"`
		}{raw})
		v, e := decodeVolume("")(body, ep)
		return v.Data, e
	})
}
func (c *Client) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[Volume], error) {
	it, e := c.Iterate(o)
	if e != nil {
		return pagination.Collection[Volume]{Items: []Volume{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Client) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[Volume], error) {
	it, e := c.Iterate(o)
	if e != nil {
		return pagination.Page[Volume]{}, e
	}
	return it.Next(ctx)
}
func (t *Types) Iterate(o pagination.Options) (*pagination.Iterator[VolumeType], error) {
	return iterator(t.client, "types.list", "/types", "volume_types", "volume_type_links", o, func(raw json.RawMessage, ep types.EndpointBinding) (VolumeType, error) {
		return decodeType(raw, ep, "")
	})
}
func (t *Types) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[VolumeType], error) {
	it, e := t.Iterate(o)
	if e != nil {
		return pagination.Collection[VolumeType]{Items: []VolumeType{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (t *Types) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[VolumeType], error) {
	it, e := t.Iterate(o)
	if e != nil {
		return pagination.Page[VolumeType]{}, e
	}
	return it.Next(ctx)
}
func (t *Types) QoSLimits(ctx context.Context, o pagination.Options) (pagination.Collection[QoSLimits], error) {
	plan, e := t.client.runtime.PagePlan("volumes.v3.types.qos_limits", "v3", t.client.scope, "", nil)
	if e != nil {
		return pagination.Collection[QoSLimits]{}, e
	}
	plan.DynamicEndpoint = plan.EndpointID == ""
	expected := plan.EndpointID
	if o.Cursor != nil {
		expected = o.Cursor.EndpointID
	}
	it, e := pagination.New(plan, o, func(ctx context.Context, _ string, _ uint) (pagination.SourcePage[QoSLimits], error) {
		result, e := core.DoJSON(t.client.runtime, ctx, operation("types.qos_limits", "GET"), core.Request{Scope: t.client.scope, Path: "/qos-specs/qos_limits", ExpectedEndpointID: expected, PinnedEndpoint: t.client.listPin}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[[]QoSLimits], error) {
			var body map[string]json.RawMessage
			if json.Unmarshal(raw, &body) != nil || len(body) == 0 {
				return core.Decoded[[]QoSLimits]{}, types.NewError(types.ProtocolError)
			}
			keys := make([]string, 0, len(body))
			for k := range body {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			items := []QoSLimits{}
			for _, k := range keys {
				v := body[k]
				if len(v) == 0 || v[0] != '{' {
					continue
				}
				var q QoSLimits
				if json.Unmarshal(v, &q) != nil || q.VolumeTypeID == "" {
					return core.Decoded[[]QoSLimits]{}, types.NewError(types.ProtocolError)
				}
				items = append(items, q)
			}
			sort.SliceStable(items, func(i, j int) bool { return items[i].VolumeTypeID < items[j].VolumeTypeID })
			return core.Decoded[[]QoSLimits]{Data: items}, nil
		})
		return pagination.SourcePage[QoSLimits]{Items: result.Data, Done: true, Metadata: result.Metadata}, e
	})
	if e != nil {
		return pagination.Collection[QoSLimits]{}, e
	}
	return it.Collect(ctx)
}
