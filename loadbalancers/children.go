package loadbalancers

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type service struct {
	runtime *core.Runtime
	scope   types.Scope
}
type Listeners struct{ service }
type Pools struct{ service }
type Monitors struct{ service }
type Members struct{ service }

func (s service) valid(ref types.ResourceRef, kind string) error {
	c := Client{runtime: s.runtime, scope: s.scope}
	if e := c.valid(ref, kind); e != nil {
		return e
	}
	if ref.Parent != nil {
		parentKind := "loadbalancer"
		if kind == "member" || kind == "healthmonitor" {
			parentKind = "pool"
		}
		if e := s.valid(*ref.Parent, parentKind); e != nil {
			return e
		}
		if ref.Parent.EndpointID != ref.EndpointID || ref.Parent.EndpointURL != ref.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
	}
	if kind == "member" && ref.Parent == nil {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func parent(ep types.EndpointBinding, kind, id string, known *types.ResourceRef) (*types.ResourceRef, error) {
	if id == "" {
		return nil, types.NewError(types.ProtocolError)
	}
	if known != nil {
		if known.Kind != kind || known.ID != id {
			return nil, types.NewError(types.ProtocolError)
		}
		b, _ := json.Marshal(known)
		var p types.ResourceRef
		json.Unmarshal(b, &p)
		return &p, nil
	}
	return core.Ref(ep, "loadbalancers", kind, id), nil
}
func singleton(ids []IDOnly) string {
	if len(ids) == 1 {
		return ids[0].ID
	}
	return ""
}
func attach[T any](v *T, ep types.EndpointBinding, kind string, expected, parentRef *types.ResourceRef, projected ...bool) (*types.ResourceRef, error) {
	var id string
	var target **types.ResourceRef
	var parentIDs []IDOnly
	parentKind := "loadbalancer"
	switch x := any(v).(type) {
	case *LoadBalancer:
		id = x.ID
		target = &x.Ref
	case *Member:
		id = x.ID
		target = &x.Ref
		parentKind = "pool"
		if parentRef == nil && expected != nil {
			parentRef = expected.Parent
		}
		if parentRef != nil {
			parentIDs = []IDOnly{{ID: parentRef.ID}}
		}
	case *Monitor:
		id = x.ID
		target = &x.Ref
		parentIDs = x.Pools
		parentKind = "pool"
	case *Pool:
		id = x.ID
		target = &x.Ref
		parentIDs = x.Loadbalancers
	case *Listener:
		id = x.ID
		target = &x.Ref
		parentIDs = x.Loadbalancers
	default:
		return nil, types.NewError(types.ProtocolError)
	}
	*target = nil
	if id == "" && len(projected) > 0 && projected[0] {
		return nil, nil
	}
	if id == "" || expected != nil && id != expected.ID {
		return nil, types.NewError(types.ProtocolError)
	}
	ref := core.Ref(ep, "loadbalancers", kind, id)
	if parentRef == nil && expected != nil {
		parentRef = expected.Parent
	}
	if kind == "pool" && parentRef != nil && parentRef.Kind == "listener" {
		if x, ok := any(v).(*Pool); ok {
			found := false
			for _, li := range x.Listeners {
				if li.ID == parentRef.ID {
					found = true
				}
			}
			if !found {
				return nil, types.NewError(types.ProtocolError)
			}
		}
		parentRef = parentRef.Parent
	}
	if kind == "loadbalancer" {
		*target = ref
		if x, ok := any(v).(*LoadBalancer); ok {
			return ref, linkNested(x)
		}
		return ref, nil
	}
	if len(parentIDs) == 0 && len(projected) > 0 && projected[0] {
		return nil, nil
	}
	p, e := parent(ep, parentKind, singleton(parentIDs), parentRef)
	if e != nil {
		return nil, e
	}
	ref.Parent = p
	*target = ref
	if x, ok := any(v).(*Pool); ok {
		linkPoolChildren(x)
	}
	return ref, nil
}
func one[T any](s service, ctx context.Context, resource, kind, action, method string, statuses []int, ref, parentRef *types.ResourceRef, spec any, o types.CallOptions) (types.Result[T], error) {
	path := "/v2/lbaas/" + resource
	if kind == "member" {
		p := parentRef
		if ref != nil {
			p = ref.Parent
		}
		if p != nil {
			path = "/v2/lbaas/pools/" + segment(p.ID) + "/members"
		}
	}
	if ref != nil {
		path += "/" + segment(ref.ID)
	}
	var body []byte
	var bodyErr error
	if spec != nil {
		raw, e := json.Marshal(spec)
		bodyErr = e
		var fields map[string]json.RawMessage
		if e == nil {
			bodyErr = json.Unmarshal(raw, &fields)
		}
		if parentRef != nil && kind != "member" {
			key := "loadbalancer_id"
			if parentRef.Kind == "listener" {
				key = "listener_id"
			}
			if parentRef.Kind == "pool" {
				key = "pool_id"
			}
			fields[key], _ = json.Marshal(parentRef.ID)
		}
		body, bodyErr = json.Marshal(map[string]any{kind: fields})
	}
	pinned := pin(ref)
	if ref == nil {
		pinned = pin(parentRef)
	}
	refs := []types.ResourceRef{}
	if ref != nil {
		refs = append(refs, *ref)
	} else if parentRef != nil {
		refs = append(refs, *parentRef)
	}
	return core.DoJSON(s.runtime, ctx, operation(resource, action, method, statuses...), core.Request{Scope: s.scope, Path: path, Body: body, Refs: refs, PinnedEndpoint: pinned, Options: o, Validate: func() error {
		if spec != nil {
			if e := validateInput(spec); e != nil {
				return e
			}
		}
		if bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		if ref != nil {
			return s.valid(*ref, kind)
		}
		if parentRef != nil {
			expectedParent := "loadbalancer"
			if resource == "healthmonitors" || resource == "members" {
				expectedParent = "pool"
			}
			if resource == "pools" && parentRef.Kind == "listener" {
				expectedParent = "listener"
			}
			return s.valid(*parentRef, expectedParent)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[T], error) {
		return decodeResource[T](raw, ep, kind, ref, parentRef)
	})
}
func remove(s service, ctx context.Context, resource, kind string, ref types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	path := "/v2/lbaas/" + resource + "/" + segment(ref.ID)
	if kind == "member" && ref.Parent != nil {
		path = "/v2/lbaas/pools/" + segment(ref.Parent.ID) + "/members/" + segment(ref.ID)
	}
	return core.DoJSON(s.runtime, ctx, operation(resource, "delete", "DELETE", 204), core.Request{Scope: s.scope, Path: path, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(&ref), Options: o, Validate: func() error { return s.valid(ref, kind) }}, func([]byte, types.EndpointBinding) (core.Decoded[Empty], error) { return core.Decoded[Empty]{}, nil })
}
func (c *Listeners) Create(ctx context.Context, lb types.ResourceRef, spec ListenerCreate, o types.CallOptions) (types.Result[Listener], error) {
	return one[Listener](c.service, ctx, "listeners", "listener", "create", "POST", []int{201}, nil, &lb, spec, o)
}
func (c *Listeners) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Listener], error) {
	return one[Listener](c.service, ctx, "listeners", "listener", "get", "GET", []int{200}, &ref, nil, nil, o)
}
func (c *Listeners) Update(ctx context.Context, ref types.ResourceRef, spec ListenerUpdate, o types.CallOptions) (types.Result[Listener], error) {
	return one[Listener](c.service, ctx, "listeners", "listener", "update", "PUT", []int{200, 202}, &ref, nil, spec, o)
}
func (c *Listeners) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return remove(c.service, ctx, "listeners", "listener", ref, o)
}

func (c *Pools) Create(ctx context.Context, p types.ResourceRef, spec PoolCreate, o types.CallOptions) (types.Result[Pool], error) {
	return one[Pool](c.service, ctx, "pools", "pool", "create", "POST", []int{201}, nil, &p, spec, o)
}
func (c *Pools) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Pool], error) {
	return one[Pool](c.service, ctx, "pools", "pool", "get", "GET", []int{200}, &ref, nil, nil, o)
}
func (c *Pools) Update(ctx context.Context, ref types.ResourceRef, spec PoolUpdate, o types.CallOptions) (types.Result[Pool], error) {
	return one[Pool](c.service, ctx, "pools", "pool", "update", "PUT", []int{200, 202}, &ref, nil, spec, o)
}
func (c *Pools) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return remove(c.service, ctx, "pools", "pool", ref, o)
}

