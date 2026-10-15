package objectstorage

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math"
	"net/url"
	"strconv"
)

type ExportListOptions struct {
	Offset  *types.Integer
	OrderBy *string
	types.CallOptions
}

func (c *LogExports) Iterate(in ExportListOptions, o pagination.Options) (*pagination.Iterator[ExportResponse], error) {
	s := c.client
	op, e := s.runtime.BindDocumented(operationBindings["log_exports.list"], s.scope)
	if e != nil {
		return nil, e
	}
	offset := uint64(0)
	if in.Offset != nil {
		var e error
		offset, e = strconv.ParseUint(in.Offset.String(), 10, 64)
		if e != nil {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	// The iterator owns the original semantic request for all pages and cursors.
	if in.OrderBy != nil {
		order := *in.OrderBy
		in.OrderBy = &order
	}
	semantic := struct {
		Offset      uint64
		OrderBy     *string
		EndpointURL string
	}{offset, in.OrderBy, op.FixedEndpoint}
	plan, e := s.runtime.PagePlan(op.ID, "v2", s.scope, op.EndpointID, semantic)
	if e != nil {
		return nil, e
	}
	plan.WireMax = 1000
	fetch := func(ctx context.Context, token string, size uint) (pagination.SourcePage[ExportResponse], error) {
		current := offset
		if token != "" {
			var e error
			current, e = strconv.ParseUint(token, 10, 64)
			if e != nil {
				return pagination.SourcePage[ExportResponse]{}, types.NewError(types.InvalidArgument)
			}
		}
		q := url.Values{"limit": {strconv.FormatUint(uint64(size), 10)}, "offset": {strconv.FormatUint(current, 10)}}
		if in.OrderBy != nil {
			q.Set("order_by", *in.OrderBy)
		}
		items := []ExportResponse{}
		count := 0
		result, e := core.DoJSON(s.runtime, ctx, op, core.Request{Scope: s.scope, Path: "/v2/logs", Query: q, Options: in.CallOptions}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]ExportResponse], error) {
			var envelope struct {
				Tasks *[]json.RawMessage `json:"tasks"`
			}
			if json.Unmarshal(raw, &envelope) != nil || envelope.Tasks == nil {
				return core.Decoded[[]ExportResponse]{}, types.NewError(types.ProtocolError)
			}
			count = len(*envelope.Tasks)
			if uint(count) > size {
				return core.Decoded[[]ExportResponse]{}, types.NewError(types.ProtocolError)
			}
			for _, rawTask := range *envelope.Tasks {
				var item ExportResponse
				if json.Unmarshal(rawTask, &item) != nil || item.validate() != nil || item.Task == nil || item.Task.ID == nil || *item.Task.ID == "" {
					return core.Decoded[[]ExportResponse]{}, types.NewError(types.ProtocolError)
				}
				item.Ref = core.Ref(ep, "object_storage", "log_export", *item.Task.ID)
				items = append(items, item)
			}
			return core.Decoded[[]ExportResponse]{Data: items}, nil
		})
		page := pagination.SourcePage[ExportResponse]{Items: items, Done: e == nil && count == 0, Metadata: result.Metadata}
		if e == nil && count > 0 {
			if current > math.MaxUint64-uint64(count) {
				return page, types.NewError(types.ProtocolError)
			}
			page.Next = strconv.FormatUint(current+uint64(count), 10)
		}
		return page, e
	}
	return pagination.New(plan, o, fetch)
}
func (c *LogExports) Collect(ctx context.Context, in ExportListOptions, o pagination.Options) (pagination.Collection[ExportResponse], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Collection[ExportResponse]{Items: []ExportResponse{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *LogExports) ListPage(ctx context.Context, in ExportListOptions, o pagination.Options) (pagination.Page[ExportResponse], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Page[ExportResponse]{}, e
	}
	return it.Next(ctx)
}
