package mkslegacy

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) GetNode(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[Node], error) {
	op := core.Operation{ID: "mks.v1.nodes.get", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: resourcePath(ref), Refs: []types.ResourceRef{ref}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateNested(ref, "node", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[Node], error) {
		d := core.Decoded[Node]{}
		var env struct {
			Node json.RawMessage `json:"node"`
		}
		if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env.Node, &d.Data) != nil || d.Data.Validate() != nil || d.Data.ID == nil || d.Data.NodegroupID == nil {
			return d, types.NewError(types.ProtocolError)
		}
		if *d.Data.ID != ref.ID || *d.Data.NodegroupID != ref.Parent.ID {
			return d, types.NewError(types.ScopeMismatch)
		}
		owned := cloneRef(ref)
		d.Data.Ref = &owned
		return d, nil
	})
}

func (c *Client) DeleteNode(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	return acknowledge(c, ctx, "mks.v1.nodes.delete", "DELETE", resourcePath(ref), []types.ResourceRef{ref}, nil, options, func(op core.Operation) error { return c.validateNested(ref, "node", op) })
}

func (c *Client) ReinstallNode(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	return acknowledge(c, ctx, "mks.v1.nodes.reinstall", "POST", resourcePath(ref)+"/reinstall", []types.ResourceRef{ref}, nil, options, func(op core.Operation) error { return c.validateNested(ref, "node", op) })
}