func (c *Monitors) Create(ctx context.Context, p types.ResourceRef, spec MonitorCreate, o types.CallOptions) (types.Result[Monitor], error) {
	return one[Monitor](c.service, ctx, "healthmonitors", "healthmonitor", "create", "POST", []int{201}, nil, &p, spec, o)
}
func (c *Monitors) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Monitor], error) {
	return one[Monitor](c.service, ctx, "healthmonitors", "healthmonitor", "get", "GET", []int{200}, &ref, nil, nil, o)
}
func (c *Monitors) Update(ctx context.Context, ref types.ResourceRef, spec MonitorUpdate, o types.CallOptions) (types.Result[Monitor], error) {
	return one[Monitor](c.service, ctx, "healthmonitors", "healthmonitor", "update", "PUT", []int{200, 202}, &ref, nil, spec, o)
}
func (c *Monitors) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return remove(c.service, ctx, "healthmonitors", "healthmonitor", ref, o)
}

func (c *Members) Create(ctx context.Context, p types.ResourceRef, spec MemberCreate, o types.CallOptions) (types.Result[Member], error) {
	return one[Member](c.service, ctx, "members", "member", "create", "POST", []int{201}, nil, &p, spec, o)
}
func (c *Members) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Member], error) {
	return one[Member](c.service, ctx, "members", "member", "get", "GET", []int{200}, &ref, nil, nil, o)
}
func (c *Members) Update(ctx context.Context, ref types.ResourceRef, spec MemberUpdate, o types.CallOptions) (types.Result[Member], error) {
	return one[Member](c.service, ctx, "members", "member", "update", "PUT", []int{200, 201, 202}, &ref, nil, spec, o)
}
func (c *Members) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return remove(c.service, ctx, "members", "member", ref, o)
}

