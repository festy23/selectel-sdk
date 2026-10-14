package loadbalancers

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
)

func diagnostic[T any](s service, ctx context.Context, ref types.ResourceRef, resource, kind, action, path, field string, o types.CallOptions, validate func(*T) error) (types.Result[T], error) {
	return core.DoJSON(s.runtime, ctx, operation(resource, action, "GET", 200), core.Request{Scope: s.scope, Path: "/v2/lbaas/" + resource + "/" + segment(ref.ID) + path, Refs: []types.ResourceRef{ref}, Options: o, Validate: func() error { return s.valid(ref, kind) }}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[T], error) {
		var env map[string]json.RawMessage
		var v *T
		if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env[field], &v) != nil || v == nil {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		if validate != nil {
			if e := validate(v); e != nil {
				return core.Decoded[T]{}, e
			}
		}
		return core.Decoded[T]{Data: *v}, nil
	})
}
func (c *Client) Statistics(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[LoadBalancerStatisticsResponse], error) {
	return diagnostic[LoadBalancerStatisticsResponse](service{c.runtime, c.scope}, ctx, ref, "loadbalancers", "loadbalancer", "statistics", "/stats", "stats", o, nil)
}
func (c *Listeners) Statistics(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[LoadBalancerStatisticsResponse], error) {
	return diagnostic[LoadBalancerStatisticsResponse](c.service, ctx, ref, "listeners", "listener", "statistics", "/stats", "stats", o, nil)
}
func (c *Client) StatusTree(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[StatusResponse], error) {
	return diagnostic[StatusResponse](service{c.runtime, c.scope}, ctx, ref, "loadbalancers", "loadbalancer", "statuses", "/status", "statuses", o, func(v *StatusResponse) error {
		if v.Loadbalancer == nil || v.Loadbalancer.ID != ref.ID {
			return types.NewError(types.ProtocolError)
		}
		return linkStatus(v, ref)
	})
}

func (c *Client) Failover(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return core.DoJSON(c.runtime, ctx, operation("loadbalancers", "failover", "PUT", 202), core.Request{Scope: c.scope, Path: "/v2/lbaas/loadbalancers/" + segment(ref.ID) + "/failover", Refs: []types.ResourceRef{ref}, Options: o, Validate: func() error { return c.valid(ref, "loadbalancer") }}, func([]byte, types.EndpointBinding) (core.Decoded[Empty], error) { return core.Decoded[Empty]{}, nil })
}

// Replace has provider replacement semantics; absent members are deleted. No atomicity is promised.
func (c *Members) BatchUpdate(ctx context.Context, pool types.ResourceRef, members []MemberCreate, additiveOnly *bool, o types.CallOptions) (types.Result[Empty], error) {
	q := url.Values{}
	if additiveOnly != nil {
		q.Set("additive_only", strconv.FormatBool(*additiveOnly))
	}
	body, e := json.Marshal(map[string]any{"members": members})
	return core.DoJSON(c.runtime, ctx, operation("members", "batch_update", "PUT", 202), core.Request{Scope: c.scope, Path: "/v2/lbaas/pools/" + segment(pool.ID) + "/members", Query: q, Body: body, Refs: []types.ResourceRef{pool}, Options: o, Validate: func() error {
		if e != nil || members == nil {
			return types.NewError(types.InvalidArgument)
		}
		for _, m := range members {
			if e := validateInput(m); e != nil {
				return e
			}
		}
		return c.service.valid(pool, "pool")
	}}, func([]byte, types.EndpointBinding) (core.Decoded[Empty], error) { return core.Decoded[Empty]{}, nil })
}

func (c *Members) Replace(ctx context.Context, pool types.ResourceRef, members []MemberCreate, o types.CallOptions) (types.Result[Empty], error) {
	no := false
	return c.BatchUpdate(ctx, pool, members, &no, o)
}

func linkStatus(v *StatusResponse, ref types.ResourceRef) error {
	v.Loadbalancer.Ref = &ref
	for i := range v.Loadbalancer.Listeners {
		li := &v.Loadbalancer.Listeners[i]
		if li.ID == "" {
			return types.NewError(types.ProtocolError)
		}
		li.Ref = childRef(&ref, "listener", li.ID)
		for j := range li.Pools {
			p := &li.Pools[j]
			if p.ID == "" {
				return types.NewError(types.ProtocolError)
			}
			p.Ref = childRef(&ref, "pool", p.ID)
			for k := range p.Members {
				m := &p.Members[k]
				if m.ID == "" {
					return types.NewError(types.ProtocolError)
				}
				m.Ref = childRef(p.Ref, "member", m.ID)
			}
			if p.HealthMonitor != nil {
				if p.HealthMonitor.ID == "" {
					return types.NewError(types.ProtocolError)
				}
				p.HealthMonitor.Ref = childRef(p.Ref, "healthmonitor", p.HealthMonitor.ID)
			}
		}
	}
	return nil
}
