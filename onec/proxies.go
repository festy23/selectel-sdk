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

func (c *Client) GetProjectIDForNetworkSharing(ctx context.Context, options types.CallOptions) (types.Result[GetProjectIDForNetworkSharingResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.cluster_proxies_network_share_project.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/cluster-proxies/network-share/project", Options: options, Validate: func() error { return bindErr }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetProjectIDForNetworkSharingResponse], error) {
		var v GetProjectIDForNetworkSharingResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.ProjectID == nil || *v.ProjectID == "" {
			return core.Decoded[GetProjectIDForNetworkSharingResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[GetProjectIDForNetworkSharingResponse]{Data: v}, nil
	})
}

func cloneResourceRef(ref *types.ResourceRef) *types.ResourceRef {
	if ref == nil {
		return nil
	}
	out := *ref
	out.Parent = cloneResourceRef(ref.Parent)
	return &out
}

func (c *Client) validateClusterAnchor(ref types.ResourceRef, op core.Operation) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != "cluster" || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.EndpointUnavailable)
	}
	return nil
}
func (c *Client) validateProxyRef(ref types.ResourceRef, op core.Operation) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != "cluster_proxy" {
		return types.NewError(types.InvalidArgument)
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.EndpointUnavailable)
	}
	if ref.Parent != nil {
		return c.validateClusterAnchor(*ref.Parent, op)
	}
	return nil
}

func proxyIdentity(value *ClusterProxy, scope types.Scope, expected string, parent *types.ResourceRef) error {
	if value == nil || value.ID == nil || *value.ID == "" || value.Spec == nil || value.Spec.ClusterID == nil || *value.Spec.ClusterID == "" {
		return types.NewError(types.ProtocolError)
	}
	if expected != "" && *value.ID != expected || value.ProjectID != nil && *value.ProjectID != scope.ProjectID {
		return types.NewError(types.ScopeMismatch)
	}
	if parent != nil && *value.Spec.ClusterID != parent.ID {
		return types.NewError(types.ScopeMismatch)
	}
	if value.Cluster != nil {
		if value.Cluster.ProjectID != nil && *value.Cluster.ProjectID != scope.ProjectID || value.Cluster.ID != nil && *value.Cluster.ID != *value.Spec.ClusterID {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}

func (c *Client) GetClusterProxy(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[GetClusterProxyResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.cluster_proxies.get", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	refErr := ref.Validate()
	if refErr == nil {
		ref.Parent = cloneResourceRef(ref.Parent)
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/cluster-proxies/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if refErr != nil {
			return refErr
		}
		if bindErr != nil {
			return bindErr
		}
		return c.validateProxyRef(ref, op)
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[GetClusterProxyResponse], error) {
		var v GetClusterProxyResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return core.Decoded[GetClusterProxyResponse]{}, types.NewError(types.ProtocolError)
		}
		if e := proxyIdentity(v.ClusterProxy, c.scope, ref.ID, ref.Parent); e != nil {
			return core.Decoded[GetClusterProxyResponse]{}, e
		}
		v.ClusterProxy.Ref = core.Ref(ep, "cloud_1c", "cluster_proxy", ref.ID)
		v.ClusterProxy.Ref.Parent = cloneResourceRef(ref.Parent)
		if ref.Parent != nil && v.ClusterProxy.Cluster != nil {
			v.ClusterProxy.Cluster.Ref = cloneResourceRef(ref.Parent)
		}
		return core.Decoded[GetClusterProxyResponse]{Data: v, Refs: []types.ResourceRef{*v.ClusterProxy.Ref}}, nil
	})
}

func scanCreatedProxy(raw []byte, scope types.Scope, parent types.ResourceRef) (string, error) {
	var identity struct {
		ClusterProxy *struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"projectId"`
			Spec      *struct {
				ClusterID *string `json:"clusterId"`
			} `json:"spec"`
			Cluster *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"cluster"`
		} `json:"clusterProxy"`
	}
	if json.Unmarshal(raw, &identity) != nil || identity.ClusterProxy == nil || identity.ClusterProxy.ID == nil || *identity.ClusterProxy.ID == "" {
		return "", types.NewError(types.ProtocolError)
	}
	value := identity.ClusterProxy
	if value.ProjectID != nil && *value.ProjectID != scope.ProjectID || value.Spec != nil && value.Spec.ClusterID != nil && *value.Spec.ClusterID != parent.ID || value.Cluster != nil && (value.Cluster.ID != nil && *value.Cluster.ID != parent.ID || value.Cluster.ProjectID != nil && *value.Cluster.ProjectID != scope.ProjectID) {
		return "", types.NewError(types.ScopeMismatch)
	}
	return *value.ID, nil
}

