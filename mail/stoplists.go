package mail

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/mail"
	"unicode/utf8"
)

type StopListEntry struct{ MailFrom, Email types.ProtectedValue }

func (c *Client) stopMutation(ctx context.Context, id string, ref types.ResourceRef, spec StopListEntry, o types.CallOptions) (types.Result[struct{}], error) {
	op, err := c.operation(id)
	op.NoContentSuccess = []int{204}
	req := c.request(ref, err)
	req.Path += "/stop-lists"
	req.Options = o
	checkedEmailQuery(&req, map[string]*types.ProtectedValue{"mail_from": &spec.MailFrom, "email": &spec.Email})
	return core.DoJSON(c.runtime, ctx, op, req, emptyDecode)
}
func (c *Client) AddStopListEntry(ctx context.Context, ref types.ResourceRef, spec StopListEntry, o types.CallOptions) (types.Result[struct{}], error) {
	return c.stopMutation(ctx, "addEmailToStopList", ref, spec, o)
}
func (c *Client) RemoveStopListEntry(ctx context.Context, ref types.ResourceRef, spec StopListEntry, o types.CallOptions) (types.Result[struct{}], error) {
	return c.stopMutation(ctx, "removeEmailFromStopList", ref, spec, o)
}
func (c *Client) SearchStopList(ctx context.Context, ref types.ResourceRef, email types.ProtectedValue, bounds ReadBounds, o types.CallOptions) (types.Result[StopList], error) {
	op, err := c.operation("searchEmailInStopList")
	req := c.request(ref, err)
	req.Path += "/stop-lists"
	req.Options = o
	checkedEmailQuery(&req, map[string]*types.ProtectedValue{"email": &email})
	limit := c.readLimit(op, bounds)
	return core.DoJSON(c.runtime, ctx, op, req, func(raw []byte, ep types.EndpointBinding) (core.Decoded[StopList], error) {
		out, e := decode[StopList](raw, ep)
		if e != nil {
			return out, e
		}
		limited := trim(out.Data.Bounces, &limit)
		limited = trim(out.Data.Domains, &limit) || limited
		limited = trim(out.Data.FblReport, &limit) || limited
		limited = trim(out.Data.UserUnsubscribe, &limit) || limited
		if limited {
			out.PartialRead = true
			return out, types.NewError(types.ResponseLimit)
		}
		return out, nil
	})
}

func checkedEmailQuery(req *core.Request, values map[string]*types.ProtectedValue) {
	checkedQuery(req, values)
	validate := req.Validate
	req.Validate = func() error {
		if e := validate(); e != nil {
			return e
		}
		for _, v := range values {
			if v == nil || !utf8.Valid(v.Reveal()) {
				return types.NewError(types.InvalidArgument)
			}
			value := string(v.Reveal())
			address, e := mail.ParseAddress(value)
			if e != nil || address.Address != value {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}
}