func childRef(p *types.ResourceRef, kind, id string) *types.ResourceRef {
	if id == "" || p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	var parent types.ResourceRef
	json.Unmarshal(b, &parent)
	r := parent
	r.Kind = kind
	r.ID = id
	r.Parent = &parent
	return &r
}
func linkPoolChildren(p *Pool) {
	for i := range p.Members {
		p.Members[i].Ref = childRef(p.Ref, "member", p.Members[i].ID)
	}
	if p.Healthmonitor != nil {
		p.Healthmonitor.Ref = childRef(p.Ref, "healthmonitor", p.Healthmonitor.ID)
	}
}
func linkNested(lb *LoadBalancer) error {
	for i := range lb.Listeners {
		lb.Listeners[i].Ref = childRef(lb.Ref, "listener", lb.Listeners[i].ID)
	}
	for i := range lb.Pools {
		lb.Pools[i].Ref = childRef(lb.Ref, "pool", lb.Pools[i].ID)
		linkPoolChildren(&lb.Pools[i])
	}
	return nil
}

// decodeResource validates identity and addressing relationships independently
// from optional payload fields. An accepted response can prove identity even
// when its remaining DTO is malformed; foreign or malformed identities cannot.
func decodeResource[T any](raw []byte, ep types.EndpointBinding, kind string, expected, parentRef *types.ResourceRef) (core.Decoded[T], error) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil {
		return core.Decoded[T]{}, types.NewError(types.ProtocolError)
	}
	body := envelope[kind]
	var fields map[string]json.RawMessage
	var id string
	if json.Unmarshal(body, &fields) != nil || json.Unmarshal(fields["id"], &id) != nil || id == "" {
		return core.Decoded[T]{}, types.NewError(types.ProtocolError)
	}
	var minimal T
	var relationErr error
	switch x := any(&minimal).(type) {
	case *LoadBalancer:
		x.ID = id
	case *Listener:
		x.ID = id
		relationErr = json.Unmarshal(fields["loadbalancers"], &x.Loadbalancers)
	case *Pool:
		x.ID = id
		relationErr = json.Unmarshal(fields["loadbalancers"], &x.Loadbalancers)
		if relationErr == nil && parentRef != nil && parentRef.Kind == "listener" {
			relationErr = json.Unmarshal(fields["listeners"], &x.Listeners)
		}
	case *Monitor:
		x.ID = id
		relationErr = json.Unmarshal(fields["pools"], &x.Pools)
	case *Member:
		x.ID = id
	default:
		return core.Decoded[T]{}, types.NewError(types.ProtocolError)
	}
	if relationErr != nil {
		return core.Decoded[T]{}, types.NewError(types.ProtocolError)
	}
	target, e := attach(&minimal, ep, kind, expected, parentRef)
	if e != nil {
		return core.Decoded[T]{}, e
	}
	out := core.Decoded[T]{Refs: []types.ResourceRef{*target}}
	var v T
	if json.Unmarshal(body, &v) != nil {
		return out, types.NewError(types.ProtocolError)
	}
	if _, e := attach(&v, ep, kind, expected, parentRef); e != nil {
		return out, e
	}
	out.Data = v
	return out, nil
}