func (c *Client) CreateClusterProxy(ctx context.Context, cluster types.ResourceRef, spec CreateClusterProxyRequest, options types.CallOptions) (types.Result[CreateClusterProxyResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.cluster_proxies.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
	endpoint := types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
	out, err := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/cluster-proxies", Body: body, Options: options, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateClusterAnchor(cluster, op); e != nil {
			return e
		}
		if bodyErr != nil || spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if *spec.Spec.ClusterID != cluster.ID {
			return types.NewError(types.ScopeMismatch)
		}
		if spec.Spec.PublicAccess != nil && *spec.Spec.PublicAccess && (spec.Spec.NetworkID == nil || *spec.Spec.NetworkID == "") {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[CreateClusterProxyResponse], error) {
		id, e := scanCreatedProxy(raw, c.scope, cluster)
		if e != nil {
			return core.Decoded[CreateClusterProxyResponse]{}, e
		}
		ref := core.Ref(ep, "cloud_1c", "cluster_proxy", id)
		parent := cluster
		ref.Parent = &parent
		decoded := core.Decoded[CreateClusterProxyResponse]{Refs: []types.ResourceRef{*ref}}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		if e := proxyIdentity(decoded.Data.ClusterProxy, c.scope, id, &cluster); e != nil {
			return decoded, e
		}
		decoded.Data.ClusterProxy.Ref = cloneResourceRef(ref)
		if decoded.Data.ClusterProxy.Cluster != nil {
			decoded.Data.ClusterProxy.Cluster.Ref = cloneResourceRef(&cluster)
		}
		return decoded, nil
	})
	if out.Receipt != nil {
		known := false
		for _, ref := range out.Receipt.ResourceRefs {
			if ref.Kind == "cluster_proxy" {
				known = true
			}
		}
		if !known {
			out.Receipt.Recovery = "unavailable"
		}
	}
	return out, err
}

func (c *Client) DeleteClusterProxy(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DeleteClusterProxyResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.cluster_proxies.delete", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodDelete, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	refErr := ref.Validate()
	if refErr == nil {
		ref.Parent = cloneResourceRef(ref.Parent)
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/cluster-proxies/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if refErr != nil {
			return refErr
		}
		if bindErr != nil {
			return bindErr
		}
		return c.validateProxyRef(ref, op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[DeleteClusterProxyResponse], error) {
		var v DeleteClusterProxyResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return core.Decoded[DeleteClusterProxyResponse]{}, types.NewError(types.ProtocolError)
		}
		if e := proxyIdentity(v.ClusterProxy, c.scope, ref.ID, ref.Parent); e != nil {
			return core.Decoded[DeleteClusterProxyResponse]{}, e
		}
		v.ClusterProxy.Ref = cloneResourceRef(&ref)
		if ref.Parent != nil && v.ClusterProxy.Cluster != nil {
			v.ClusterProxy.Cluster.Ref = cloneResourceRef(ref.Parent)
		}
		return core.Decoded[DeleteClusterProxyResponse]{Data: v, Refs: []types.ResourceRef{ref}}, nil
	})
}

type ProxyListing struct {
	pagination.Collection[ClusterProxy]
	Envelopes []ListClusterProxiesResponse `json:"envelopes,omitempty"`
}

func (c *Client) ListClusterProxies(ctx context.Context, cluster *types.ResourceRef, options pagination.Options) (ProxyListing, error) {
	result := ProxyListing{}
	op := core.Operation{ID: "cloud_1c.v2.cluster_proxies.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return result, e
	}
	if cluster != nil {
		if e := c.validateClusterAnchor(*cluster, op); e != nil {
			return result, e
		}
		cluster = cloneResourceRef(cluster)
	}
	plan, e := c.runtime.PagePlan(op.ID, "v2", c.scope, op.EndpointID, cluster)
	if e != nil {
		return result, e
	}
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[ClusterProxy], error) {
		query := url.Values{"cursor.limit": {strconv.FormatUint(uint64(size), 10)}}
		if input != "" {
			query.Set("cursor.cursor", input)
		}
		if cluster != nil {
			query.Set("clusterId", cluster.ID)
		}
		in := core.Request{Scope: c.scope, Path: "/cluster-proxies", Query: query}
		if cluster != nil {
			in.Refs = []types.ResourceRef{*cluster}
			in.PinnedEndpoint = &types.EndpointBinding{ID: cluster.EndpointID, URL: cluster.EndpointURL, Scope: cluster.Scope, APIGeneration: cluster.APIGeneration, Audience: "selectel"}
		}
		out, e := core.DoJSON(c.runtime, ctx, op, in, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ListClusterProxiesResponse], error) {
			var v ListClusterProxiesResponse
			if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.ClusterProxies == nil {
				return core.Decoded[ListClusterProxiesResponse]{}, types.NewError(types.ProtocolError)
			}
			seen := map[string]bool{}
			for i := range v.ClusterProxies {
				row := &v.ClusterProxies[i]
				if e := proxyIdentity(row, c.scope, "", cluster); e != nil {
					return core.Decoded[ListClusterProxiesResponse]{}, e
				}
				if seen[*row.ID] {
					return core.Decoded[ListClusterProxiesResponse]{}, types.NewError(types.ProtocolError)
				}
				seen[*row.ID] = true
				row.Ref = core.Ref(ep, "cloud_1c", "cluster_proxy", *row.ID)
				row.Ref.Parent = cloneResourceRef(cluster)
				if cluster != nil && row.Cluster != nil {
					row.Cluster.Ref = cloneResourceRef(cluster)
				}
			}
			return core.Decoded[ListClusterProxiesResponse]{Data: v}, nil
		})
		page := pagination.SourcePage[ClusterProxy]{Metadata: out.Metadata, Next: input}
		if e != nil {
			return page, e
		}
		page.Items = out.Data.ClusterProxies
		out.Data.ClusterProxies = nil
		if out.Data.Cursor != nil && out.Data.Cursor.Next != nil {
			page.Next = *out.Data.Cursor.Next
		}
		result.Envelopes = append(result.Envelopes, out.Data)
		return page, nil
	}
	iterator, e := pagination.New(plan, options, fetch)
	if e != nil {
		return result, e
	}
	result.Collection, e = iterator.Collect(ctx)
	return result, e
}
