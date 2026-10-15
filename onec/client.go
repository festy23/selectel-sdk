// Package onec provides explicitly scoped Cloud for 1C API access.
package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client { return &Client{runtime: r, scope: s} }

// Endpoints reads the provider's regional endpoint inventory; it does not select
// a region or authorize sending a subsequent request to a discovered address.
func (c *Client) Endpoints(ctx context.Context, options types.CallOptions) (types.Result[GetServiceEndpointsResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.service_endpoints.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, FixedEndpoint: "https://api.selectel.ru/ones/paas/v2", EndpointID: "selectel-cloud-1c-discovery", Success: []int{200}}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/service/endpoints", Options: options}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetServiceEndpointsResponse], error) {
		var v GetServiceEndpointsResponse
		if json.Unmarshal(raw, &v) != nil || v.Endpoints == nil {
			return core.Decoded[GetServiceEndpointsResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[GetServiceEndpointsResponse]{Data: v}, nil
	})
}

func (c *Client) GetCluster(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[GetClusterResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.clusters.get", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
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
		if bindErr != nil {
			return bindErr
		}
		if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
			return types.NewError(types.EndpointUnavailable)
		}
		return nil
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[GetClusterResponse], error) {
		var identity struct {
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Cluster == nil || identity.Cluster.ID == nil {
			return core.Decoded[GetClusterResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Cluster.ID != ref.ID || (identity.Cluster.ProjectID != nil && *identity.Cluster.ProjectID != c.scope.ProjectID) {
			return core.Decoded[GetClusterResponse]{}, types.NewError(types.ScopeMismatch)
		}
		var v GetClusterResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Cluster == nil {
			return core.Decoded[GetClusterResponse]{}, types.NewError(types.ProtocolError)
		}
		v.Cluster.Ref = core.Ref(endpoint, "cloud_1c", "cluster", ref.ID)
		return core.Decoded[GetClusterResponse]{Data: v, Refs: []types.ResourceRef{*v.Cluster.Ref}}, nil
	})
}
