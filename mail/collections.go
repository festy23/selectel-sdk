package mail

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
)

type Listing[T, H any] struct {
	pagination.Collection[T]
	Envelopes []H `json:"envelopes,omitempty"`
}

// collect follows strictly advancing source offsets. The published schema does
// not define a terminal sentinel: absent/nonadvancing offsets remain incomplete.
func collect[T, H any](c *Client, ctx context.Context, id string, req core.Request, options pagination.Options, tokenName string, split func(*H, types.EndpointBinding) ([]T, *types.Integer, error)) (Listing[T, H], error) {
	result := Listing[T, H]{}
	op, bindErr := c.operation(id)
	validate := req.Validate
	req.Validate = func() error {
		if bindErr != nil {
			return bindErr
		}
		if validate != nil {
			return validate()
		}
		return nil
	}
	if e := req.Validate(); e != nil {
		return result, e
	}
	if req.Query == nil {
		req.Query = url.Values{}
	}
	plan, e := c.runtime.PagePlan(op.ID, "v1", c.scope, op.EndpointID, struct{ Path, Query string }{req.Path, req.Query.Encode()})
	if e != nil {
		return result, e
	}
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[T], error) {
		page := req
		page.Query = cloneQuery(req.Query)
		offset := input
		if offset == "" {
			offset = "0"
		}
		page.Query.Set(tokenName, offset)
		page.Query.Set("limit", new(big.Int).SetUint64(uint64(size)).String())
		var rows []T
		var next *types.Integer
		out, err := core.DoJSON(c.runtime, ctx, op, page, func(raw []byte, ep types.EndpointBinding) (core.Decoded[H], error) {
			decoded, e := decode[H](raw, ep)
			if e != nil {
				return decoded, e
			}
			rows, next, e = split(&decoded.Data, ep)
			return decoded, e
		})
		source := pagination.SourcePage[T]{Items: rows, Metadata: out.Metadata, Next: input}
		if err != nil {
			return source, err
		}
		result.Envelopes = append(result.Envelopes, out.Data)
		if next != nil {
			n, ok := new(big.Int).SetString(next.String(), 10)
			old, _ := new(big.Int).SetString(offset, 10)
			if !ok || n.Sign() < 0 {
				return source, types.NewError(types.ProtocolError)
			}
			if n.Cmp(old) > 0 {
				source.Next = next.String()
			} else {
				source.Next = input
			}
		}
		return source, nil
	}
	it, e := pagination.New(plan, options, fetch)
	if e != nil {
		return result, e
	}
	result.Collection, e = it.Collect(ctx)
	return result, e
}
func cloneQuery(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		out[k] = append([]string(nil), v...)
	}
	return out
}
func (c *Client) ListResources(ctx context.Context, options pagination.Options) (Listing[ResourceWithCredentialsAndDomains, ListResources], error) {
	return collect(c, ctx, "listResources", core.Request{Scope: c.scope, Path: "/resources"}, options, "offset", func(data *ListResources, ep types.EndpointBinding) ([]ResourceWithCredentialsAndDomains, *types.Integer, error) {
		if data.Resources == nil {
			return nil, nil, types.NewError(types.ProtocolError)
		}
		rows := *data.Resources
		seen := map[string]bool{}
		for i := range rows {
			row := ResourceWithCredentials{ID: rows[i].ID, ProjectID: rows[i].ProjectID}
			_, e := identity(&row, "", ep)
			if e != nil {
				return nil, nil, e
			}
			if seen[row.Ref.ID] {
				return nil, nil, types.NewError(types.ProtocolError)
			}
			seen[row.Ref.ID] = true
			rows[i].Ref = row.Ref
		}
		data.Resources = nil
		return rows, data.NextOffset, nil
	})
}

func (c *Client) ListDomains(ctx context.Context, ref types.ResourceRef, options pagination.Options) (Listing[types.ProtectedValue, ListDomains], error) {
	op, err := c.operation("retireveResourceDomains")
	_ = op
	req := c.request(ref, err)
	req.Path += "/domains"
	return collect(c, ctx, "retireveResourceDomains", req, options, "offset", func(data *ListDomains, _ types.EndpointBinding) ([]types.ProtectedValue, *types.Integer, error) {
		if data.Domains == nil {
			return nil, nil, types.NewError(types.ProtocolError)
		}
		rows := *data.Domains
		data.Domains = nil
		return rows, data.NextOffset, nil
	})
}
func (c *Client) reportRequest(id string, ref types.ResourceRef, period Period) core.Request {
	_, err := c.operation(id)
	req := c.request(ref, err)
	req.Path += "/blist/report/"
	if id == "getMessageComplaintsByPeriod" {
		req.Path += "fbl"
	} else {
		req.Path += "non-delivery"
	}
	req.Query = url.Values{"date_from": {period.DateFrom}, "date_to": {period.DateTo}}
	validate := req.Validate
	req.Validate = func() error {
		if e := validate(); e != nil {
			return e
		}
		return period.validate()
	}
	return req
}
func (c *Client) ListComplaints(ctx context.Context, ref types.ResourceRef, period Period, options pagination.Options) (Listing[Complaint, ComplaintsByPeriod], error) {
	return collect(c, ctx, "getMessageComplaintsByPeriod", c.reportRequest("getMessageComplaintsByPeriod", ref, period), options, "cursor_next", func(data *ComplaintsByPeriod, _ types.EndpointBinding) ([]Complaint, *types.Integer, error) {
		if data.Report == nil {
			return nil, nil, types.NewError(types.ProtocolError)
		}
		rows := *data.Report
		data.Report = nil
		var next *types.Integer
		if data.Pagination != nil {
			next = data.Pagination.CursorNext
		}
		return rows, next, nil
	})
}
func (c *Client) ListUndelivered(ctx context.Context, ref types.ResourceRef, period Period, options pagination.Options) (Listing[NonDelivered, NonDeliveredByPeriod], error) {
	return collect(c, ctx, "getUndeliveredMessagesByPeriod", c.reportRequest("getUndeliveredMessagesByPeriod", ref, period), options, "cursor_next", func(data *NonDeliveredByPeriod, _ types.EndpointBinding) ([]NonDelivered, *types.Integer, error) {
		if data.Report == nil {
			return nil, nil, types.NewError(types.ProtocolError)
		}
		rows := *data.Report
		data.Report = nil
		var next *types.Integer
		if data.Pagination != nil {
			next = data.Pagination.CursorNext
		}
		return rows, next, nil
	})
}
