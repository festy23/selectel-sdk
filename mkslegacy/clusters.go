package mkslegacy

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

func (c *Client) GetCluster(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DetailedCluster], error) {
	op := core.Operation{ID: "mks.v1.clusters.get", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRoot(ref, "cluster", op)
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[DetailedCluster], error) {
		d := core.Decoded[DetailedCluster]{}
		var env struct {
			Cluster json.RawMessage `json:"cluster"`
		}
		if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env.Cluster, &d.Data) != nil || d.Data.Validate() != nil || d.Data.ID == nil || d.Data.ProjectID == nil || d.Data.Region == nil {
			return d, types.NewError(types.ProtocolError)
		}
		if *d.Data.ID != ref.ID || *d.Data.ProjectID != c.scope.ProjectID || *d.Data.Region != c.scope.Pool {
			return d, types.NewError(types.ScopeMismatch)
		}
		copied := ref
		d.Data.Ref = &copied
		return d, nil
	})
}
func (c *Client) validateRoot(ref types.ResourceRef, kind string, op core.Operation) error {
	if ref.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Service != "mks" || ref.APIGeneration != "v1" || ref.Scope != c.scope || ref.Kind != kind || ref.Parent != nil || ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}

func (c *Client) DeleteCluster(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	op := core.Operation{ID: "mks.v1.clusters.delete", Service: "mks", APIGeneration: "v1", Method: "DELETE", Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{204}, NoContentSuccess: []int{204}, Rejected: []int{404}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRoot(ref, "cluster", op)
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

// GetKubeconfig is a source gate: schema204/bodyless and official SDK raw payload
// disagree. No credentials or HTTP are used until that contract is reconciled.
func (c *Client) GetKubeconfig(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[types.ProtectedValue], error) {
	op := core.Operation{ID: "mks.v1.clusters_kubeconfig.list", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{204}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: resourcePath(ref) + "/kubeconfig", Refs: []types.ResourceRef{ref}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateRoot(ref, "cluster", op); e != nil {
			return e
		}
		return types.NewError(types.CapabilityUnavailable)
	}}, func([]byte, types.EndpointBinding) (core.Decoded[types.ProtectedValue], error) {
		return core.Decoded[types.ProtectedValue]{}, types.NewError(types.CapabilityUnavailable)
	})
}
