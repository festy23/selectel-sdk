package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) ListClusterFlavors(ctx context.Context, includeDeprecated *bool, options pagination.Options) (pagination.Collection[ClusterFlavor], error) {
	op := core.Operation{ID: "cloud_1c.v2.cluster_flavors.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return pagination.Collection[ClusterFlavor]{}, err
	}
	query := url.Values{}
	if includeDeprecated != nil {
		query.Set("includeDeprecated", strconv.FormatBool(*includeDeprecated))
	}
	iterator, err := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/cluster-flavors", Query: query}, query.Encode(), options, "flavors", func(v *ClusterFlavor, _ types.EndpointBinding) error { return v.Validate() })
	if err != nil {
		return pagination.Collection[ClusterFlavor]{}, err
	}
	return iterator.Collect(ctx)
}

func (c *Client) ListClusterPlatforms(ctx context.Context, includeDeprecated *bool, options pagination.Options) (pagination.Collection[ClusterPlatform], error) {
	op := core.Operation{ID: "cloud_1c.v2.cluster_platforms.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return pagination.Collection[ClusterPlatform]{}, err
	}
	query := url.Values{}
	if includeDeprecated != nil {
		query.Set("includeDeprecated", strconv.FormatBool(*includeDeprecated))
	}
	iterator, err := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/cluster-platforms", Query: query}, query.Encode(), options, "platforms", func(v *ClusterPlatform, _ types.EndpointBinding) error { return v.Validate() })
	if err != nil {
		return pagination.Collection[ClusterPlatform]{}, err
	}
	return iterator.Collect(ctx)
}

func (c *Client) GetClusterFlavorConstraints(ctx context.Context, options types.CallOptions) (types.Result[GetClusterFlavorConstraintsResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.cluster_flavors_constraints.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/cluster-flavors/constraints", Options: options, Validate: func() error { return bindErr }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetClusterFlavorConstraintsResponse], error) {
		var v GetClusterFlavorConstraintsResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Constraints == nil {
			return core.Decoded[GetClusterFlavorConstraintsResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[GetClusterFlavorConstraintsResponse]{Data: v}, nil
	})
}

func (c *Client) Regions(ctx context.Context, options pagination.Options) (pagination.Collection[Region], error) {
	op := core.Operation{ID: "cloud_1c.v2.service_regions.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return pagination.Collection[Region]{}, err
	}
	query := url.Values{}
	iterator, err := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/service/regions", Query: query}, query.Encode(), options, "regions", func(v *Region, _ types.EndpointBinding) error { return v.Validate() })
	if err != nil {
		return pagination.Collection[Region]{}, err
	}
	return iterator.Collect(ctx)
}

func (c *Client) GetServiceConfig(ctx context.Context, options types.CallOptions) (types.Result[GetServiceConfigResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.service_config.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/service/config", Options: options, Validate: func() error { return bindErr }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetServiceConfigResponse], error) {
		var v GetServiceConfigResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Config == nil {
			return core.Decoded[GetServiceConfigResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[GetServiceConfigResponse]{Data: v}, nil
	})
}
