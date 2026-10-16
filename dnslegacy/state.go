package dnslegacy

import (
	"context"
	"encoding/json"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Domains) State(ctx context.Context, selector DomainSelector, o types.CallOptions) (types.Result[ZoneState], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.state.list", Service: "dns", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.StaticToken, Success: []int{200}}, c.scope)
	path, selectorErr := selector.path()
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[ZoneState], error) {
		var row ZoneState
		if json.Unmarshal(raw, &row) != nil || row.Validate() != nil {
			return core.Decoded[ZoneState]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[ZoneState]{Data: row}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/state", Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return selectorErr
	}}, decode)
}

func (c *Domains) SetState(ctx context.Context, ref types.ResourceRef, spec ZoneStateSpec, o types.CallOptions) (types.Result[struct{}], error) {
	return c.setState(ctx, domainTarget{ref: &ref}, spec, o)
}

// SetStateByName targets the caller's explicit name. No numeric resource identity
// is inferred from the 204 response, and unknown outcomes cannot recover by name.
func (c *Domains) SetStateByName(ctx context.Context, name types.ProtectedValue, spec ZoneStateSpec, o types.CallOptions) (types.Result[struct{}], error) {
	return c.setState(ctx, domainTarget{name: &name}, spec, o)
}
func (c *Domains) setState(ctx context.Context, target domainTarget, spec ZoneStateSpec, o types.CallOptions) (types.Result[struct{}], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.state.update", Service: "dns", APIGeneration: "v1", Method: "PATCH", Effect: "mutation", Auth: types.StaticToken, Success: []int{204}, NoContentSuccess: []int{204}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	path, refs, targetErr := target.request(c.scope, op)
	body, bodyErr := json.Marshal(spec)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if targetErr != nil {
			return targetErr
		}
		if spec.Disabled == nil {
			return types.NewError(types.InvalidArgument)
		}
		return bodyErr
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/state", Body: body, Refs: refs, Options: o, Validate: validate}, func(_ []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
