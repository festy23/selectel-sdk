// Package loadbalancers manages project-scoped Octavia resources.
package loadbalancers

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
)

type Client struct {
	runtime   *core.Runtime
	scope     types.Scope
	Listeners *Listeners
	Pools     *Pools
	Monitors  *Monitors
	Members   *Members
}

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{runtime: r, scope: s, Listeners: &Listeners{service{r, s}}, Pools: &Pools{service{r, s}}, Monitors: &Monitors{service{r, s}}, Members: &Members{service{r, s}}}
}
func (c *Client) Create(ctx context.Context, spec LoadBalancerCreate, opts types.CallOptions) (types.Result[LoadBalancer], error) {
	body, err := json.Marshal(map[string]any{"loadbalancer": spec})
	op := core.Operation{ID: "loadbalancers.v2.loadbalancers.create", Service: "loadbalancers", APIGeneration: "v2", ServiceType: "load-balancer", EndpointVersionPrefix: "/v2", EndpointVersionAliases: []string{"/v2.0"}, Method: "POST", Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{201}, Rejected: []int{400, 401, 403, 404, 409}}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/lbaas/loadbalancers", Body: body, Options: opts, Validate: func() error {
		if err != nil {
			return types.NewError(types.InvalidArgument)
		}
		if (spec.VIPPortID == nil || *spec.VIPPortID == "") && (spec.VIPNetworkID == nil || *spec.VIPNetworkID == "") && (spec.VIPSubnetID == nil || *spec.VIPSubnetID == "") {
			return types.NewError(types.InvalidArgument)
		}
		return validateInput(spec)
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[LoadBalancer], error) {
		return decodeResource[LoadBalancer](raw, ep, "loadbalancer", nil, nil)
	})
}

func operation(resource, action, method string, success ...int) core.Operation {
	effect := "mutation"
	if method == "GET" {
		effect = "read"
	}
	return core.Operation{ID: "loadbalancers.v2." + resource + "." + action, Service: "loadbalancers", APIGeneration: "v2", ServiceType: "load-balancer", EndpointVersionPrefix: "/v2", EndpointVersionAliases: []string{"/v2.0"}, Method: method, Effect: effect, Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: success, Rejected: []int{400, 401, 403, 404, 409}}
}
func (c *Client) valid(ref types.ResourceRef, kind string) error {
	if kind == "loadbalancer" && ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Scope != c.scope || ref.Service != "loadbalancers" || ref.APIGeneration != "v2" || ref.Kind != kind {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (c *Client) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[LoadBalancer], error) {
	return core.DoJSON(c.runtime, ctx, operation("loadbalancers", "get", "GET", 200), core.Request{Scope: c.scope, Path: "/v2/lbaas/loadbalancers/" + segment(ref.ID), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(&ref), Validate: func() error { return c.valid(ref, "loadbalancer") }}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[LoadBalancer], error) {
		return decodeResource[LoadBalancer](raw, ep, "loadbalancer", &ref, nil)
	})
}

type Empty struct{}

func (c *Client) Update(ctx context.Context, ref types.ResourceRef, spec LoadBalancerUpdate, o types.CallOptions) (types.Result[LoadBalancer], error) {
	body, e := json.Marshal(map[string]any{"loadbalancer": spec})
	return core.DoJSON(c.runtime, ctx, operation("loadbalancers", "update", "PUT", 200, 202), core.Request{Scope: c.scope, Path: "/v2/lbaas/loadbalancers/" + segment(ref.ID), Body: body, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(&ref), Validate: func() error {
		if e != nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := validateInput(spec); e != nil {
			return e
		}
		return c.valid(ref, "loadbalancer")
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[LoadBalancer], error) {
		return decodeResource[LoadBalancer](raw, ep, "loadbalancer", &ref, nil)
	})
}
func (c *Client) Delete(ctx context.Context, ref types.ResourceRef, cascade *bool, o types.CallOptions) (types.Result[Empty], error) {
	q := url.Values{}
	if cascade != nil {
		q.Set("cascade", strconv.FormatBool(*cascade))
	}
	return core.DoJSON(c.runtime, ctx, operation("loadbalancers", "delete", "DELETE", 204), core.Request{Scope: c.scope, Path: "/v2/lbaas/loadbalancers/" + segment(ref.ID), Query: q, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(&ref), Validate: func() error { return c.valid(ref, "loadbalancer") }}, func([]byte, types.EndpointBinding) (core.Decoded[Empty], error) { return core.Decoded[Empty]{}, nil })
}

func pin(ref *types.ResourceRef) *types.EndpointBinding {
	if ref == nil {
		return nil
	}
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
