package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

func (c *Client) CreateCluster(ctx context.Context, spec CreateClusterRequest, options types.CallOptions) (types.Result[CreateClusterResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters", Body: body, Options: options, Validate: func() error {
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[CreateClusterResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Cluster == nil || identity.Cluster.ID == nil || *identity.Cluster.ID == "" {
			return core.Decoded[CreateClusterResponse]{}, types.NewError(types.ProtocolError)
		}
		if identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID {
			return core.Decoded[CreateClusterResponse]{}, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(endpoint, "cloud_1c", "cluster", *identity.Cluster.ID)
		out := core.Decoded[CreateClusterResponse]{Refs: []types.ResourceRef{*ref}}
		var v CreateClusterResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Cluster == nil {
			return out, types.NewError(types.ProtocolError)
		}
		v.Cluster.Ref = ref
		out.Data = v
		return out, nil
	})
}

// DeleteCluster never deletes linked infobases implicitly. The caller must
// satisfy the provider's documented prerequisite before requesting deletion.
func (c *Client) DeleteCluster(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DeleteClusterResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters.delete", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodDelete, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	validate := func() error {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
			return types.NewError(types.ScopeMismatch)
		}
		if ref.Kind != "cluster" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[DeleteClusterResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Cluster == nil || identity.Cluster.ID == nil {
			return core.Decoded[DeleteClusterResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Cluster.ID != ref.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID {
			return core.Decoded[DeleteClusterResponse]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[DeleteClusterResponse]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.Cluster == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Cluster.Ref = core.Ref(ep, "cloud_1c", "cluster", ref.ID)
		return out, nil
	})
}

func (c *Client) PatchCluster(ctx context.Context, ref types.ResourceRef, spec ClusterSpec, options types.CallOptions) (types.Result[PatchClusterResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters.update", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPatch, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
	validate := func() error {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
			return types.NewError(types.ScopeMismatch)
		}
		if ref.Kind != "cluster" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID), Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[PatchClusterResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Cluster == nil || identity.Cluster.ID == nil {
			return core.Decoded[PatchClusterResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Cluster.ID != ref.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID {
			return core.Decoded[PatchClusterResponse]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[PatchClusterResponse]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.Cluster == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Cluster.Ref = core.Ref(ep, "cloud_1c", "cluster", ref.ID)
		return out, nil
	})
}

func (c *Client) RestartCluster(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[RestartClusterResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_restart.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	validate := func() error {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
			return types.NewError(types.ScopeMismatch)
		}
		if ref.Kind != "cluster" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID) + "/restart", Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[RestartClusterResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Cluster == nil || identity.Cluster.ID == nil {
			return core.Decoded[RestartClusterResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Cluster.ID != ref.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID {
			return core.Decoded[RestartClusterResponse]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[RestartClusterResponse]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.Cluster == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Cluster.Ref = core.Ref(ep, "cloud_1c", "cluster", ref.ID)
		return out, nil
	})
}

func (c *Client) ChangeClusterConfig(ctx context.Context, ref types.ResourceRef, spec ChangeClusterConfigBody, options types.CallOptions) (types.Result[ChangeClusterConfigResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_config.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
	validate := func() error {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
			return types.NewError(types.ScopeMismatch)
		}
		if ref.Kind != "cluster" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID) + "/config", Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ChangeClusterConfigResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Cluster == nil || identity.Cluster.ID == nil {
			return core.Decoded[ChangeClusterConfigResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Cluster.ID != ref.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID {
			return core.Decoded[ChangeClusterConfigResponse]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[ChangeClusterConfigResponse]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.Cluster == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Cluster.Ref = core.Ref(ep, "cloud_1c", "cluster", ref.ID)
		return out, nil
	})
}

func (c *Client) ResizeCluster(ctx context.Context, ref types.ResourceRef, spec ResizeClusterBody, options types.CallOptions) (types.Result[ResizeClusterResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_resize.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
	validate := func() error {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
			return types.NewError(types.ScopeMismatch)
		}
		if ref.Kind != "cluster" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID) + "/resize", Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ResizeClusterResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Cluster == nil || identity.Cluster.ID == nil {
			return core.Decoded[ResizeClusterResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Cluster.ID != ref.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID {
			return core.Decoded[ResizeClusterResponse]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[ResizeClusterResponse]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.Cluster == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Cluster.Ref = core.Ref(ep, "cloud_1c", "cluster", ref.ID)
		return out, nil
	})
}

func (c *Client) UpgradeCluster(ctx context.Context, ref types.ResourceRef, spec UpgradeClusterBody, options types.CallOptions) (types.Result[UpgradeClusterResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters_upgrade.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
	validate := func() error {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
			return types.NewError(types.ScopeMismatch)
		}
		if ref.Kind != "cluster" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID) + "/upgrade", Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[UpgradeClusterResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Cluster == nil || identity.Cluster.ID == nil {
			return core.Decoded[UpgradeClusterResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Cluster.ID != ref.ID || identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID {
			return core.Decoded[UpgradeClusterResponse]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[UpgradeClusterResponse]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.Cluster == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Cluster.Ref = core.Ref(ep, "cloud_1c", "cluster", ref.ID)
		return out, nil
	})
}
