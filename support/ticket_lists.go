package support

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"strconv"
)

type TicketListOptions struct {
	TicketFilters
	Page              *types.Integer
	SortBy, SortOrder *string
}
type TicketListing struct {
	pagination.Collection[TicketItemResponse]
	Envelopes []GetMultipleTicketsResponse `json:"envelopes,omitempty"`
}

func ticketListQuery(spec TicketListOptions) (url.Values, error) {
	query, err := filtersQuery(spec.TicketFilters)
	sortValues, sortErr := sortQuery(spec.SortBy, spec.SortOrder)
	if sortErr != nil {
		err = sortErr
	}
	for k, v := range sortValues {
		query[k] = v
	}
	if spec.SortBy != nil {
		switch *spec.SortBy {
		case "created_at", "updated_at", "resolved_at", "closed_at":
		default:
			err = types.NewError(types.InvalidArgument)
		}
	}
	query.Set("page", "1")
	if spec.Page != nil {
		number, ok := new(big.Int).SetString(spec.Page.String(), 10)
		if !ok || number.Sign() < 1 {
			err = types.NewError(types.InvalidArgument)
		}
		query.Set("page", spec.Page.String())
	}
	return query, err
}
func (c *TicketsClient) List(ctx context.Context, spec TicketListOptions, options pagination.Options) (TicketListing, error) {
	result := TicketListing{}
	op, bindErr := c.operation("tickets.v2.tickets.list", "GET", []int{400, 404})
	query, queryErr := ticketListQuery(spec)
	if bindErr != nil {
		return result, bindErr
	}
	if queryErr != nil {
		return result, queryErr
	}
	plan, e := c.runtime.PagePlan(op.ID, "v2", c.scope, op.EndpointID, query.Encode())
	if e != nil {
		return result, e
	}
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[TicketItemResponse], error) {
		q := url.Values{}
		for k, v := range query {
			q[k] = append([]string(nil), v...)
		}
		q.Set("items_per_page", strconv.FormatUint(uint64(size), 10))
		out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/tickets", Query: q}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[GetMultipleTicketsResponse], error) {
			decoded := core.Decoded[GetMultipleTicketsResponse]{}
			if json.Unmarshal(raw, &decoded.Data) != nil {
				return decoded, types.NewError(types.ProtocolError)
			}
			if e := decoded.Data.Validate(); e != nil {
				return decoded, e
			}
			if decoded.Data.Page.String() != q.Get("page") {
				return decoded, types.NewError(types.ScopeMismatch)
			}
			seen := map[string]bool{}
			for i := range *decoded.Data.Items {
				row := &(*decoded.Data.Items)[i]
				if row.Number == nil {
					return decoded, types.NewError(types.ProtocolError)
				}
				id := row.Number.String()
				if seen[id] {
					return decoded, types.NewError(types.ProtocolError)
				}
				seen[id] = true
				row.Ref = core.Ref(ep, "tickets", "ticket", id)
			}
			return decoded, nil
		})
		page := pagination.SourcePage[TicketItemResponse]{Metadata: out.Metadata, Next: input}
		if e != nil {
			return page, e
		}
		page.Items = *out.Data.Items
		out.Data.Items = nil
		result.Envelopes = append(result.Envelopes, out.Data)
		return page, nil
	}
	it, e := pagination.New(plan, options, fetch)
	if e != nil {
		return result, e
	}
	result.Collection, e = it.Collect(ctx)
	return result, e
}
