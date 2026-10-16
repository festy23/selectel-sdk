package dnslegacy

import (
	"context"
	"encoding/json"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Domains) Update(ctx context.Context, ref types.ResourceRef, spec DomainUpdateSpec, o types.CallOptions) (types.Result[Domain], error) {
	return c.update(ctx, domainTarget{ref: &ref}, spec, o)
}
func (c *Domains) UpdateByName(ctx context.Context, name types.ProtectedValue, spec DomainUpdateSpec, o types.CallOptions) (types.Result[Domain], error) {
	return c.update(ctx, domainTarget{name: &name}, spec, o)
}
func (c *Domains) update(ctx context.Context, target domainTarget, spec DomainUpdateSpec, o types.CallOptions) (types.Result[Domain], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.secrets.update", Service: "dns", APIGeneration: "v1", Method: "PATCH", Effect: "mutation", Auth: types.StaticToken, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	path, refs, targetErr := target.request(c.scope, op)
	body, bodyErr := json.Marshal(spec)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if targetErr != nil {
			return targetErr
		}
		if spec.Tags == nil {
			return types.NewError(types.InvalidArgument)
		}
		for _, id := range spec.Tags {
			if _, e := types.ParseInteger(id.String()); e != nil {
				return types.NewError(types.InvalidArgument)
			}
		}
		return bodyErr
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Refs: refs, Body: body, Options: o, Validate: validate}, decodeDomain(c.scope, target))
}

func (c *Domains) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Acknowledgement], error) {
	return c.delete(ctx, domainTarget{ref: &ref}, o)
}
func (c *Domains) DeleteByName(ctx context.Context, name types.ProtectedValue, o types.CallOptions) (types.Result[Acknowledgement], error) {
	return c.delete(ctx, domainTarget{name: &name}, o)
}
func (c *Domains) delete(ctx context.Context, target domainTarget, o types.CallOptions) (types.Result[Acknowledgement], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.secrets.delete", Service: "dns", APIGeneration: "v1", Method: "DELETE", Effect: "mutation", Auth: types.StaticToken, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	path, refs, targetErr := target.request(c.scope, op)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Refs: refs, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return targetErr
	}}, acknowledgement)
}
