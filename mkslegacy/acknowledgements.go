package mkslegacy

import (
	"bytes"
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

// acknowledge binds only the private compiled bodyless204 operations below.
func acknowledge(c *Client, ctx context.Context, id, method, path string, refs []types.ResourceRef, body []byte, options types.CallOptions, validate func(core.Operation) error) (types.Result[struct{}], error) {
	op := core.Operation{ID: id, Service: "mks", APIGeneration: "v1", Method: method, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{204}, NoContentSuccess: []int{204}, Rejected: []int{404}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Body: body, Refs: refs, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return validate(op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		if len(bytes.TrimSpace(raw)) != 0 {
			return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[struct{}]{}, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
