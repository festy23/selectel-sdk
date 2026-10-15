package support

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

type TicketsClient struct {
	runtime *core.Runtime
	scope   types.Scope
}

func (c *TicketsClient) operation(id, method string, rejected []int) (core.Operation, error) {
	return ticketOperation(c.runtime, c.scope, "v2", id, method, rejected)
}
func (c *TicketsClient) Ticket(number types.Integer) (types.ResourceRef, error) {
	if _, e := types.ParseInteger(number.String()); e != nil {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("tickets.v2.tickets.get", "GET", nil)
	if e != nil {
		return types.ResourceRef{}, e
	}
	return *core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.scope, APIGeneration: "v2"}, "tickets", "ticket", number.String()), nil
}
func (c *TicketsClient) refError(ref types.ResourceRef) error {
	if ref.Validate() != nil || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	if _, e := types.ParseInteger(ref.ID); e != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Service != "tickets" || ref.APIGeneration != "v2" || ref.Kind != "ticket" || ref.Scope != c.scope {
		return types.NewError(types.ScopeMismatch)
	}
	op, e := c.operation("tickets.v2.tickets.get", "GET", nil)
	if e != nil {
		return e
	}
	if op.EndpointID != ref.EndpointID || op.FixedEndpoint != ref.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func ticketIdentity(expected string, raw []byte, ep types.EndpointBinding) (*types.ResourceRef, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, types.NewError(types.ProtocolError)
	}
	id := expected
	if wire, ok := fields["number"]; ok {
		var number types.Integer
		if json.Unmarshal(wire, &number) != nil {
			return nil, types.NewError(types.ProtocolError)
		}
		if expected != "" && number.String() != expected {
			return nil, types.NewError(types.ScopeMismatch)
		}
		id = number.String()
	}
	if id == "" {
		return nil, types.NewError(types.ProtocolError)
	}
	return core.Ref(ep, "tickets", "ticket", id), nil
}
func (c *TicketsClient) request(ref types.ResourceRef, bindErr error) core.Request {
	return core.Request{Scope: c.scope, Path: "/v2/tickets/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref)
	}}
}
func (c *TicketsClient) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[GetTicketResponse], error) {
	op, bindErr := c.operation("tickets.v2.tickets.get", "GET", nil)
	req := c.request(ref, bindErr)
	req.Options = o
	return core.DoJSON(c.runtime, ctx, op, req, func(raw []byte, ep types.EndpointBinding) (core.Decoded[GetTicketResponse], error) {
		out := core.Decoded[GetTicketResponse]{}
		known, e := ticketIdentity(ref.ID, raw, ep)
		if e != nil {
			return out, e
		}
		out.Refs = []types.ResourceRef{*known}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = known
		return out, out.Data.Validate()
	})
}

func (c *TicketsClient) Create(ctx context.Context, spec CreateTicketRequestData, o types.CallOptions) (types.Result[CreateTicketResponse], error) {
	op, bindErr := c.operation("tickets.v2.tickets.create", "POST", []int{400, 404, 429})
	body, bodyErr := wireBody(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/tickets", Body: body, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return bodyErr
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[CreateTicketResponse], error) {
		out := core.Decoded[CreateTicketResponse]{}
		ref, e := ticketIdentity("", raw, ep)
		if e != nil {
			return out, e
		}
		out.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = ref
		return out, out.Data.Validate()
	})
}

func (c *TicketsClient) Update(ctx context.Context, ref types.ResourceRef, spec UpdateTicketRequest, o types.CallOptions) (types.Result[UpdateTicketResponse], error) {
	op, bindErr := c.operation("tickets.v2.tickets.update", "PATCH", []int{404})
	req := c.request(ref, bindErr)
	req.Options = o
	body, bodyErr := wireBody(spec)
	req.Body = body
	validate := req.Validate
	req.Validate = func() error {
		if e := validate(); e != nil {
			return e
		}
		return bodyErr
	}
	return core.DoJSON(c.runtime, ctx, op, req, func(raw []byte, ep types.EndpointBinding) (core.Decoded[UpdateTicketResponse], error) {
		out := core.Decoded[UpdateTicketResponse]{}
		known, e := ticketIdentity(ref.ID, raw, ep)
		if e != nil {
			return out, e
		}
		out.Refs = []types.ResourceRef{*known}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = known
		return out, out.Data.Validate()
	})
}
