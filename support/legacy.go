package support

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

type LegacyClient struct {
	runtime *core.Runtime
	scope   types.Scope
}

func (c *LegacyClient) operation(id, method string, rejected []int) (core.Operation, error) {
	return ticketOperation(c.runtime, c.scope, "v1", id, method, rejected)
}
func ticketOperation(runtime *core.Runtime, scope types.Scope, generation, id, method string, rejected []int) (core.Operation, error) {
	effect := "read"
	if method != "GET" {
		effect = "mutation"
	}
	op := core.Operation{ID: id, Service: "tickets", APIGeneration: generation, Method: method, Effect: effect, Auth: types.StaticToken, Alternatives: []types.AuthMode{types.IAMAccount}, Success: []int{200}, Rejected: append([]int{401}, rejected...)}
	if scope.ProjectID != "" || scope.Region != "" || scope.Pool != "" {
		return op, types.NewError(types.ScopeMismatch)
	}
	return runtime.BindDocumented(op, scope)
}
func (c *LegacyClient) Ticket(number types.Integer) (types.ResourceRef, error) {
	if _, e := types.ParseInteger(number.String()); e != nil {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("tickets.v1.tickets.get", "GET", nil)
	if e != nil {
		return types.ResourceRef{}, e
	}
	return *core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.scope, APIGeneration: "v1"}, "tickets", "ticket", number.String()), nil
}
func (c *LegacyClient) refError(ref types.ResourceRef, kind string) error {
	if ref.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if _, e := types.ParseInteger(ref.ID); e != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Service != "tickets" || ref.APIGeneration != "v1" || ref.Scope != c.scope || ref.Kind != kind {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.Parent != nil {
		if kind != "comment" {
			return types.NewError(types.InvalidArgument)
		}
		if e := c.refError(*ref.Parent, "ticket"); e != nil {
			return e
		}
	}
	op, e := c.operation("tickets.v1.tickets.get", "GET", nil)
	if e != nil {
		return e
	}
	if op.EndpointID != ref.EndpointID || op.FixedEndpoint != ref.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (c *LegacyClient) request(ref types.ResourceRef, kind string, bindErr error) core.Request {
	prefix := "/tickets/"
	if kind == "comment" {
		prefix = "/comments/"
	}
	return core.Request{Scope: c.scope, Path: prefix + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref, kind)
	}}
}
func legacyTicketDecode(expected string) core.Decoder[Ticket] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[Ticket], error) {
		out := core.Decoded[Ticket]{}
		ref, e := ticketIdentity(expected, raw, ep)
		if e != nil {
			return out, e
		}
		out.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = ref
		return out, out.Data.Validate()
	}
}
func (c *LegacyClient) ticketCall(ctx context.Context, id, method string, ref *types.ResourceRef, suffix string, spec interface{ Validate() error }, rejected []int, o types.CallOptions) (types.Result[Ticket], error) {
	op, bindErr := c.operation(id, method, rejected)
	request := core.Request{Scope: c.scope, Path: "/tickets", Validate: func() error { return bindErr }}
	expected := ""
	if ref != nil {
		request = c.request(*ref, "ticket", bindErr)
		expected = ref.ID
	}
	request.Path += suffix
	request.Options = o
	if spec != nil {
		body, e := wireBody(spec)
		request.Body = body
		validate := request.Validate
		request.Validate = func() error {
			if err := validate(); err != nil {
				return err
			}
			return e
		}
	}
	return core.DoJSON(c.runtime, ctx, op, request, legacyTicketDecode(expected))
}
func (c *LegacyClient) Create(ctx context.Context, spec TicketCreateArgs, o types.CallOptions) (types.Result[Ticket], error) {
	return c.ticketCall(ctx, "tickets.v1.tickets.create", "POST", nil, "", spec, []int{400, 404, 429}, o)
}
func (c *LegacyClient) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Ticket], error) {
	return c.ticketCall(ctx, "tickets.v1.tickets.get", "GET", &ref, "", nil, nil, o)
}
func (c *LegacyClient) Update(ctx context.Context, ref types.ResourceRef, spec TicketModifyArgs, o types.CallOptions) (types.Result[Ticket], error) {
	return c.ticketCall(ctx, "tickets.v1.tickets.update", "PATCH", &ref, "", spec, []int{400}, o)
}
func (c *LegacyClient) Close(ctx context.Context, ref types.ResourceRef, spec TicketCloseArgs, o types.CallOptions) (types.Result[Ticket], error) {
	return c.ticketCall(ctx, "tickets.v1.tickets.close", "POST", &ref, "/close", spec, []int{400}, o)
}
func (c *LegacyClient) Reopen(ctx context.Context, ref types.ResourceRef, spec TicketReopenArgs, o types.CallOptions) (types.Result[Ticket], error) {
	return c.ticketCall(ctx, "tickets.v1.tickets.reopen", "POST", &ref, "/reopen", spec, []int{400}, o)
}
func (c *LegacyClient) Comment(number types.Integer, parent *types.ResourceRef) (types.ResourceRef, error) {
	if _, e := types.ParseInteger(number.String()); e != nil {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	if parent != nil {
		if e := c.refError(*parent, "ticket"); e != nil {
			return types.ResourceRef{}, e
		}
	}
	op, e := c.operation("tickets.v1.comments.get", "GET", nil)
	if e != nil {
		return types.ResourceRef{}, e
	}
	ref := core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.scope, APIGeneration: "v1"}, "tickets", "comment", number.String())
	if parent != nil {
		copy := *parent
		ref.Parent = &copy
	}
	return *ref, nil
}
func commentDecode(expected string, parent *types.ResourceRef) core.Decoder[Comment] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[Comment], error) {
		out := core.Decoded[Comment]{}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return out, types.NewError(types.ProtocolError)
		}
		id := expected
		if wire, ok := fields["id"]; ok {
			var value types.Integer
			if json.Unmarshal(wire, &value) != nil {
				return out, types.NewError(types.ProtocolError)
			}
			if id != "" && value.String() != id {
				return out, types.NewError(types.ScopeMismatch)
			}
			id = value.String()
		}
		if id == "" {
			return out, types.NewError(types.ProtocolError)
		}
		if parent != nil {
			if wire, ok := fields["ticket_number"]; ok {
				var number types.Integer
				if json.Unmarshal(wire, &number) != nil {
					return out, types.NewError(types.ProtocolError)
				}
				if number.String() != parent.ID {
					return out, types.NewError(types.ScopeMismatch)
				}
			}
		}
		ref := core.Ref(ep, "tickets", "comment", id)
		if parent != nil {
			copy := *parent
			ref.Parent = &copy
		}
		out.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = ref
		return out, out.Data.Validate()
	}
}
func (c *LegacyClient) AddComment(ctx context.Context, ref types.ResourceRef, spec CommentCreateArgs, o types.CallOptions) (types.Result[Comment], error) {
	op, bindErr := c.operation("tickets.v1.comments.create", "POST", []int{400, 429})
	req := c.request(ref, "ticket", bindErr)
	req.Path += "/comments"
	req.Options = o
	body, bodyErr := wireBody(spec)
	req.Body = body
	validate := req.Validate
	req.Validate = func() error {
		if e := validate(); e != nil {
			return e
		}
		if spec.Body == nil && spec.Files == nil {
			return types.NewError(types.InvalidArgument)
		}
		return bodyErr
	}
	return core.DoJSON(c.runtime, ctx, op, req, commentDecode("", &ref))
}
func (c *LegacyClient) GetComment(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Comment], error) {
	op, bindErr := c.operation("tickets.v1.comments.get", "GET", nil)
	req := c.request(ref, "comment", bindErr)
	req.Options = o
	return core.DoJSON(c.runtime, ctx, op, req, commentDecode(ref.ID, ref.Parent))
}
func (c *LegacyClient) Comments(ctx context.Context, ref types.ResourceRef, options pagination.Options) (pagination.Collection[Comment], error) {
	op, bindErr := c.operation("tickets.v1.comments.list", "GET", nil)
	req := c.request(ref, "ticket", bindErr)
	req.Path += "/comments"
	it, e := core.FixedIterator(c.runtime, op, req, ref.ID, options, "items", func(row *Comment, ep types.EndpointBinding) error {
		if row.TicketNumber != nil && row.TicketNumber.String() != ref.ID {
			return types.NewError(types.ScopeMismatch)
		}
		if row.ID != nil {
			row.Ref = core.Ref(ep, "tickets", "comment", row.ID.String())
			parent := ref
			row.Ref.Parent = &parent
		}
		return row.Validate()
	})
	if e != nil {
		return pagination.Collection[Comment]{}, e
	}
	return it.Collect(ctx)
}
func (c *LegacyClient) vote(ctx context.Context, id, method string, ref types.ResourceRef, kind string, spec interface{ Validate() error }, o types.CallOptions) (types.Result[Vote], error) {
	op, bindErr := c.operation(id, method, []int{400})
	req := c.request(ref, kind, bindErr)
	req.Path += "/vote"
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
	return core.DoJSON(c.runtime, ctx, op, req, decodeTicketModel[Vote])
}
func (c *LegacyClient) RateTicket(ctx context.Context, ref types.ResourceRef, spec TicketVoteCreateArgs, o types.CallOptions) (types.Result[Vote], error) {
	return c.vote(ctx, "tickets.v1.tickets.vote", "PATCH", ref, "ticket", spec, o)
}
func (c *LegacyClient) RateComment(ctx context.Context, ref types.ResourceRef, spec CommentVoteCreateArgs, o types.CallOptions) (types.Result[Vote], error) {
	return c.vote(ctx, "tickets.v1.comments.vote", "POST", ref, "comment", spec, o)
}
func (c *LegacyClient) Topics(ctx context.Context, options pagination.Options) (pagination.Collection[Topic], error) {
	op, bindErr := c.operation("tickets.v1.topics.list", "GET", nil)
	req := core.Request{Scope: c.scope, Path: "/topics", Validate: func() error { return bindErr }}
	it, e := core.FixedIterator(c.runtime, op, req, nil, options, "items", func(row *Topic, _ types.EndpointBinding) error { return row.Validate() })
	if e != nil {
		return pagination.Collection[Topic]{}, e
	}
	return it.Collect(ctx)
}
