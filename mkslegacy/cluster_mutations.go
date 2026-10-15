package mkslegacy

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

func (c *Client) UpdateCluster(ctx context.Context, ref types.ResourceRef, input ClusterUpdateSpec, options types.CallOptions) (types.Result[DetailedCluster], error) {
	body, e := requestBody("cluster", input)
	if input.Validate() != nil {
		e = types.NewError(types.InvalidArgument)
	}
	return mutateCluster(c, ctx, "mks.v1.clusters.replace", "PUT", "/clusters/"+url.PathEscape(ref.ID), ref, body, e, options)
}
func mutateCluster(c *Client, ctx context.Context, id, method, path string, ref types.ResourceRef, body []byte, inputErr error, options types.CallOptions) (types.Result[DetailedCluster], error) {
	op := core.Operation{ID: id, Service: "mks", APIGeneration: "v1", Method: method, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}, Rejected: []int{404}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Body: body, Refs: []types.ResourceRef{ref}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateRoot(ref, "cluster", op); e != nil {
			return e
		}
		return inputErr
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[DetailedCluster], error) {
		d := core.Decoded[DetailedCluster]{}
		var env struct {
			Cluster json.RawMessage `json:"cluster"`
		}
		var identity struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"project_id"`
			Region    *string `json:"region"`
		}
		if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env.Cluster, &identity) != nil || identity.ID == nil || identity.ProjectID == nil || identity.Region == nil {
			return d, types.NewError(types.ProtocolError)
		}
		if *identity.ID != ref.ID || *identity.ProjectID != c.scope.ProjectID || *identity.Region != c.scope.Pool {
			return d, types.NewError(types.ScopeMismatch)
		}
		d.Refs = []types.ResourceRef{ref}
		if json.Unmarshal(env.Cluster, &d.Data) != nil || d.Data.Validate() != nil {
			return d, types.NewError(types.ProtocolError)
		}
		copied := ref
		d.Data.Ref = &copied
		return d, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) UpgradeMinorVersion(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DetailedCluster], error) {
	return mutateCluster(c, ctx, "mks.v1.clusters_upgrade_minor_version.create", "POST", "/clusters/"+url.PathEscape(ref.ID)+"/upgrade-minor-version", ref, nil, nil, options)
}

func (c *Client) UpgradePatchVersion(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DetailedCluster], error) {
	return mutateCluster(c, ctx, "mks.v1.clusters_upgrade_patch_version.create", "POST", "/clusters/"+url.PathEscape(ref.ID)+"/upgrade-patch-version", ref, nil, nil, options)
}

func (c *Client) RotateCertificates(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	return acknowledge(c, ctx, "mks.v1.clusters_rotate_certs.create", "POST", "/clusters/"+url.PathEscape(ref.ID)+"/rotate-certs", []types.ResourceRef{ref}, nil, options, func(op core.Operation) error { return c.validateRoot(ref, "cluster", op) })
}
