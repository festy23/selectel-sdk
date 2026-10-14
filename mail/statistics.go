package mail

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"time"
)

type Period struct{ DateFrom, DateTo string }
type ReadBounds struct{ MaxItems uint }

func (p Period) validate() error {
	from, e := time.Parse("2006-01-02", p.DateFrom)
	if e != nil {
		return types.NewError(types.InvalidArgument)
	}
	to, e := time.Parse("2006-01-02", p.DateTo)
	if e != nil || to.Before(from) {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func (c *Client) readLimit(op core.Operation, b ReadBounds) uint {
	if b.MaxItems > 0 {
		return b.MaxItems
	}
	plan, e := c.runtime.PagePlan(op.ID, "v1", c.scope, op.EndpointID, nil)
	if e != nil {
		return 1
	}
	return plan.Defaults.MaxItems
}
func trim[T any](rows *[]T, remaining *uint) bool {
	if rows == nil {
		return false
	}
	n := uint(len(*rows))
	if n > *remaining {
		*rows = (*rows)[:*remaining]
		*remaining = 0
		return true
	}
	*remaining -= n
	return false
}
func (c *Client) ProjectQuotas(ctx context.Context, o types.CallOptions) (types.Result[ProjectQuotas], error) {
	op, err := c.operation("retrieveProjectQuotas")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/quotas", Options: o, Validate: func() error { return err }}, decode[ProjectQuotas])
}
func (c *Client) ResourceQuotas(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[SesResourceQuotas], error) {
	op, err := c.operation("retireveResourceQuotas")
	req := c.request(ref, err)
	req.Path += "/quotas"
	req.Options = o
	return core.DoJSON(c.runtime, ctx, op, req, decode[SesResourceQuotas])
}
func (c *Client) GetStatistics(ctx context.Context, ref types.ResourceRef, period Period, bounds ReadBounds, o types.CallOptions) (types.Result[GetStatistics], error) {
	op, err := c.operation("retireveResourceStatistics")
	req := c.request(ref, err)
	req.Path += "/statistics"
	req.Options = o
	req.Query = url.Values{"date_from": {period.DateFrom}, "date_to": {period.DateTo}}
	validate := req.Validate
	req.Validate = func() error {
		if e := validate(); e != nil {
			return e
		}
		return period.validate()
	}
	limit := c.readLimit(op, bounds)
	return core.DoJSON(c.runtime, ctx, op, req, func(raw []byte, ep types.EndpointBinding) (core.Decoded[GetStatistics], error) {
		out, e := decode[GetStatistics](raw, ep)
		if e != nil {
			return out, e
		}
		if out.Data.Statistics == nil {
			return out, types.NewError(types.ProtocolError)
		}
		if trim(out.Data.Statistics.StatisticsByDates, &limit) {
			out.PartialRead = true
			return out, types.NewError(types.ResponseLimit)
		}
		return out, nil
	})
}

type DeliveryQuery struct{ MessageID, XTrackID *types.ProtectedValue }

func (c *Client) GetDeliveryStatus(ctx context.Context, ref types.ResourceRef, spec DeliveryQuery, bounds ReadBounds, o types.CallOptions) (types.Result[DeliveryStatus], error) {
	op, err := c.operation("getDeliveryStatus")
	req := c.request(ref, err)
	req.Path += "/issue/status"
	req.Options = o
	checkedQuery(&req, map[string]*types.ProtectedValue{"message_id": spec.MessageID, "x_track_id": spec.XTrackID})
	limit := c.readLimit(op, bounds)
	return core.DoJSON(c.runtime, ctx, op, req, func(raw []byte, ep types.EndpointBinding) (core.Decoded[DeliveryStatus], error) {
		out, e := decode[DeliveryStatus](raw, ep)
		if e != nil {
			return out, e
		}
		if out.Data.Messages == nil {
			return out, types.NewError(types.ProtocolError)
		}
		if trim(out.Data.Messages, &limit) {
			out.PartialRead = true
			return out, types.NewError(types.ResponseLimit)
		}
		return out, nil
	})
}
